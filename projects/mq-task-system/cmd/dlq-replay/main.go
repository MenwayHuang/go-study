package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go-study/projects/mq-task-system/internal/consumer"
	"go-study/projects/mq-task-system/internal/mq"
)

// dlq-replay 用于将 DLQ（死信队列）中的消息重放回主队列。
// 面试价值：补齐生产系统常见的“失败隔离 + 人工修复后重放”闭环。
func main() {
	var (
		n     = flag.Int("n", 100, "max messages to replay")
		sleep = flag.Duration("sleep", 0, "sleep duration between messages")
	)
	flag.Parse()

	cfg := consumer.ConfigFromEnv()

	r, err := mq.New(mq.Config{URL: cfg.RabbitURL})
	if err != nil {
		log.Fatal(err)
	}
	defer r.Close()

	if err := r.Setup(cfg.Exchange, cfg.Queue, cfg.RetryQ, cfg.DLQ); err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	deliveries, err := r.Ch.Consume(cfg.DLQ, "", false, false, false, false, nil)
	if err != nil {
		log.Fatal(err)
	}

	replayed := 0
	for {
		if *n > 0 && replayed >= *n {
			log.Printf("replay done replayed=%d", replayed)
			return
		}

		select {
		case <-ctx.Done():
			log.Printf("replay canceled replayed=%d", replayed)
			return
		case d, ok := <-deliveries:
			if !ok {
				log.Printf("dlq deliveries closed replayed=%d", replayed)
				return
			}
			// 原样重放到主队列 routing key。
			err := r.PublishJSON(ctx, cfg.Exchange, cfg.Queue, d.Body)
			if err != nil {
				_ = d.Nack(false, true)
				log.Printf("replay publish failed, nack requeue=true err=%v", err)
				continue
			}

			_ = d.Ack(false)
			replayed++
			if *sleep > 0 {
				time.Sleep(*sleep)
			}
		}
	}
}
