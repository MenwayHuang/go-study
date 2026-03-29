package mq

import (
	"context"
	"fmt"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

type Rabbit struct {
	Conn *amqp.Connection
	Ch   *amqp.Channel
}

type Config struct {
	URL string
}

func New(cfg Config) (*Rabbit, error) {
	conn, err := amqp.Dial(cfg.URL)
	if err != nil {
		return nil, err
	}
	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	return &Rabbit{Conn: conn, Ch: ch}, nil
}

func (r *Rabbit) Close() {
	_ = r.Ch.Close()
	_ = r.Conn.Close()
}

type PublishOptions struct {
	Headers    amqp.Table
	Expiration string
}

// Setup 声明主队列、重试队列和死信队列：
// - 主队列消费失败后，默认死信到 DLQ。
// - 重试队列本身不被 consumer 直接消费，而是通过消息 TTL 到期后重新投回主队列。
// - 这样可以在不依赖 RabbitMQ 延迟插件的前提下，实现“延迟重试”。
func (r *Rabbit) Setup(exchange, queue, retryQueue, dlq string) error {
	if err := r.Ch.ExchangeDeclare(exchange, "direct", true, false, false, false, nil); err != nil {
		return err
	}

	// 主队列上的失败消息，如果 consumer 选择不再原地重试，就会进入 DLQ。
	mainArgs := amqp.Table{
		"x-dead-letter-exchange":    exchange,
		"x-dead-letter-routing-key": dlq,
	}
	if _, err := r.Ch.QueueDeclare(queue, true, false, false, false, mainArgs); err != nil {
		return err
	}
	// 重试队列的消息在 TTL 到期后会死信回主队列，形成“失败 -> 延迟 -> 再消费”的链路。
	retryArgs := amqp.Table{
		"x-dead-letter-exchange":    exchange,
		"x-dead-letter-routing-key": queue,
	}
	if _, err := r.Ch.QueueDeclare(retryQueue, true, false, false, false, retryArgs); err != nil {
		return err
	}
	if _, err := r.Ch.QueueDeclare(dlq, true, false, false, false, nil); err != nil {
		return err
	}

	if err := r.Ch.QueueBind(queue, queue, exchange, false, nil); err != nil {
		return err
	}
	if err := r.Ch.QueueBind(retryQueue, retryQueue, exchange, false, nil); err != nil {
		return err
	}
	if err := r.Ch.QueueBind(dlq, dlq, exchange, false, nil); err != nil {
		return err
	}
	return nil
}

func (r *Rabbit) PublishJSON(ctx context.Context, exchange, routingKey string, body []byte) error {
	return r.PublishJSONWithOptions(ctx, exchange, routingKey, body, PublishOptions{})
}

// PublishJSONWithOptions 支持附加 headers 和消息级别的 expiration：
// - headers 用来传递重试次数、最后一次错误原因等“投递元信息”；
// - expiration 用来指定这条消息在重试队列里要延迟多久后再回主队列。
func (r *Rabbit) PublishJSONWithOptions(ctx context.Context, exchange, routingKey string, body []byte, opts PublishOptions) error {
	pub := amqp.Publishing{
		ContentType:  "application/json",
		Body:         body,
		DeliveryMode: amqp.Persistent,
		Headers:      opts.Headers,
		Expiration:   opts.Expiration,
		Timestamp:    time.Now(),
	}

	if err := r.Ch.PublishWithContext(ctx, exchange, routingKey, false, false, pub); err != nil {
		return fmt.Errorf("publish: %w", err)
	}
	return nil
}
