package main

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"time"
)

// ============================================================
// 重试策略 — 应对临时性故障
// ============================================================
// 关键点:
//   - 不是所有错误都应该重试! 只有临时性错误才值得重试
//   - 临时性: 网络超时、服务暂时不可用、数据库连接池满
//   - 永久性: 参数错误、权限不足、资源不存在 → 重试无意义

// RetryStrategy 重试策略接口
type RetryStrategy interface {
	// NextDelay 返回第n次重试的等待时间, ok=false表示不再重试
	NextDelay(attempt int) (time.Duration, bool)
	Name() string
}

// FixedRetry 固定间隔重试
type FixedRetry struct {
	MaxAttempts int
	Delay       time.Duration
}

func (f *FixedRetry) NextDelay(attempt int) (time.Duration, bool) {
	if attempt >= f.MaxAttempts {
		return 0, false
	}
	return f.Delay, true
}
func (f *FixedRetry) Name() string { return "固定间隔" }

// ExponentialBackoff 指数退避重试
// 关键点: 每次重试间隔翻倍 1s → 2s → 4s → 8s
// 好处: 给下游服务更多恢复时间
type ExponentialBackoff struct {
	MaxAttempts int
	BaseDelay   time.Duration
	MaxDelay    time.Duration // 最大延迟上限
}

func (e *ExponentialBackoff) NextDelay(attempt int) (time.Duration, bool) {
	if attempt >= e.MaxAttempts {
		return 0, false
	}
	delay := time.Duration(math.Pow(2, float64(attempt))) * e.BaseDelay
	if delay > e.MaxDelay {
		delay = e.MaxDelay
	}
	return delay, true
}
func (e *ExponentialBackoff) Name() string { return "指数退避" }

// ExponentialBackoffWithJitter 指数退避 + 随机抖动
// 关键点: 加随机抖动避免"重试风暴"
//   1000个请求同时失败 → 1秒后同时重试 → 又全部失败!
//   加抖动: 0.7s~1.3s 之间随机重试 → 请求分散开
type ExponentialBackoffWithJitter struct {
	MaxAttempts int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
}

func (e *ExponentialBackoffWithJitter) NextDelay(attempt int) (time.Duration, bool) {
	if attempt >= e.MaxAttempts {
		return 0, false
	}
	delay := time.Duration(math.Pow(2, float64(attempt))) * e.BaseDelay
	if delay > e.MaxDelay {
		delay = e.MaxDelay
	}
	// 关键点: 添加 ±30% 的随机抖动
	jitter := time.Duration(float64(delay) * (0.7 + rand.Float64()*0.6))
	return jitter, true
}
func (e *ExponentialBackoffWithJitter) Name() string { return "指数退避+抖动" }

// Retry 通用重试执行器
// 关键点:
//   - 支持 context 取消
//   - 支持可插拔的重试策略
//   - 返回最后一次的错误
func Retry(ctx context.Context, strategy RetryStrategy, fn func() error) error {
	var lastErr error
	for attempt := 0; ; attempt++ {
		lastErr = fn()
		if lastErr == nil {
			return nil // 成功
		}

		delay, shouldRetry := strategy.NextDelay(attempt)
		if !shouldRetry {
			return fmt.Errorf("重试耗尽(共%d次): %w", attempt+1, lastErr)
		}

		fmt.Printf("    第%d次失败: %v, %v后重试...\n", attempt+1, lastErr, delay)

		select {
		case <-time.After(delay):
			// 等待后继续重试
		case <-ctx.Done():
			return fmt.Errorf("重试被取消: %w", ctx.Err())
		}
	}
}

// ============================================================
// 演示
// ============================================================

func RunRetryDemo() {
	fmt.Println("--- 示例1: 固定间隔重试 ---")
	fixedRetryDemo()

	fmt.Println("\n--- 示例2: 指数退避重试 ---")
	exponentialRetryDemo()

	fmt.Println("\n--- 示例3: 指数退避+抖动 (推荐) ---")
	jitterRetryDemo()
}

func fixedRetryDemo() {
	callCount := 0
	// 模拟: 前2次失败，第3次成功
	unreliableService := func() error {
		callCount++
		if callCount < 3 {
			return fmt.Errorf("连接超时")
		}
		return nil
	}

	strategy := &FixedRetry{MaxAttempts: 5, Delay: 100 * time.Millisecond}
	ctx := context.Background()

	err := Retry(ctx, strategy, unreliableService)
	if err != nil {
		fmt.Printf("  最终失败: %v\n", err)
	} else {
		fmt.Printf("  第%d次调用成功 ✓\n", callCount)
	}
}

func exponentialRetryDemo() {
	callCount := 0
	unreliableService := func() error {
		callCount++
		if callCount < 4 {
			return fmt.Errorf("服务不可用")
		}
		return nil
	}

	strategy := &ExponentialBackoff{
		MaxAttempts: 5,
		BaseDelay:   50 * time.Millisecond,
		MaxDelay:    1 * time.Second,
	}

	start := time.Now()
	err := Retry(context.Background(), strategy, unreliableService)
	elapsed := time.Since(start)

	if err != nil {
		fmt.Printf("  最终失败: %v\n", err)
	} else {
		fmt.Printf("  第%d次调用成功, 总耗时: %v ✓\n", callCount, elapsed)
		fmt.Println("  (注意: 每次重试间隔翻倍 50ms → 100ms → 200ms)")
	}
}

func jitterRetryDemo() {
	callCount := 0
	unreliableService := func() error {
		callCount++
		if callCount < 3 {
			return fmt.Errorf("网络抖动")
		}
		return nil
	}

	strategy := &ExponentialBackoffWithJitter{
		MaxAttempts: 5,
		BaseDelay:   50 * time.Millisecond,
		MaxDelay:    1 * time.Second,
	}

	err := Retry(context.Background(), strategy, unreliableService)
	if err != nil {
		fmt.Printf("  最终失败: %v\n", err)
	} else {
		fmt.Printf("  第%d次调用成功 ✓\n", callCount)
		fmt.Println("  (每次重试间隔有随机抖动，避免重试风暴)")
	}

	fmt.Println("\n  === 重试策略选择指南 ===")
	fmt.Println("  固定间隔:     简单场景，重试次数少")
	fmt.Println("  指数退避:     调用外部服务，给对方恢复时间")
	fmt.Println("  指数退避+抖动: 高并发场景必用，避免重试风暴 (推荐)")
	fmt.Println("  不要重试:     参数错误、权限不足等永久性错误")
}
