package consumer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"golang.org/x/sync/errgroup"

	"go-study/projects/mq-task-system/internal/dedup"
	"go-study/projects/mq-task-system/internal/metrics"
	"go-study/projects/mq-task-system/internal/mq"
)

type Config struct {
	RabbitURL      string
	RabbitMgmtURL  string
	RabbitMgmtUser string
	RabbitMgmtPass string
	RabbitVHost    string
	RedisAddr      string
	Exchange       string
	Queue          string
	// RetryQueue 用来承接“稍后再试”的消息，不被业务 consumer 直接消费。
	RetryQueue string
	// DLQ 用来承接“最终失败”的消息，便于人工排查或后续补偿。
	DLQ               string
	Workers           int
	Prefetch          int
	DedupTTL          time.Duration
	QueuePollInterval time.Duration
	// RetryDelay 是单次重试前的延迟时间，通过消息 expiration 实现。
	RetryDelay time.Duration
	// MaxRetries 控制消息最多经历几次延迟重试，超过后直接进入 DLQ。
	MaxRetries int
	// ConsumeTimeout 用来限制单条消息处理时间，防止 worker 被慢任务长期占住。
	ConsumeTimeout time.Duration
	// RateLimitPerSec 控制 consumer 每秒最多真正开始处理多少条消息。
	RateLimitPerSec int
	// CircuitThreshold / CircuitCooldown 用于简单熔断：连续失败达到阈值后，短时间内不再继续执行业务处理。
	CircuitThreshold int
	CircuitCooldown  time.Duration
}

func ConfigFromEnv() Config {
	url := os.Getenv("RABBITMQ_URL")
	if url == "" {
		url = "amqp://guest:guest@localhost:5672/"
	}
	mgmtURL := os.Getenv("RABBITMQ_MGMT_URL")
	if mgmtURL == "" {
		mgmtURL = "http://localhost:15672"
	}
	mgmtUser := os.Getenv("RABBITMQ_MGMT_USER")
	if mgmtUser == "" {
		mgmtUser = "guest"
	}
	mgmtPass := os.Getenv("RABBITMQ_MGMT_PASS")
	if mgmtPass == "" {
		mgmtPass = "guest"
	}
	vhost := os.Getenv("RABBITMQ_VHOST")
	if vhost == "" {
		vhost = "/"
	}
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}
	workers, _ := strconv.Atoi(os.Getenv("WORKERS"))
	if workers <= 0 {
		workers = 10
	}
	prefetch, _ := strconv.Atoi(os.Getenv("PREFETCH"))
	if prefetch <= 0 {
		prefetch = workers
	}
	retryDelayMs, _ := strconv.Atoi(os.Getenv("RETRY_DELAY_MS"))
	if retryDelayMs <= 0 {
		retryDelayMs = 10000
	}
	maxRetries, _ := strconv.Atoi(os.Getenv("MAX_RETRIES"))
	if maxRetries <= 0 {
		maxRetries = 3
	}
	consumeTimeoutMs, _ := strconv.Atoi(os.Getenv("CONSUME_TIMEOUT_MS"))
	if consumeTimeoutMs <= 0 {
		consumeTimeoutMs = 2000
	}
	rateLimitPerSec, _ := strconv.Atoi(os.Getenv("RATE_LIMIT_PER_SEC"))
	if rateLimitPerSec <= 0 {
		rateLimitPerSec = 200
	}
	circuitThreshold, _ := strconv.Atoi(os.Getenv("CIRCUIT_THRESHOLD"))
	if circuitThreshold <= 0 {
		circuitThreshold = 5
	}
	circuitCooldownMs, _ := strconv.Atoi(os.Getenv("CIRCUIT_COOLDOWN_MS"))
	if circuitCooldownMs <= 0 {
		circuitCooldownMs = 15000
	}
	return Config{
		RabbitURL:         url,
		RabbitMgmtURL:     mgmtURL,
		RabbitMgmtUser:    mgmtUser,
		RabbitMgmtPass:    mgmtPass,
		RabbitVHost:       vhost,
		RedisAddr:         redisAddr,
		Exchange:          "tasks.ex",
		Queue:             "tasks.q",
		RetryQueue:        "tasks.retry.q",
		DLQ:               "tasks.dlq",
		Workers:           workers,
		Prefetch:          prefetch,
		DedupTTL:          10 * time.Minute,
		QueuePollInterval: 2 * time.Second,
		RetryDelay:        time.Duration(retryDelayMs) * time.Millisecond,
		MaxRetries:        maxRetries,
		ConsumeTimeout:    time.Duration(consumeTimeoutMs) * time.Millisecond,
		RateLimitPerSec:   rateLimitPerSec,
		CircuitThreshold:  circuitThreshold,
		CircuitCooldown:   time.Duration(circuitCooldownMs) * time.Millisecond,
	}
}

