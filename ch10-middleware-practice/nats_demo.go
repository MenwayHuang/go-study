package main

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go"
)

// ============================================================
// NATS 实战 — 云原生消息系统
// ============================================================
//
// 前置条件: docker-compose up -d (启动NATS)
// 观测方式: 浏览器打开 http://localhost:8222 查看连接和订阅
//
// NATS 三种模式:
//   1. Pub/Sub  — 广播，所有订阅者都收到
//   2. Queue Group — 负载均衡，同组只有一个消费者收到
//   3. Request/Reply — 同步RPC调用
//
// 连接管理:
//   生产中 NATS 连接应全局复用（一个进程一个连接即可）
//   连接选项中配置断线重连、错误处理回调
//   本Demo每个子函数用 Unsubscribe() 清理订阅，共享同一个连接

// connectNATS 创建 NATS 连接（演示连接选项的配置方式）
func connectNATS() (*nats.Conn, error) {
	nc, err := nats.Connect("nats://localhost:4222",
		nats.Name("go-study-client"),      // 连接名称 (在 http://localhost:8222/connz 可见)
		nats.ReconnectWait(2*time.Second), // 断线重连间隔
		nats.MaxReconnects(10),            // 最大重连次数
		nats.DisconnectErrHandler(func(nc *nats.Conn, err error) {
			fmt.Printf("  ⚠️ NATS断开连接: %v\n", err)
		}),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			fmt.Printf("  ✅ NATS重新连接: %s\n", nc.ConnectedUrl())
		}),
	)
	return nc, err
}

func RunNatsDemo() {
	nc, err := connectNATS()
	if err != nil {
		fmt.Printf("❌ NATS连接失败: %v\n", err)
		fmt.Println("   请先运行: docker-compose up -d")
		return
	}
	defer nc.Close()
	fmt.Printf("✅ NATS连接成功: %s\n", nc.ConnectedUrl())

	fmt.Println("\n--- 模式1: Pub/Sub (广播) ---")
	pubsubDemo(nc)

	fmt.Println("\n--- 模式2: Queue Group (负载均衡消费) ---")
	queueGroupDemo(nc)

	fmt.Println("\n--- 模式3: Request/Reply (RPC) ---")
	requestReplyDemo(nc)

	fmt.Println("\n--- 实战: 异步订单处理 ---")
	asyncOrderDemo(nc)
}

// ============================================================
// 模式1: Pub/Sub — 发布订阅（广播）
// ============================================================
// 关键点:
//   - 发布者发一条消息，所有订阅者都能收到
//   - 场景: 配置变更通知、事件广播、日志收集
//   - 注意: 消息是即时的，没有订阅者在线则消息丢失
//     (如需持久化，使用 NATS JetStream)

func pubsubDemo(nc *nats.Conn) {
	var wg sync.WaitGroup
	var subs []*nats.Subscription

	// 3个订阅者，都订阅同一个主题
	for i := 1; i <= 3; i++ {
		wg.Add(1)
		subID := i
		sub, _ := nc.Subscribe("events.user.login", func(msg *nats.Msg) {
			fmt.Printf("  订阅者%d 收到: %s\n", subID, string(msg.Data))
			wg.Done()
		})
		subs = append(subs, sub)
	}

	nc.Flush()
	time.Sleep(50 * time.Millisecond)

	// 发布一条消息 → 3个订阅者都会收到
	nc.Publish("events.user.login", []byte(`{"user":"张三","time":"2024-01-01"}`))
	wg.Wait()
	fmt.Println("  → 1条消息，3个订阅者都收到了 (广播)")

	// 关键点: 用完后 Unsubscribe 清理，不影响后续 demo
	for _, sub := range subs {
		sub.Unsubscribe()
	}
}

// ============================================================
// 模式2: Queue Group — 队列组（负载均衡）
// ============================================================
// 关键点:
//   - 同一队列组内，消息只会被组中一个消费者处理
//   - 不同组之间互不影响，各自都能收到全量消息
//   - 场景: 订单处理、任务分发 (类似Kafka消费者组)
//   - 优势: 天然负载均衡，增减消费者即可扩缩容

