package main

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ============================================================
// 发布/订阅模式 (Pub/Sub)
// ============================================================
// 关键点: 与消息队列的区别
//   - 消息队列: 一条消息只被一个消费者处理 (点对点)
//   - 发布订阅: 一条消息被所有订阅者收到 (广播)
//
// 实际场景:
//   - 配置变更通知所有服务
//   - 用户注册后通知: 邮件服务 + 积分服务 + 推荐服务

// PubSub 发布订阅系统
type PubSub struct {
	mu          sync.RWMutex
	subscribers map[string][]chan *Message // topic → 订阅者列表
	bufSize     int
}

func NewPubSub(bufSize int) *PubSub {
	return &PubSub{
		subscribers: make(map[string][]chan *Message),
		bufSize:     bufSize,
	}
}

// Subscribe 订阅一个Topic，返回该订阅者的专属channel
// 关键点: 每个订阅者有自己的channel，发布时广播到所有channel
func (ps *PubSub) Subscribe(topic string) <-chan *Message {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	ch := make(chan *Message, ps.bufSize)
	ps.subscribers[topic] = append(ps.subscribers[topic], ch)
	return ch
}

// Publish 发布消息，广播到所有订阅者
// 关键点: 发布是非阻塞的，如果某个订阅者的channel满了就跳过
// 生产中应该有更好的策略: 如溢出到磁盘、告警等
func (ps *PubSub) Publish(topic string, body []byte) {
	ps.mu.RLock()
	subs := ps.subscribers[topic]
	ps.mu.RUnlock()

	msg := &Message{
		ID:        fmt.Sprintf("pub-%d", time.Now().UnixNano()),
		Topic:     topic,
		Body:      body,
		CreatedAt: time.Now(),
	}

	for _, ch := range subs {
		select {
		case ch <- msg:
		default:
			// 订阅者消费太慢，消息丢弃
			// 生产中可以记录告警
		}
	}
}

// Unsubscribe 取消订阅（关闭channel）
func (ps *PubSub) Unsubscribe(topic string, sub <-chan *Message) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	subs := ps.subscribers[topic]
	for i, ch := range subs {
		if ch == sub {
			close(ch)
			ps.subscribers[topic] = append(subs[:i], subs[i+1:]...)
			return
		}
	}
}

// ============================================================
// 演示
// ============================================================

func RunPubSubDemo() {
	fmt.Println("--- 示例: 发布/订阅 — 一条消息多个服务消费 ---")
	fmt.Println("  场景: 用户注册事件 → 邮件服务 + 积分服务 + 推荐服务")

	ps := NewPubSub(10)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var wg sync.WaitGroup

	// 订阅者1: 邮件服务
	emailCh := ps.Subscribe("user.registered")
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case msg := <-emailCh:
				fmt.Printf("  📧 邮件服务: 发送欢迎邮件给 %s\n", string(msg.Body))
			case <-ctx.Done():
				return
			}
		}
	}()

	// 订阅者2: 积分服务
	pointsCh := ps.Subscribe("user.registered")
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case msg := <-pointsCh:
				fmt.Printf("  🎁 积分服务: 赠送新用户积分给 %s\n", string(msg.Body))
			case <-ctx.Done():
				return
			}
		}
	}()

	// 订阅者3: 推荐服务
	recCh := ps.Subscribe("user.registered")
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case msg := <-recCh:
				fmt.Printf("  🔮 推荐服务: 初始化推荐模型给 %s\n", string(msg.Body))
			case <-ctx.Done():
				return
			}
		}
	}()

	// 发布者: 模拟用户注册
	time.Sleep(50 * time.Millisecond) // 等订阅者就绪
	users := []string{"Alice", "Bob", "Charlie"}
	for _, user := range users {
		ps.Publish("user.registered", []byte(user))
		fmt.Printf("  → 发布事件: %s 注册了\n", user)
		time.Sleep(100 * time.Millisecond)
	}

	time.Sleep(200 * time.Millisecond)
	cancel()
	wg.Wait()
	fmt.Println("  ✅ Pub/Sub 演示完成 — 每条消息被3个服务各自独立消费")
}