type Consumer struct {
	r     *mq.Rabbit
	mgmt  *mq.MgmtClient
	dedup *dedup.RedisDedup
	cfg   Config
	// limiter 基于 time.Ticker 实现一个轻量版令牌节流器。
	limiter <-chan time.Time
	// cb 用来保护下游依赖：连续失败太多时，先快速失败并进入延迟重试。
	cb *circuitBreaker
}

// circuitBreaker 是一个最小实现：
// - 连续失败累计到阈值时，进入 open 状态；
// - open 状态持续一段 cooldown 时间后自动关闭。
type circuitBreaker struct {
	mu                  sync.Mutex
	consecutiveFailures int
	openUntil           time.Time
}

type Task struct {
	TaskID  string `json:"task_id"`
	Payload string `json:"payload"`
}

func New(cfg Config) (*Consumer, error) {
	r, err := mq.New(mq.Config{URL: cfg.RabbitURL})
	if err != nil {
		return nil, err
	}
	if err := r.Setup(cfg.Exchange, cfg.Queue, cfg.RetryQueue, cfg.DLQ); err != nil {
		r.Close()
		return nil, err
	}

	if err := r.Ch.Qos(cfg.Prefetch, 0, false); err != nil {
		r.Close()
		return nil, err
	}

	d := dedup.NewRedisDedup(cfg.RedisAddr, cfg.DedupTTL)
	mgmt := mq.NewMgmtClient(cfg.RabbitMgmtURL, cfg.RabbitMgmtUser, cfg.RabbitMgmtPass)
	var limiter <-chan time.Time
	// 这里不直接“拒绝消费”，而是在真正处理前等待令牌，属于消费侧限流。
	if cfg.RateLimitPerSec > 0 {
		interval := time.Second / time.Duration(cfg.RateLimitPerSec)
		if interval <= 0 {
			interval = time.Nanosecond
		}
		ticker := time.NewTicker(interval)
		limiter = ticker.C
	}
	metrics.TaskCircuitState.Set(0)
	return &Consumer{r: r, mgmt: mgmt, dedup: d, cfg: cfg, limiter: limiter, cb: &circuitBreaker{}}, nil
}

func (c *Consumer) Close() {
	c.dedup.Close()
	c.r.Close()
}

func (c *Consumer) Run(ctx context.Context) error {
	deliveries, err := c.r.Ch.Consume(c.cfg.Queue, "", false, false, false, false, nil)
	if err != nil {
		return err
	}

	// 采集队列深度/消费者数，用于“制造积压→恢复”的演练曲线。
	go c.pollQueueStats(ctx)

	jobs := make(chan amqp.Delivery)
	g, ctx := errgroup.WithContext(ctx)

	// dispatcher
	g.Go(func() error {
		defer close(jobs)
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case d, ok := <-deliveries:
				if !ok {
					return errors.New("deliveries closed")
				}
				jobs <- d
			}
		}
	})

	for i := 0; i < c.cfg.Workers; i++ {
		g.Go(func() error {
			for {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case d, ok := <-jobs:
					if !ok {
						return nil
					}
					c.handle(ctx, d)
				}
			}
		})
	}

	// Wait：任一协程出错会取消 ctx，其他 worker 尽快退出。
	err = g.Wait()
	if err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}

func (c *Consumer) pollQueueStats(ctx context.Context) {
	ticker := time.NewTicker(c.cfg.QueuePollInterval)
	defer ticker.Stop()

	log.Printf("queue stats polling started: mgmt_url=%s vhost=%s queue=%s retry=%s dlq=%s interval=%s", c.cfg.RabbitMgmtURL, c.cfg.RabbitVHost, c.cfg.Queue, c.cfg.RetryQueue, c.cfg.DLQ, c.cfg.QueuePollInterval)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.updateQueueStats(ctx, c.cfg.Queue)
			c.updateQueueStats(ctx, c.cfg.RetryQueue)
			c.updateQueueStats(ctx, c.cfg.DLQ)
		}
	}
}

