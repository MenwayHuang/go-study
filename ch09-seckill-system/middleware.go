package main

import (
	"fmt"
	"log"
	"net/http"
	"runtime/debug"
	"sync"
	"time"
)

// ============================================================
// 秒杀系统中间件 (复用第3章的中间件模式)
// ============================================================

type middleware func(http.Handler) http.Handler

func chainMiddleware(handler http.Handler, middlewares ...middleware) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}
	return handler
}

// 日志中间件
type statusWriter struct {
	http.ResponseWriter
	statusCode int
}

func (sw *statusWriter) WriteHeader(code int) {
	sw.statusCode = code
	sw.ResponseWriter.WriteHeader(code)
}

func logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, statusCode: 200}
		next.ServeHTTP(sw, r)
		log.Printf("[%s] %s %d %v", r.Method, r.URL.Path, sw.statusCode, time.Since(start))
	})
}

// Panic恢复中间件
func recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				log.Printf("🔥 PANIC: %v\n%s", err, debug.Stack())
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// 令牌桶限流中间件
type tokenBucket struct {
	tokens   float64
	capacity float64
	rate     float64
	lastTime time.Time
	mu       sync.Mutex
}

func newTokenBucket(rate, capacity float64) *tokenBucket {
	return &tokenBucket{
		tokens:   capacity,
		capacity: capacity,
		rate:     rate,
		lastTime: time.Now(),
	}
}

func (tb *tokenBucket) allow() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()
	tb.tokens += now.Sub(tb.lastTime).Seconds() * tb.rate
	tb.lastTime = now
	if tb.tokens > tb.capacity {
		tb.tokens = tb.capacity
	}
	if tb.tokens < 1 {
		return false
	}
	tb.tokens--
	return true
}

func rateLimitMiddleware(rate, capacity float64) middleware {
	limiter := newTokenBucket(rate, capacity)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !limiter.allow() {
				w.Header().Set("Retry-After", "1")
				writeJSON(w, http.StatusTooManyRequests, map[string]string{
					"error": "系统繁忙，请稍后重试",
				})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// 模拟压测脚本提示
func init() {
	fmt.Println(`
  ============================================
  秒杀压测命令 (在另一个终端执行):

  # 1. 查询库存
  curl http://localhost:8080/seckill/stock?product_id=PROD-001

  # 2. 单次抢购
  curl -X POST http://localhost:8080/seckill/buy -H "Content-Type: application/json" -d "{\"product_id\":\"PROD-001\",\"user_id\":\"user-001\"}"

  # 3. 查看订单
  curl http://localhost:8080/seckill/orders

  # 4. 查看统计
  curl http://localhost:8080/seckill/stats
  ============================================`)
}