func queueGroupDemo(nc *nats.Conn) {
	var received [3]int32
	var subs []*nats.Subscription

	// 3个Worker组成一个队列组 "order-workers"
	for i := 0; i < 3; i++ {
		workerID := i
		sub, _ := nc.QueueSubscribe("orders.process", "order-workers", func(msg *nats.Msg) {
			atomic.AddInt32(&received[workerID], 1)
		})
		subs = append(subs, sub)
	}

	nc.Flush()
	time.Sleep(50 * time.Millisecond)

	// 发布30条消息
	for i := 0; i < 30; i++ {
		nc.Publish("orders.process", []byte(fmt.Sprintf(`{"order_id":%d}`, i+1)))
	}

	nc.Flush()
	time.Sleep(200 * time.Millisecond)

	fmt.Println("  30条消息分发到3个Worker (队列组):")
	for i := 0; i < 3; i++ {
		fmt.Printf("    Worker%d 处理了 %d 条\n", i, atomic.LoadInt32(&received[i]))
	}
	fmt.Println("  → 每条消息只被一个Worker处理，自动负载均衡")

	for _, sub := range subs {
		sub.Unsubscribe()
	}
}

// ============================================================
// 模式3: Request/Reply — 请求响应（同步RPC）
// ============================================================
// 关键点:
//   - 客户端发请求，等待服务端响应 (同步阻塞)
//   - NATS自动创建临时reply subject (inbox)
//   - 场景: 微服务间同步调用 (简易版gRPC)
//   - 可以有多个服务端(Queue Subscribe)，NATS自动选一个响应

func requestReplyDemo(nc *nats.Conn) {
	// 启动"用户服务" — 处理用户查询请求
	sub, _ := nc.QueueSubscribe("service.user.get", "user-service", func(msg *nats.Msg) {
		// 收到请求，返回响应
		response := fmt.Sprintf(`{"id":1001,"name":"张三","vip":true}`)
		msg.Respond([]byte(response))
	})

	nc.Flush()
	time.Sleep(50 * time.Millisecond)

	// 客户端发起请求 (带超时)
	msg, err := nc.Request("service.user.get", []byte(`{"user_id":1001}`), 2*time.Second)
	if err != nil {
		fmt.Printf("  请求失败: %v\n", err)
		return
	}
	fmt.Printf("  请求: {user_id:1001}\n")
	fmt.Printf("  响应: %s\n", string(msg.Data))
	fmt.Println("  → 类似RPC调用，但通过NATS解耦，服务端可水平扩展")

	sub.Unsubscribe()
}

// ============================================================
// 实战: 异步订单处理 (结合第4章和第9章)
// ============================================================
// 流程: 秒杀接口 → 发消息到NATS → Worker消费创建订单
// 对比: 第9章用 chan，这里用真实的NATS
//
// 对比表:
//   第9章 chan            |  NATS Queue Group
//   单进程内通信           |  跨进程/跨机器通信
//   进程崩了消息丢失       |  JetStream支持持久化
//   固定Worker数          |  动态增减消费者
//   无监控                |  http://localhost:8222 实时监控

func asyncOrderDemo(nc *nats.Conn) {
	var processedCount int32
	var subs []*nats.Subscription

	// 启动3个订单处理Worker (Queue Group保证不重复消费)
	for i := 0; i < 3; i++ {
		workerID := i
		sub, _ := nc.QueueSubscribe("seckill.order.create", "order-workers", func(msg *nats.Msg) {
			atomic.AddInt32(&processedCount, 1)
			// 模拟处理订单 (写DB、发通知)
			time.Sleep(5 * time.Millisecond)
			_ = workerID
		})
		subs = append(subs, sub)
	}

	nc.Flush()
	time.Sleep(50 * time.Millisecond)

	// 模拟: 秒杀接口收到100个请求，全部发到NATS
	fmt.Println("  模拟100个秒杀请求 → 发送到NATS...")
	for i := 0; i < 100; i++ {
		data := fmt.Sprintf(`{"product_id":"PROD-001","user_id":"user-%d","price":5999}`, i)
		nc.Publish("seckill.order.create", []byte(data))
	}
	nc.Flush()

	time.Sleep(800 * time.Millisecond) // 等Worker处理完

	fmt.Printf("  发送: 100条订单消息\n")
	fmt.Printf("  处理: %d条 (3个Worker并发消费)\n", atomic.LoadInt32(&processedCount))
	fmt.Println("  → NATS Queue Group 实现了真正的消息队列消费")
	fmt.Println("  → 对比第9章的 chan: NATS支持多进程、持久化、集群")

	for _, sub := range subs {
		sub.Unsubscribe()
	}
}
