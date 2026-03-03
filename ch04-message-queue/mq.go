package main

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ============================================================
// 内存消息队列实现 — 理解MQ核心原理
// ============================================================

// Message 消息结构
type Message struct {
	ID        string
	Topic     string
	Body      []byte
	CreatedAt time.Time
	Retries   int // 重试次数
}

// MessageQueue 内存消息队列
// 关键点: 这是一个简化版的MQ，帮助理解核心概念
//   - Topic: 消息分类，类似 Kafka 的 Topic
//   - 消费者组: 同组内竞争消费，不同组各自消费（广播）
//   - ACK: 消费确认机制
type MessageQueue struct {
	topics map[string]chan *Message // 每个topic一个channel
	mu     sync.RWMutex
	bufLen int // channel 缓冲大小
}

func NewMessageQueue(bufferSize int) *MessageQueue {
	return &MessageQueue{
		topics: make(map[string]chan *Message),
		bufLen: bufferSize,
	}
}

// Publish 发布消息到指定Topic
// 关键点: 生产者只需要知道Topic名称，不关心谁来消费
func (mq *MessageQueue) Publish(topic string, body []byte) error {
	mq.mu.Lock()
	ch, ok := mq.topics[topic]
	if !ok {
		ch = make(chan *Message, mq.bufLen)
		mq.topics[topic] = ch
	}
	mq.mu.Unlock()

	msg := &Message{
		ID:        fmt.Sprintf("%s-%d", topic, time.Now().UnixNano()),
		Topic:     topic,
		Body:      body,
		CreatedAt: time.Now(),
	}

	select {
	case ch <- msg:
		return nil
	default:
		// 关键点: 队列满了的处理策略
		// 实际项目中可以: 阻塞等待 / 丢弃 / 写入溢出存储
		return fmt.Errorf("topic [%s] 队列已满 (容量: %d)", topic, mq.bufLen)
	}
}

// Subscribe 订阅Topic，返回消息channel
// 关键点: 多个消费者订阅同一个Topic时，消息是竞争消费的（类似负载均衡）
func (mq *MessageQueue) Subscribe(topic string) <-chan *Message {
	mq.mu.Lock()
	defer mq.mu.Unlock()

	ch, ok := mq.topics[topic]
	if !ok {
		ch = make(chan *Message, mq.bufLen)
		mq.topics[topic] = ch
	}
	return ch
}

// QueueLen 获取指定Topic的队列长度（监控用）
func (mq *MessageQueue) QueueLen(topic string) int {
	mq.mu.RLock()
	defer mq.mu.RUnlock()
	if ch, ok := mq.topics[topic]; ok {
		return len(ch)
	}
	return 0
}

// ============================================================
// 演示
// ============================================================

func RunMQDemo() {
	fmt.Println("--- 示例: 内存消息队列 —— 生产者/消费者模式 ---")

	mq := NewMessageQueue(100) // 缓冲100条消息

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var wg sync.WaitGroup

	// 启动3个消费者 (竞争消费同一个Topic)
	for i := 1; i <= 3; i++ {
		wg.Add(1)
		go func(consumerID int) {
			defer wg.Done()
			ch := mq.Subscribe("orders")
			for {
				select {
				case msg := <-ch:
					// 模拟消息处理
					time.Sleep(50 * time.Millisecond)
					fmt.Printf("  消费者%d 处理消息: %s [%s]\n",
						consumerID, string(msg.Body), msg.ID[:20])
				case <-ctx.Done():
					fmt.Printf("  消费者%d 退出\n", consumerID)
					return
				}
			}
		}(i)
	}

	// 生产者: 快速发送20条消息
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 1; i <= 20; i++ {
			body := fmt.Sprintf("订单#%d", i)
			if err := mq.Publish("orders", []byte(body)); err != nil {
				fmt.Printf("  发送失败: %v\n", err)
			}
			time.Sleep(20 * time.Millisecond) // 生产速度 > 消费速度 → 队列会积压
		}
		fmt.Printf("  生产者完成, 队列剩余: %d 条\n", mq.QueueLen("orders"))
	}()

	wg.Wait()
	fmt.Println("  ✅ 消息队列演示完成")
}
