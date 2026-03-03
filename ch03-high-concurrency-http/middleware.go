package main

import (
	"fmt"
	"log"
	"net/http"
	"runtime/debug"
	"sync"
	"time"
)

// Middleware 中间件类型定义
// 关键点: 中间件的本质就是一个函数，接收Handler返回Handler
// 这种设计让中间件可以像"洋葱"一样层层包裹
type Middleware func(http.Handler) http.Handler

// ChainMiddleware 将多个中间件串联起来
// 关键点: 中间件从右到左包裹，但执行时从左到右
// Chain(handler, A, B, C) → A(B(C(handler)))
// 请求流: A.before → B.before → C.before → handler → C.after → B.after → A.after
func ChainMiddleware(handler http.Handler, middlewares ...Middleware) http.Handler {
	// 从后往前包裹，这样第一个中间件在最外层
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}
	return handler
}

// ============================================================
// 中间件1: 请求日志
// ============================================================
// 关键点: 生产环境必备，记录每个请求的关键信息
//   - 方法、路径、状态码、耗时
//   - 用于排查问题和性能监控

// responseWriter 包装 http.ResponseWriter 以捕获状态码
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func LoggerMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(rw, r) // 调用下一层

		// 关键点: 在Handler执行后记录日志（洋葱模型的"回程"）
		log.Printf("[%s] %s %s %d %v",
			r.Method, r.URL.Path, r.RemoteAddr, rw.statusCode, time.Since(start))
	})
}

// ============================================================
// 中间件2: Panic 恢复
// ============================================================
// 关键点: 防止单个请求的panic导致整个服务崩溃
// 在生产环境中绝对不能没有这个中间件

func RecoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				// 记录错误和堆栈信息
				log.Printf("🔥 PANIC 恢复: %v\n堆栈:\n%s", err, debug.Stack())

				// 返回500给客户端
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Fprintf(w, `{"error": "internal server error"}`)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// ============================================================
// 中间件3: 限流 (基于令牌桶)
// ============================================================
// 关键点: 保护后端服务不被突发流量打垮
//   - 超过限制返回 429 Too Many Requests
//   - 生产中通常配合 Redis 做分布式限流

type rateLimiter struct {
	tokens   float64
	capacity float64
	rate     float64 // 每秒补充的令牌数
	lastTime time.Time
	mu       sync.Mutex
}

func newRateLimiter(rate float64, capacity float64) *rateLimiter {
	return &rateLimiter{
		tokens:   capacity,
		capacity: capacity,
		rate:     rate,
		lastTime: time.Now(),
	}
}

func (rl *rateLimiter) allow() bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(rl.lastTime).Seconds()
	rl.lastTime = now

	// 补充令牌
	rl.tokens += elapsed * rl.rate
	if rl.tokens > rl.capacity {
		rl.tokens = rl.capacity
	}

	if rl.tokens < 1 {
		return false
	}
	rl.tokens--
	return true
}

// RateLimitMiddleware 创建限流中间件
// rate: 每秒允许的请求数, capacity: 令牌桶容量(允许的突发量)
func RateLimitMiddleware(rate float64, capacity float64) Middleware {
	limiter := newRateLimiter(rate, capacity)
	// 注意:
	//   - limiter 实例会被该中间件包裹的所有请求共享（进程内共享限流器）。
	//   - 这种限流适合单实例或“每实例独立限流”的场景；多实例全局限流通常要上 Redis/网关。
	//   - limiter 内部用 mutex 保证并发安全。

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !limiter.allow() {
				// 关键点: 返回标准的429状态码
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Retry-After", "1") // 告诉客户端1秒后重试
				w.WriteHeader(http.StatusTooManyRequests)
				fmt.Fprintf(w, `{"error": "rate limit exceeded", "retry_after": "1s"}`)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
