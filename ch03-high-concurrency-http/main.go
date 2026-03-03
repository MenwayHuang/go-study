package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	// 创建路由
	mux := http.NewServeMux()

	// 注册业务Handler
	mux.HandleFunc("/api/hello", helloHandler)
	mux.HandleFunc("/api/slow", slowHandler)   // 模拟慢请求
	mux.HandleFunc("/api/panic", panicHandler)  // 模拟panic

	// 关键点: 中间件链 — 从外到内包裹Handler
	// 执行顺序: Logger → Recovery → RateLimit → Handler
	handler := ChainMiddleware(mux,
		LoggerMiddleware,
		RecoveryMiddleware,
		RateLimitMiddleware(10, 20), // 每秒10个请求，桶容量20
	)

	// 关键点: 生产级HTTP Server配置
	server := &http.Server{
		Addr:              ":8080",
		Handler:           handler,
		ReadTimeout:       5 * time.Second,   // 读取请求超时
		WriteTimeout:      10 * time.Second,  // 写入响应超时
		IdleTimeout:       120 * time.Second, // Keep-Alive空闲超时
		ReadHeaderTimeout: 2 * time.Second,   // 读取请求头超时
		MaxHeaderBytes:    1 << 20,           // 1MB 请求头大小限制
	}

	// 关键点: 优雅关闭 — 在独立goroutine中启动server
	go func() {
		fmt.Println("🚀 服务启动: http://localhost:8080")
		fmt.Println("   试试: GET /api/hello")
		fmt.Println("   试试: GET /api/slow (模拟慢请求)")
		fmt.Println("   按 Ctrl+C 测试优雅关闭")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("服务启动失败: %v", err)
		}
	}()

	// 关键点: 监听系统信号实现优雅关闭
	gracefulShutdown(server)
}

// gracefulShutdown 优雅关闭
// 关键点:
//  1. 监听 SIGINT (Ctrl+C) 和 SIGTERM (docker stop / k8s)
//  2. 收到信号后停止接受新连接
//  3. 等待已有请求处理完成（最多等30秒）
//  4. 超时则强制关闭
func gracefulShutdown(server *http.Server) {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	sig := <-quit // 阻塞等待信号
	fmt.Printf("\n📛 收到信号: %v, 开始优雅关闭...\n", sig)

	// 关键点: 给已有请求最多30秒的时间完成
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Shutdown 会:
	// 1. 关闭所有空闲连接
	// 2. 等待活跃请求处理完成
	// 3. 超时后返回错误
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("优雅关闭失败: %v, 强制退出", err)
	}

	fmt.Println("✅ 服务已安全关闭")
}

// ============================================================
// 业务 Handler
// ============================================================

func helloHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"message": "hello", "time": "%s"}`, time.Now().Format(time.RFC3339))
}

func slowHandler(w http.ResponseWriter, r *http.Request) {
	// 模拟慢请求 — 用来测试优雅关闭
	// 关键点: 使用 request context 检测客户端是否断开
	select {
	case <-time.After(3 * time.Second):
		fmt.Fprintf(w, `{"message": "slow response done"}`)
	case <-r.Context().Done():
		// 客户端断开或服务关闭，停止处理
		log.Println("客户端断开连接，停止处理")
		return
	}
}

func panicHandler(w http.ResponseWriter, r *http.Request) {
	panic("模拟panic! 看Recovery中间件能否捕获")
}
