package consumer

import (
	"context"
	"encoding/json"
	"errors"
	"log"
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
	DLQ               string
	Workers           int
	Prefetch          int
	DedupTTL          time.Duration
	QueuePollInterval time.Duration
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
	return Config{
		RabbitURL:         url,
		RabbitMgmtURL:     mgmtURL,
		RabbitMgmtUser:    mgmtUser,
		RabbitMgmtPass:    mgmtPass,
		RabbitVHost:       vhost,
		RedisAddr:         redisAddr,
		Exchange:          "tasks.ex",
		Queue:             "tasks.q",
		DLQ:               "tasks.dlq",
		Workers:           workers,
		Prefetch:          prefetch,
		DedupTTL:          10 * time.Minute,
		QueuePollInterval: 2 * time.Second,
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
	if err := r.Setup(cfg.Exchange, cfg.Queue, cfg.DLQ); err != nil {
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

	log.Printf("queue stats polling started: mgmt_url=%s vhost=%s queue=%s dlq=%s interval=%s", c.cfg.RabbitMgmtURL, c.cfg.RabbitVHost, c.cfg.Queue, c.cfg.DLQ, c.cfg.QueuePollInterval)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			qi, err := c.mgmt.GetQueueInfo(ctx, c.cfg.RabbitVHost, c.cfg.Queue)
			if err == nil {
				metrics.QueueMessages.WithLabelValues(c.cfg.Queue).Set(float64(qi.Messages))
				metrics.QueueConsumers.WithLabelValues(c.cfg.Queue).Set(float64(qi.Consumers))
				log.Printf("queue stats updated: queue=%s messages=%d consumers=%d", c.cfg.Queue, qi.Messages, qi.Consumers)
			} else {
				log.Printf("queue stats poll failed: queue=%s err=%v", c.cfg.Queue, err)
			}
			dlq, err := c.mgmt.GetQueueInfo(ctx, c.cfg.RabbitVHost, c.cfg.DLQ)
			if err == nil {
				metrics.QueueMessages.WithLabelValues(c.cfg.DLQ).Set(float64(dlq.Messages))
				metrics.QueueConsumers.WithLabelValues(c.cfg.DLQ).Set(float64(dlq.Consumers))
				log.Printf("queue stats updated: queue=%s messages=%d consumers=%d", c.cfg.DLQ, dlq.Messages, dlq.Consumers)
			} else {
				log.Printf("queue stats poll failed: queue=%s err=%v", c.cfg.DLQ, err)
			}
		}
	}
}

func (c *Consumer) handle(ctx context.Context, d amqp.Delivery) {
	start := time.Now()
	defer func() {
		metrics.TaskConsumeDuration.Observe(time.Since(start).Seconds())
	}()

	var t Task
	if err := json.Unmarshal(d.Body, &t); err != nil {
		_ = d.Reject(false)
		metrics.TaskConsumedTotal.WithLabelValues("error").Inc()
		return
	}

	seen, err := c.dedup.SeenBefore(ctx, t.TaskID)
	if err != nil {
		// Redis 不可用时，谨慎起见：不要 ack，走重试（或降级为本地幂等）。
		_ = d.Nack(false, true)
		metrics.TaskConsumedTotal.WithLabelValues("error").Inc()
		return
	}
	if seen {
		// 重复消息：直接 ack，避免重复执行
		_ = d.Ack(false)
		metrics.TaskConsumedTotal.WithLabelValues("dedup").Inc()
		return
	}

	// 模拟业务处理
	if err := doWork(ctx, t); err != nil {
		// 失败：reject 不重回队列，进入 DLQ（死信队列）
		_ = d.Reject(false)
		metrics.TaskRetryTotal.Inc()
		metrics.TaskConsumedTotal.WithLabelValues("error").Inc()
		log.Printf("task failed task_id=%s err=%v", t.TaskID, err)
		return
	}

	_ = d.Ack(false)
	metrics.TaskConsumedTotal.WithLabelValues("ok").Inc()
}

func doWork(ctx context.Context, t Task) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(30 * time.Millisecond):
	}

	// 你可以按需制造失败：比如每 10 个失败一次，用于演练 DLQ
	return nil
}
