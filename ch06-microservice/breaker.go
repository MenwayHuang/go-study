package main

import (
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"time"
)

// ============================================================
// 熔断器 (Circuit Breaker) — 防止级联雪崩
// ============================================================
// 三种状态:
//   Closed (关闭): 正常放行请求，统计失败率
//   Open   (打开): 直接拒绝请求，快速失败
//   HalfOpen(半开): 允许少量请求探测，成功则关闭，失败则打开

type BreakerState int

const (
	StateClosed   BreakerState = iota // 正常
	StateOpen                          // 熔断中
	StateHalfOpen                      // 探测中
)

func (s BreakerState) String() string {
	switch s {
	case StateClosed:
		return "CLOSED(正常)"
	case StateOpen:
		return "OPEN(熔断)"
	case StateHalfOpen:
		return "HALF-OPEN(探测)"
	}
	return "UNKNOWN"
}

var ErrBreakerOpen = errors.New("circuit breaker is open")

// CircuitBreaker 熔断器
// 关键点:
//   - failThreshold: 连续失败多少次触发熔断
//   - resetTimeout:  熔断后多久进入半开状态尝试恢复
//   - halfOpenMax:   半开状态允许的最大探测请求数
type CircuitBreaker struct {
	mu             sync.Mutex
	state          BreakerState
	failCount      int           // 连续失败次数
	failThreshold  int           // 触发熔断的失败阈值
	successCount   int           // 半开状态的成功次数
	halfOpenMax    int           // 半开状态需要的成功次数
	resetTimeout   time.Duration // 熔断后多久尝试恢复
	lastFailTime   time.Time     // 最后一次失败时间
	totalRequests  int64
	totalFailures  int64
}

func NewCircuitBreaker(failThreshold int, resetTimeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		state:         StateClosed,
		failThreshold: failThreshold,
		halfOpenMax:   2, // 半开状态需要2次成功才完全恢复
		resetTimeout:  resetTimeout,
	}
}

// Execute 通过熔断器执行函数
// 关键点:
//  1. Closed: 正常执行，失败则计数
//  2. Open:   检查是否到了恢复时间，到了转半开，没到直接拒绝
//  3. HalfOpen: 允许执行，成功则关闭，失败则打开
func (cb *CircuitBreaker) Execute(fn func() error) error {
	cb.mu.Lock()

	cb.totalRequests++

	switch cb.state {
	case StateOpen:
		// 检查是否到了恢复时间
		if time.Since(cb.lastFailTime) > cb.resetTimeout {
			cb.state = StateHalfOpen
			cb.successCount = 0
			fmt.Printf("    [熔断器] 状态: OPEN → HALF-OPEN (开始探测)\n")
		} else {
			cb.mu.Unlock()
			cb.totalFailures++
			return ErrBreakerOpen // 快速失败
		}
	case StateHalfOpen:
		// 半开状态，允许继续
	case StateClosed:
		// 正常状态，允许继续
	}
	cb.mu.Unlock()

	// 执行实际函数
	err := fn()

	cb.mu.Lock()
	defer cb.mu.Unlock()

	if err != nil {
		cb.onFailure()
		return err
	}
	cb.onSuccess()
	return nil
}

func (cb *CircuitBreaker) onSuccess() {
	switch cb.state {
	case StateHalfOpen:
		cb.successCount++
		if cb.successCount >= cb.halfOpenMax {
			cb.state = StateClosed
			cb.failCount = 0
			fmt.Printf("    [熔断器] 状态: HALF-OPEN → CLOSED (恢复正常) ✓\n")
		}
	case StateClosed:
		cb.failCount = 0 // 成功一次就重置失败计数
	}
}

func (cb *CircuitBreaker) onFailure() {
	cb.totalFailures++
	cb.lastFailTime = time.Now()

	switch cb.state {
	case StateClosed:
		cb.failCount++
		if cb.failCount >= cb.failThreshold {
			cb.state = StateOpen
			fmt.Printf("    [熔断器] 状态: CLOSED → OPEN (连续失败%d次，触发熔断!) 🔴\n",
				cb.failCount)
		}
	case StateHalfOpen:
		cb.state = StateOpen
		fmt.Printf("    [熔断器] 状态: HALF-OPEN → OPEN (探测失败，继续熔断) 🔴\n")
	}
}

func (cb *CircuitBreaker) State() BreakerState {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.state
}

// ============================================================
// 演示
// ============================================================

func RunBreakerDemo() {
	fmt.Println("--- 示例: 熔断器保护下游服务 ---")
	fmt.Println("  场景: 下游服务前5次正常，中间故障，之后恢复")

	// 创建熔断器: 连续3次失败触发熔断, 500ms后尝试恢复
	breaker := NewCircuitBreaker(3, 500*time.Millisecond)

	callCount := 0
	// 模拟下游服务: 前5次正常，6-15次故障，16+次恢复
	downstreamService := func() error {
		callCount++
		if callCount > 5 && callCount <= 15 {
			return fmt.Errorf("服务不可用")
		}
		return nil
	}

	// 模拟30个请求
	for i := 1; i <= 30; i++ {
		err := breaker.Execute(downstreamService)

		status := "✓"
		errMsg := ""
		if err != nil {
			status = "✗"
			errMsg = err.Error()
			if errors.Is(err, ErrBreakerOpen) {
				errMsg = "熔断器拒绝 (快速失败)"
			}
		}

		fmt.Printf("  请求%2d: %s 状态=%s %s\n", i, status, breaker.State(), errMsg)

		time.Sleep(100 * time.Millisecond)

		// 在熔断期间快速跳过一些请求
		if breaker.State() == StateOpen && rand.Intn(3) > 0 {
			i += 2
		}
	}

	fmt.Println("\n  === 熔断器工作流程总结 ===")
	fmt.Println("  1. 正常时 (CLOSED): 所有请求正常通过")
	fmt.Println("  2. 故障时: 连续失败达阈值 → 熔断器打开 (OPEN)")
	fmt.Println("  3. 熔断中 (OPEN): 请求被直接拒绝，不调用下游，保护系统")
	fmt.Println("  4. 恢复探测 (HALF-OPEN): 超时后允许少量请求探测下游")
	fmt.Println("  5. 探测成功 → CLOSED; 探测失败 → 继续OPEN")
}
