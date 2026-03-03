package main

import (
	"fmt"
	"sync"
	"time"
)

func RunRateLimitDemo() {
	fmt.Println("--- 示例1: 简易令牌桶限流器 ---")
	tokenBucketDemo()

	fmt.Println("\n--- 示例2: 滑动窗口限流器 ---")
	slidingWindowDemo()

	fmt.Println("\n--- 示例3: 用 time.Ticker 实现匀速限流 ---")
	tickerRateLimitDemo()
}

// ============================================================
// 令牌桶 (Token Bucket) — 最常用的限流算法
// ============================================================
// 原理:
//   - 桶里有固定数量的令牌 (capacity)
//   - 以固定速率往桶里放令牌 (rate)
//   - 每个请求消耗一个令牌
//   - 桶满了新令牌丢弃，桶空了请求被拒绝/等待
// 优点: 允许一定程度的突发流量 (桶里攒了令牌就可以突发消费)
// 实际项目理解:
//   - rate 决定“稳态吞吐”（例如每 100ms 放 1 个令牌 ≈ 10 QPS）
//   - capacity 决定“突发能力”（桶里攒满令牌时可以瞬间放行一波）

type TokenBucket struct {
	capacity int           // 桶容量
	tokens   int           // 当前令牌数
	rate     time.Duration // 放令牌的间隔
	mu       sync.Mutex
	stopCh   chan struct{}
}

func NewTokenBucket(capacity int, rate time.Duration) *TokenBucket {
	tb := &TokenBucket{
		capacity: capacity,
		tokens:   capacity, // 初始满桶
		rate:     rate,
		stopCh:   make(chan struct{}),
	}
	// 后台定时放令牌
	go tb.refill()
	return tb
}

func (tb *TokenBucket) refill() {
	ticker := time.NewTicker(tb.rate)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			tb.mu.Lock()
			if tb.tokens < tb.capacity {
				tb.tokens++
			}
			tb.mu.Unlock()
		case <-tb.stopCh:
			return
		}
	}

}

// Allow 尝试获取一个令牌，返回是否允许
func (tb *TokenBucket) Allow() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	if tb.tokens > 0 {
		tb.tokens--
		return true
	}
	return false
}

func (tb *TokenBucket) Stop() {
	// 注意: Stop() 代表不再使用该 limiter。
	// 实际项目里应确保 Stop() 只调用一次（重复 close 会 panic），或用 sync.Once 保护。
	close(tb.stopCh)
}

func tokenBucketDemo() {
	// 创建: 容量5，每100ms补充1个令牌 → 稳态QPS=10
	limiter := NewTokenBucket(5, 100*time.Millisecond)
	defer limiter.Stop()

	allowed := 0
	denied := 0

	// 模拟突发20个请求
	for i := 0; i < 20; i++ {
		if limiter.Allow() {
			allowed++
			fmt.Printf("  请求%d: ✓ 允许\n", i+1)
		} else {
			denied++
			fmt.Printf("  请求%d: ✗ 拒绝 (限流)\n", i+1)
		}
		time.Sleep(30 * time.Millisecond) // 请求间隔30ms，快于令牌补充速率
	}
	fmt.Printf("  结果: 允许=%d, 拒绝=%d\n", allowed, denied)
}

// ============================================================
// 滑动窗口 (Sliding Window) — 精确计数
// ============================================================
// 原理:
//   - 记录每个请求的时间戳
//   - 统计窗口时间内的请求数
//   - 超过阈值则拒绝
// 取舍:
//   - 简易实现通常需要每次清理窗口外时间戳，QPS 很高时可能变慢
//   - 生产级更常用“分桶计数/环形数组”把开销压到近似 O(1)

type SlidingWindow struct {
	windowSize time.Duration
	maxReqs    int
	timestamps []time.Time
	mu         sync.Mutex
}

func NewSlidingWindow(windowSize time.Duration, maxReqs int) *SlidingWindow {
	return &SlidingWindow{
		windowSize: windowSize,
		maxReqs:    maxReqs,
		timestamps: make([]time.Time, 0),
	}
}

func (sw *SlidingWindow) Allow() bool {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	now := time.Now()
	windowStart := now.Add(-sw.windowSize)

	// 清除窗口外的旧时间戳
	valid := sw.timestamps[:0]
	for _, ts := range sw.timestamps {
		if ts.After(windowStart) {
			valid = append(valid, ts)
		}
	}
	sw.timestamps = valid

	if len(sw.timestamps) >= sw.maxReqs {
		return false
	}
	sw.timestamps = append(sw.timestamps, now)
	return true
}

func slidingWindowDemo() {
	// 1秒内最多5个请求
	limiter := NewSlidingWindow(1*time.Second, 5)

	allowed := 0
	denied := 0
	for i := 0; i < 10; i++ {
		if limiter.Allow() {
			allowed++
			fmt.Printf("  请求%d: ✓ 允许\n", i+1)
		} else {
			denied++
			fmt.Printf("  请求%d: ✗ 拒绝\n", i+1)
		}
		time.Sleep(80 * time.Millisecond)
	}
	fmt.Printf("  1秒窗口限制5个: 允许=%d, 拒绝=%d\n", allowed, denied)
}

// ============================================================
// time.Ticker 匀速限流 — 最简单的限流方式
// ============================================================
// 关键点: 用 time.Ticker 控制请求速率，适合简单场景
// 实际项目推荐使用 golang.org/x/time/rate 包
// 注意:
//   - 一定要 Stop() ticker，避免资源泄漏
//   - ticker 适合“匀速执行”；如果需要“允许突发”，更适合令牌桶

func tickerRateLimitDemo() {
	// 每100ms允许一个请求 → QPS=10
	limiter := time.NewTicker(100 * time.Millisecond)
	defer limiter.Stop()

	requests := make(chan int, 20)
	for i := 1; i <= 8; i++ {
		requests <- i
	}
	close(requests)

	start := time.Now()
	for req := range requests {
		<-limiter.C // 等待下一个tick
		fmt.Printf("  请求%d 在 %v 时执行\n", req, time.Since(start).Round(time.Millisecond))
	}
	fmt.Println("  (请求被匀速处理，间隔约100ms)")
}
