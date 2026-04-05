package consumer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"golang.org/x/sync/errgroup"

	"go-study/projects/mq-task-system/internal/dedup"
	"go-study/projects/mq-task-system/internal/metrics"
	"go-study/projects/mq-task-system/internal/mq"
)

type Config struct {
	RabbitURL         string
	RabbitMgmtURL     string
	RabbitMgmtUser    string
	RabbitMgmtPass    string
	RabbitVHost       string
	RedisAddr         string
	Exchange          string
	Queue             string
	RetryQ            string
	DLQ               string
	Workers           int
	Prefetch          int
	DedupTTL          time.Duration
	QueuePollInterval time.Duration
	RetryDelay        time.Duration
	MaxRetries        int
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
	return Config{
		RabbitURL:         url,
		RabbitMgmtURL:     mgmtURL,
		RabbitMgmtUser:    mgmtUser,
		RabbitMgmtPass:    mgmtPass,
		RabbitVHost:       vhost,
		RedisAddr:         redisAddr,
		Exchange:          "tasks.ex",
		Queue:             "tasks.q",
		RetryQ:            "tasks.retry",
		DLQ:               "tasks.dlq",
		Workers:           workers,
		Prefetch:          prefetch,
		DedupTTL:          10 * time.Minute,
		QueuePollInterval: 2 * time.Second,
		RetryDelay:        time.Duration(retryDelayMs) * time.Millisecond,
		MaxRetries:        maxRetries,
	}
}

type Consumer struct {
	r     *mq.Rabbit
	mgmt  *mq.MgmtClient
	dedup *dedup.RedisDedup
	cfg   Config
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
	if err := r.Setup(cfg.Exchange, cfg.Queue, cfg.RetryQ, cfg.DLQ); err != nil {
		r.Close()
		return nil, err
	}
	if err := r.Ch.Qos(cfg.Prefetch, 0, false); err != nil {
		r.Close()
		return nil, err
	}
	d := dedup.NewRedisDedup(cfg.RedisAddr, cfg.DedupTTL)
	mgmt := mq.NewMgmtClient(cfg.RabbitMgmtURL, cfg.RabbitMgmtUser, cfg.RabbitMgmtPass)
	return &Consumer{r: r, mgmt: mgmt, dedup: d, cfg: cfg}, nil
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

	go c.pollQueueStats(ctx)

	jobs := make(chan amqp.Delivery)
	g, ctx := errgroup.WithContext(ctx)

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

	err = g.Wait()
	if err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}

func (c *Consumer) handle(ctx context.Context, d amqp.Delivery) {
	start := time.Now()
	defer func() {
		metrics.TaskConsumeDuration.Observe(time.Since(start).Seconds())
	}()

	var t Task
	if err := json.Unmarshal(d.Body, &t); err != nil {
		_ = c.sendToDLQ(ctx, d, "invalid_payload")
		metrics.TaskConsumedTotal.WithLabelValues("error").Inc()
		return
	}

	seen, err := c.dedup.SeenBefore(ctx, t.TaskID)
	if err != nil {
		_ = c.scheduleRetry(ctx, d, "dedup_store_unavailable")
		metrics.TaskConsumedTotal.WithLabelValues("error").Inc()
		return
	}
	if seen {
		_ = d.Ack(false)
		metrics.TaskConsumedTotal.WithLabelValues("dedup").Inc()
		return
	}

	if err := doWork(ctx, t); err != nil {
		_ = c.scheduleRetry(ctx, d, err.Error())
		metrics.TaskConsumedTotal.WithLabelValues("error").Inc()
		return
	}

	_ = d.Ack(false)
	metrics.TaskConsumedTotal.WithLabelValues("ok").Inc()
}

func (c *Consumer) pollQueueStats(ctx context.Context) {
	ticker := time.NewTicker(c.cfg.QueuePollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.updateQueueStats(ctx, c.cfg.Queue)
			c.updateQueueStats(ctx, c.cfg.RetryQ)
			c.updateQueueStats(ctx, c.cfg.DLQ)
		}
	}
}

func (c *Consumer) updateQueueStats(ctx context.Context, queue string) {
	qi, err := c.mgmt.GetQueueInfo(ctx, c.cfg.RabbitVHost, queue)
	if err != nil {
		return
	}
	metrics.QueueMessages.WithLabelValues(queue).Set(float64(qi.Messages))
	metrics.QueueConsumers.WithLabelValues(queue).Set(float64(qi.Consumers))
}

func (c *Consumer) scheduleRetry(ctx context.Context, d amqp.Delivery, reason string) error {
	retryCount := deliveryRetryCount(d) + 1
	if retryCount > c.cfg.MaxRetries {
		metrics.TaskRetryTotal.Inc()
		return c.sendToDLQ(ctx, d, reason)
	}
	headers := cloneHeaders(d.Headers)
	headers["x-retry-count"] = int32(retryCount)
	headers["x-last-error"] = reason
	if err := c.r.PublishJSONWithOptions(ctx, c.cfg.Exchange, c.cfg.RetryQ, d.Body, mq.PublishOptions{
		Headers:    headers,
		Expiration: strconv.FormatInt(c.cfg.RetryDelay.Milliseconds(), 10),
	}); err != nil {
		_ = d.Nack(false, true)
		return err
	}
	metrics.TaskRetryTotal.Inc()
	return d.Ack(false)
}

func (c *Consumer) sendToDLQ(ctx context.Context, d amqp.Delivery, reason string) error {
	headers := cloneHeaders(d.Headers)
	headers["x-last-error"] = reason
	if _, ok := headers["x-retry-count"]; !ok {
		headers["x-retry-count"] = int32(deliveryRetryCount(d))
	}
	if err := c.r.PublishJSONWithOptions(ctx, c.cfg.Exchange, c.cfg.DLQ, d.Body, mq.PublishOptions{Headers: headers}); err != nil {
		_ = d.Nack(false, true)
		return err
	}
	return d.Ack(false)
}

func cloneHeaders(src amqp.Table) amqp.Table {
	dst := amqp.Table{}
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

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