func (c *Consumer) updateQueueStats(ctx context.Context, queue string) {
	qi, err := c.mgmt.GetQueueInfo(ctx, c.cfg.RabbitVHost, queue)
	if err == nil {
		metrics.QueueMessages.WithLabelValues(queue).Set(float64(qi.Messages))
		metrics.QueueConsumers.WithLabelValues(queue).Set(float64(qi.Consumers))
		log.Printf("queue stats updated: queue=%s messages=%d consumers=%d", queue, qi.Messages, qi.Consumers)
		return
	}
	log.Printf("queue stats poll failed: queue=%s err=%v", queue, err)
}

// handle 是消息处理的主入口，处理顺序是：
// 1. 限流；
// 2. 熔断判断；
// 3. 反序列化和幂等检查；
// 4. 带超时的业务处理；
// 5. 成功 ack，失败走延迟重试或 DLQ。
func (c *Consumer) handle(ctx context.Context, d amqp.Delivery) {
	start := time.Now()
	defer func() {
		metrics.TaskConsumeDuration.Observe(time.Since(start).Seconds())
	}()

	if err := c.waitRateLimit(ctx); err != nil {
		_ = d.Nack(false, true)
		metrics.TaskConsumedTotal.WithLabelValues("error").Inc()
		return
	}

	if c.cb.IsOpen() {
		// 熔断打开时，不再直接执行业务处理，而是把消息送入延迟重试链路。
		metrics.TaskCircuitOpenTotal.Inc()
		metrics.TaskConsumedTotal.WithLabelValues("error").Inc()
		if err := c.scheduleRetry(ctx, d, "circuit_open"); err != nil {
			log.Printf("task retry schedule failed task_id=%s err=%v", deliveryTaskID(d), err)
			_ = d.Nack(false, true)
		}
		return
	}

	var t Task
	if err := json.Unmarshal(d.Body, &t); err != nil {
		// 消息体本身就不合法，属于不可恢复错误，直接进入 DLQ。
		if errPub := c.sendToDLQ(ctx, d, "invalid_payload"); errPub != nil {
			log.Printf("task dlq publish failed err=%v", errPub)
			_ = d.Nack(false, true)
			return
		}
		metrics.TaskConsumedTotal.WithLabelValues("error").Inc()
		c.cb.RecordFailure(c.cfg.CircuitThreshold, c.cfg.CircuitCooldown)
		return
	}

	seen, err := c.dedup.SeenBefore(ctx, t.TaskID)
	if err != nil {
		// 幂等存储异常更像“临时依赖故障”，适合延迟后再试，而不是立刻打入 DLQ。
		if errSched := c.scheduleRetry(ctx, d, "dedup_store_unavailable"); errSched != nil {
			log.Printf("task retry schedule failed task_id=%s err=%v", t.TaskID, errSched)
			_ = d.Nack(false, true)
			return
		}
		metrics.TaskConsumedTotal.WithLabelValues("error").Inc()
		c.cb.RecordFailure(c.cfg.CircuitThreshold, c.cfg.CircuitCooldown)
		return
	}
	if seen {
		// 重复消息：直接 ack，避免重复执行
		_ = d.Ack(false)
		metrics.TaskConsumedTotal.WithLabelValues("dedup").Inc()
		c.cb.RecordSuccess()
		return
	}

	// 模拟业务处理
	workCtx, cancel := context.WithTimeout(ctx, c.cfg.ConsumeTimeout)
	defer cancel()
	if err := doWork(workCtx, t); err != nil {
		// 业务处理失败默认走“延迟重试 -> 超过最大次数后进 DLQ”。
		metrics.TaskConsumedTotal.WithLabelValues("error").Inc()
		c.cb.RecordFailure(c.cfg.CircuitThreshold, c.cfg.CircuitCooldown)
		if errSched := c.scheduleRetry(ctx, d, err.Error()); errSched != nil {
			log.Printf("task retry schedule failed task_id=%s err=%v", t.TaskID, errSched)
			_ = d.Nack(false, true)
			return
		}
		log.Printf("task failed task_id=%s err=%v", t.TaskID, err)
		return
	}

	_ = d.Ack(false)
	metrics.TaskConsumedTotal.WithLabelValues("ok").Inc()
	c.cb.RecordSuccess()
}

// waitRateLimit 在真正处理消息前阻塞等待令牌，从而把消费速度限制在可控范围内。
func (c *Consumer) waitRateLimit(ctx context.Context) error {
	if c.limiter == nil {
		return nil
	}
	metrics.TaskRateLimitWaitTotal.Inc()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-c.limiter:
		return nil
	}
}

