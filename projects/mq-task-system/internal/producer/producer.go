package producer

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync/atomic"

	"golang.org/x/sync/errgroup"

	"go-study/projects/mq-task-system/internal/metrics"
	"go-study/projects/mq-task-system/internal/mq"
)

type Config struct {
	RabbitURL string
	Exchange  string
	QueueKey  string
	RetryKey  string
	DLQKey    string
}

func ConfigFromEnv() Config {
	url := os.Getenv("RABBITMQ_URL")
	if url == "" {
		url = "amqp://guest:guest@localhost:5672/"
	}
	return Config{RabbitURL: url, Exchange: "tasks.ex", QueueKey: "tasks.q", RetryKey: "tasks.retry", DLQKey: "tasks.dlq"}
}

type Producer struct {
	r *mq.Rabbit
	c Config
}

type Task struct {
	TaskID  string `json:"task_id"`
	Payload string `json:"payload"`
}

func New(cfg Config) (*Producer, error) {
	r, err := mq.New(mq.Config{URL: cfg.RabbitURL})
	if err != nil {
		return nil, err
	}
	if err := r.Setup(cfg.Exchange, cfg.QueueKey, cfg.RetryKey, cfg.DLQKey); err != nil {
		r.Close()
		return nil, err
	}
	return &Producer{r: r, c: cfg}, nil
}

func (p *Producer) Close() {
	p.r.Close()
}

// PublishMany 使用 errgroup 并发发布任务：
// - 任一 goroutine 出错会取消 ctx，其他 goroutine 尽快结束。
// - 这是一个可直接在面试里讲的 errgroup 实战点。
func (p *Producer) PublishMany(ctx context.Context, n, concurrency int) error {
	g, ctx := errgroup.WithContext(ctx)

	var seq int64
	for i := 0; i < concurrency; i++ {
		g.Go(func() error {
			for {
				select {
				case <-ctx.Done():
					return ctx.Err()
				default:
				}

				id := int(atomic.AddInt64(&seq, 1))
				if id > n {
					return nil
				}

				t := Task{TaskID: fmt.Sprintf("task-%d", id), Payload: "demo"}
				b, _ := json.Marshal(t)
				if err := p.r.PublishJSON(ctx, p.c.Exchange, p.c.QueueKey, b); err != nil {
					return err
				}
				metrics.TaskPublishedTotal.Inc()
			}
		})
	}

	return g.Wait()
}
