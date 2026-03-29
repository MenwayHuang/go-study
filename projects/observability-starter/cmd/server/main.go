package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/gin"

	"go-study/projects/observability-starter/internal/metrics"
	"go-study/projects/observability-starter/internal/middleware"
	"go-study/projects/observability-starter/internal/tracing"
)

func main() {
	// 初始化 Jaeger tracing（通过 OpenTelemetry）
	jaegerEndpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if jaegerEndpoint == "" {
		jaegerEndpoint = "localhost:4318" // Jaeger OTLP HTTP 默认端口
	}
	shutdown, err := tracing.InitTracer(context.Background(), "observability-starter", jaegerEndpoint)
	if err != nil {
		log.Printf("WARN: tracing init failed (Jaeger may not be running): %v", err)
	} else {
		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = shutdown(ctx)
		}()
		log.Println("tracing initialized, exporting to Jaeger at", jaegerEndpoint)
	}

	r := gin.New()

	// 结构化日志（这里用最朴素的 log.Print，方便你后续替换 zap/zerolog）
	r.Use(gin.Recovery())
	r.Use(tracing.GinTracing()) // OpenTelemetry tracing 中间件（替代手动 TraceID）
	r.Use(middleware.TraceID()) // 保留兼容：如果 tracing 没初始化，仍用 UUID 生成 trace_id
	r.Use(middleware.AccessLog())
	r.Use(metrics.HTTPMetrics())

	// pprof
	pprof.Register(r)

	// Prometheus
	r.GET("/metrics", gin.WrapH(metrics.Handler()))

	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	api := r.Group("/api")
	{
		api.GET("/echo", func(c *gin.Context) {
			msg := c.Query("msg")
			if msg == "" {
				msg = "hello"
			}
			c.JSON(http.StatusOK, gin.H{"msg": msg})
		})

		// 模拟慢请求
		api.GET("/slow", func(c *gin.Context) {
			ms, _ := time.ParseDuration(c.DefaultQuery("ms", "200") + "ms")
			time.Sleep(ms)
			c.JSON(http.StatusOK, gin.H{"slept_ms": ms.Milliseconds()})
		})

		// 模拟 CPU hotspot
		api.GET("/cpu", func(c *gin.Context) {
			ms, _ := time.ParseDuration(c.DefaultQuery("ms", "200") + "ms")
			deadline := time.Now().Add(ms)
			x := 0
			for time.Now().Before(deadline) {
				// 做一些无意义计算，用来制造 CPU 火焰图热点
				x = (x*33 + 7) % 1000003
			}
			c.JSON(http.StatusOK, gin.H{"ok": true, "x": x})
		})

		// 一个演示 5xx 的接口
		api.GET("/error", func(c *gin.Context) {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "simulated"})
		})

		// 演示跨服务调用时 trace context 传播
		// 访问 /api/chain?target=http://localhost:8080/api/echo?msg=chained
		// 在 Jaeger 里可以看到两个 span：当前服务的 server span + 调用下游的 client span
		api.GET("/chain", func(c *gin.Context) {
			target := c.Query("target")
			if target == "" {
				target = "http://localhost:8080/healthz"
			}

			// 使用 TracedHTTPClient 发起请求
			// 它会自动把 traceparent Header 注入到请求里
			client := tracing.NewTracedHTTPClient()
			req, _ := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, target, nil)
			resp, err := client.Do(c.Request.Context(), req)
			if err != nil {
				c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("downstream call failed: %v", err)})
				return
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)

			c.JSON(http.StatusOK, gin.H{
				"downstream_status": resp.StatusCode,
				"downstream_body":   string(body),
			})
		})
	}

	srv := &http.Server{
		Addr:              ":8080",
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Println("observability-starter listening on :8080")
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