// scheduleRetry 会把当前消息重新发布到 retry queue：
// - 重试次数放在 headers 里，不污染业务 JSON；
// - expiration 决定这条消息多久后重新回到主队列；
// - 超过最大重试次数则不再进入 retry queue，而是直接转入 DLQ。
func (c *Consumer) scheduleRetry(ctx context.Context, d amqp.Delivery, reason string) error {
	retryCount := deliveryRetryCount(d) + 1
	if retryCount > c.cfg.MaxRetries {
		metrics.TaskRetryTotal.Inc()
		return c.sendToDLQ(ctx, d, reason)
	}
	headers := cloneHeaders(d.Headers)
	headers["x-retry-count"] = int32(retryCount)
	headers["x-last-error"] = reason
	if err := c.r.PublishJSONWithOptions(ctx, c.cfg.Exchange, c.cfg.RetryQueue, d.Body, mq.PublishOptions{
		Headers:    headers,
		Expiration: strconv.FormatInt(c.cfg.RetryDelay.Milliseconds(), 10),
	}); err != nil {
		return err
	}
	metrics.TaskRetryScheduledTotal.Inc()
	return d.Ack(false)
}

// sendToDLQ 用来承接最终失败消息：
// - body 保留原始业务内容；
// - headers 补充最后一次错误原因和重试次数，便于后续排查。
func (c *Consumer) sendToDLQ(ctx context.Context, d amqp.Delivery, reason string) error {
	headers := cloneHeaders(d.Headers)
	headers["x-last-error"] = reason
	if _, ok := headers["x-retry-count"]; !ok {
		headers["x-retry-count"] = int32(deliveryRetryCount(d))
	}
	if err := c.r.PublishJSONWithOptions(ctx, c.cfg.Exchange, c.cfg.DLQ, d.Body, mq.PublishOptions{Headers: headers}); err != nil {
		return err
	}
	metrics.TaskDLQTotal.Inc()
	return d.Ack(false)
}

// cloneHeaders 避免直接修改原消息的 headers，便于安全地追加元信息。
func cloneHeaders(src amqp.Table) amqp.Table {
	dst := amqp.Table{}
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

// deliveryRetryCount 从 RabbitMQ headers 中读取当前消息已经历过的重试次数。
func deliveryRetryCount(d amqp.Delivery) int {
	v, ok := d.Headers["x-retry-count"]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case int32:
		return int(n)
	case int64:
		return int(n)
	case int:
		return n
	case string:
		parsed, _ := strconv.Atoi(n)
		return parsed
	default:
		return 0
	}
}

// deliveryTaskID 主要用于日志打印，尽量在错误场景下也能带出 task_id。
func deliveryTaskID(d amqp.Delivery) string {
	var t Task
	if err := json.Unmarshal(d.Body, &t); err != nil {
		return "unknown"
	}
	return t.TaskID
}

// IsOpen 判断熔断器是否处于打开状态；如果冷却期已过，会自动恢复为关闭状态。
func (cb *circuitBreaker) IsOpen() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	if time.Now().Before(cb.openUntil) {
		metrics.TaskCircuitState.Set(1)
		return true
	}
	if !cb.openUntil.IsZero() {
		cb.openUntil = time.Time{}
		metrics.TaskCircuitState.Set(0)
	}
	return false
}

// RecordFailure 在连续失败达到阈值后打开熔断器。
func (cb *circuitBreaker) RecordFailure(threshold int, cooldown time.Duration) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.consecutiveFailures++
	if cb.consecutiveFailures >= threshold {
		cb.openUntil = time.Now().Add(cooldown)
		cb.consecutiveFailures = 0
		metrics.TaskCircuitState.Set(1)
	}
}

// RecordSuccess 在成功处理一条消息后清空失败计数，并关闭熔断状态。
func (cb *circuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.consecutiveFailures = 0
	cb.openUntil = time.Time{}
	metrics.TaskCircuitState.Set(0)
}

// doWork 用来模拟真实业务处理：
// - payload=fail 时故意失败，方便演练延迟重试 / DLQ / 熔断；
// - 正常情况下耗时约 30ms，用来观察吞吐和延迟。
func doWork(ctx context.Context, t Task) error {
	if t.Payload == "fail" {
		return fmt.Errorf("simulated task failure")
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(30 * time.Millisecond):
	}

	// 你可以按需制造失败：比如每 10 个失败一次，用于演练 DLQ
	return nil
}
