package main

import (
	"log"
	"net/http"
	"time"

	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/gin"

	"go-study/projects/observability-starter/internal/metrics"
	"go-study/projects/observability-starter/internal/middleware"
)

func main() {
	r := gin.New()

	// 结构化日志（这里用最朴素的 log.Print，方便你后续替换 zap/zerolog）
	r.Use(gin.Recovery())
	r.Use(middleware.TraceID())
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
