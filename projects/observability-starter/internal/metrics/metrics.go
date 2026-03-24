package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	reqTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests.",
		},
		[]string{"method", "path", "status"},
	)

	reqDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request duration in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)
)

func init() {
	prometheus.MustRegister(reqTotal, reqDuration)
}

func Handler() http.Handler {
	return promhttp.Handler()
}

// HTTPMetrics 记录 QPS / 错误率 / 延迟直方图。
// 注意：这里的 path 用 gin 的 FullPath()，避免把 id 等动态参数打爆 label。
func HTTPMetrics() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		path := c.FullPath()
		if path == "" {
			path = "unknown"
		}

		status := strconv.Itoa(c.Writer.Status())
		method := c.Request.Method

		reqTotal.WithLabelValues(method, path, status).Inc()
		reqDuration.WithLabelValues(method, path).Observe(time.Since(start).Seconds())
	}
}
