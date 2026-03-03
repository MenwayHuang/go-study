package main

import (
	"context"
	"fmt"
	"time"
)

func RunContextDemo() {
	fmt.Println("--- 示例1: WithCancel 手动取消 ---")
	cancelDemo()

	fmt.Println("\n--- 示例2: WithTimeout 超时控制 ---")
	timeoutDemo()

	fmt.Println("\n--- 示例3: 模拟HTTP请求超时 ---")
	httpTimeoutDemo()

	fmt.Println("\n--- 示例4: Context 传播链 ---")
	propagationDemo()
}

// cancelDemo 手动取消: 父goroutine通知子goroutine退出
// 关键点: 这是管理goroutine生命周期的标准方式
func cancelDemo() {
	ctx, cancel := context.WithCancel(context.Background())

	// 启动一个后台worker
	go func(ctx context.Context) {
		for {
			select {
			case <-ctx.Done():
				// 关键点: ctx.Done() 返回一个channel，cancel()被调用时关闭
				fmt.Printf("  Worker 收到取消信号: %v\n", ctx.Err())
				return
			default:
				fmt.Println("  Worker 工作中...")
				time.Sleep(50 * time.Millisecond)
			}
		}
	}(ctx)

	// 让worker运行一会儿
	time.Sleep(150 * time.Millisecond)
	cancel() // 发送取消信号
	time.Sleep(20 * time.Millisecond)
	fmt.Println("  主程序: Worker 已取消")
}

// timeoutDemo 超时自动取消
// 关键点: 在微服务中，每个请求都应该有超时控制
//   - 防止下游服务慢导致请求堆积
//   - 防止goroutine泄漏
func timeoutDemo() {
	// 关键点: WithTimeout 会在超时后自动调用 cancel
	// 但仍然建议 defer cancel()，确保资源及时释放
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// 模拟一个耗时操作
	select {
	case <-time.After(200 * time.Millisecond): // 模拟200ms的操作
		fmt.Println("  操作完成")
	case <-ctx.Done():
		fmt.Printf("  操作超时: %v (设置了100ms超时)\n", ctx.Err())
	}
}

// httpTimeoutDemo 模拟微服务间调用的超时控制
// 关键点: 实际项目中，调用下游服务必须设置超时
//   - 网关层通常设置总超时（如3秒）
//   - 每个下游调用设置各自超时（如1秒）
//   - 超时应层层传递，子context不能超过父context的deadline
func httpTimeoutDemo() {
	// 模拟: 网关收到请求，总超时200ms
	gatewayCtx, gatewayCancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer gatewayCancel()

	// 调用服务A（快，50ms）
	resultA := make(chan string, 1)
	go func() {
		resultA <- callService(gatewayCtx, "ServiceA", 50*time.Millisecond)
	}()

	// 调用服务B（慢，300ms，会超时）
	resultB := make(chan string, 1)
	go func() {
		resultB <- callService(gatewayCtx, "ServiceB", 300*time.Millisecond)
	}()

	// 收集结果
	for i := 0; i < 2; i++ {
		select {
		case r := <-resultA:
			fmt.Printf("  %s\n", r)
		case r := <-resultB:
			fmt.Printf("  %s\n", r)
		case <-gatewayCtx.Done():
			fmt.Printf("  网关总超时: %v\n", gatewayCtx.Err())
			return
		}
	}
}

func callService(ctx context.Context, name string, latency time.Duration) string {
	select {
	case <-time.After(latency):
		return fmt.Sprintf("%s 响应成功 (耗时 %v)", name, latency)
	case <-ctx.Done():
		return fmt.Sprintf("%s 被取消: %v", name, ctx.Err())
	}
}

// propagationDemo 展示 Context 的树形传播
// 关键点:
//   - 父context取消，所有子context都会被取消
//   - 子context取消，不影响父context
//   - 这就是为什么context适合做请求级别的控制
func propagationDemo() {
	// 根context
	rootCtx, rootCancel := context.WithCancel(context.Background())

	// 子context A (继承root)
	childCtxA, _ := context.WithCancel(rootCtx)
	// 子context B (继承root)
	childCtxB, _ := context.WithCancel(rootCtx)
	// 孙context (继承A)
	grandchildCtx, _ := context.WithCancel(childCtxA)

	// 监听各个context
	monitor := func(name string, ctx context.Context) {
		<-ctx.Done()
		fmt.Printf("  %s 被取消\n", name)
	}

	go monitor("ChildA", childCtxA)
	go monitor("ChildB", childCtxB)
	go monitor("GrandChild", grandchildCtx)

	// 取消root，所有后代都被取消
	fmt.Println("  取消 Root Context...")
	rootCancel()
	time.Sleep(50 * time.Millisecond)
	fmt.Println("  所有子context已级联取消 ✓")
}
