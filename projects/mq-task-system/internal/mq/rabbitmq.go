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

// Setup 声明 exchange/queue/DLQ 结构。
func (r *Rabbit) Setup(exchange, queue, dlq string) error {
	if err := r.Ch.ExchangeDeclare(exchange, "direct", true, false, false, false, nil); err != nil {
		return err
	}

	args := amqp.Table{
		"x-dead-letter-exchange":    exchange,
		"x-dead-letter-routing-key": dlq,
	}
	if _, err := r.Ch.QueueDeclare(queue, true, false, false, false, args); err != nil {
		return err
	}
	if _, err := r.Ch.QueueDeclare(dlq, true, false, false, false, nil); err != nil {
		return err
	}

	if err := r.Ch.QueueBind(queue, queue, exchange, false, nil); err != nil {
		return err
	}
	if err := r.Ch.QueueBind(dlq, dlq, exchange, false, nil); err != nil {
		return err
	}
	return nil
}

func (r *Rabbit) PublishJSON(ctx context.Context, exchange, routingKey string, body []byte) error {
	pub := amqp.Publishing{
		ContentType:  "application/json",
		Body:         body,
		DeliveryMode: amqp.Persistent,
		Timestamp:    time.Now(),
	}

	if err := r.Ch.PublishWithContext(ctx, exchange, routingKey, false, false, pub); err != nil {
		return fmt.Errorf("publish: %w", err)
	}
	return nil
}
