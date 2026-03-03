package main

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"
)

// ============================================================
// 实战: 订单异步处理系统
// ============================================================
// 架构:
//   HTTP请求 → 创建订单(同步) → 发MQ消息(异步) → 返回用户
//                                    ↓
//                        ┌────── 消费者组 ──────┐
//                        │  Worker1: 扣库存     │
//                        │  Worker2: 发通知     │
//                        │  Worker3: 记日志     │
//                        └────────────────────┘
//
// 关键点:
//   - 核心操作（创建订单）同步处理，快速返回
//   - 非核心操作（通知、日志）异步处理，解耦
//   - 消费失败可以重试，不影响用户体验

type Order struct {
	ID     string
	UserID string
	Amount float64
	Status string
}

type OrderEvent struct {
	Type  string // "created", "paid", "shipped"
	Order Order
}

func RunOrderDemo() {
	fmt.Println("--- 实战: 订单异步处理系统 ---")

	// 创建事件总线
	eventBus := NewPubSub(100)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	var processed int64

	// 消费者1: 库存服务 (扣减库存)
	stockCh := eventBus.Subscribe("order.created")
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case msg := <-stockCh:
				time.Sleep(30 * time.Millisecond) // 模拟扣库存
				fmt.Printf("  📦 库存服务: 扣减库存 - %s\n", string(msg.Body))
				atomic.AddInt64(&processed, 1)
			case <-ctx.Done():
				return
			}
		}
	}()

	// 消费者2: 通知服务 (发短信/邮件)
	notifyCh := eventBus.Subscribe("order.created")
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case msg := <-notifyCh:
				time.Sleep(50 * time.Millisecond) // 模拟发通知
				fmt.Printf("  📱 通知服务: 发送下单通知 - %s\n", string(msg.Body))
				atomic.AddInt64(&processed, 1)
			case <-ctx.Done():
				return
			}
		}
	}()

	// 消费者3: 日志服务 (记录操作日志)
	logCh := eventBus.Subscribe("order.created")
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case msg := <-logCh:
				time.Sleep(10 * time.Millisecond) // 模拟写日志
				fmt.Printf("  📝 日志服务: 记录日志 - %s\n", string(msg.Body))
				atomic.AddInt64(&processed, 1)
			case <-ctx.Done():
				return
			}
		}
	}()

	// 模拟用户下单 (生产者)
	time.Sleep(50 * time.Millisecond) // 等消费者就绪

	fmt.Println("\n  === 模拟5个用户下单 ===")
	start := time.Now()
	for i := 1; i <= 5; i++ {
		order := Order{
			ID:     fmt.Sprintf("ORD-%04d", i),
			UserID: fmt.Sprintf("USER-%03d", rand.Intn(100)),
			Amount: float64(rand.Intn(1000)) + 0.99,
			Status: "created",
		}

		// 同步: 创建订单 (模拟写数据库)
		time.Sleep(20 * time.Millisecond)
		fmt.Printf("  ✓ 订单创建成功: %s (用户:%s, 金额:%.2f)\n",
			order.ID, order.UserID, order.Amount)

		// 异步: 发布事件到MQ
		body := fmt.Sprintf("%s (用户:%s, ¥%.2f)", order.ID, order.UserID, order.Amount)
		eventBus.Publish("order.created", []byte(body))
	}

	syncTime := time.Since(start)
	fmt.Printf("\n  === 用户视角: 5个订单同步处理耗时 %v ===\n", syncTime)
	fmt.Println("  (异步任务在后台继续处理，用户已收到响应)")

	// 等待异步消费完成
	time.Sleep(1 * time.Second)
	cancel()
	wg.Wait()

	fmt.Printf("\n  === 系统视角: 共异步处理 %d 个事件 ===\n", atomic.LoadInt64(&processed))
	fmt.Println("  ✅ 核心链路快速返回，非核心异步处理 — 这就是消息队列的价值")
}
