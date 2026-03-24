package middleware

import (
	"log"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	TraceIDHeader = "X-Trace-Id"
	TraceIDKey    = "trace_id"
)

// TraceID 负责生成/透传 trace_id，并写入 response header。
// 这不是 Jaeger 的 tracing，但它能满足：日志字段贯通、排障快速定位。
func TraceID() gin.HandlerFunc {
	return func(c *gin.Context) {
		traceID := c.GetHeader(TraceIDHeader)
		if traceID == "" {
			traceID = uuid.NewString()
		}
		c.Set(TraceIDKey, traceID)
		c.Writer.Header().Set(TraceIDHeader, traceID)
		c.Next()
	}
}

// AccessLog 是一个最小 access log 示例，带 trace_id / latency / status。
func AccessLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		lat := time.Since(start)
		traceID, _ := c.Get(TraceIDKey)
		log.Printf("trace_id=%v method=%s path=%s status=%d latency_ms=%d", traceID, c.Request.Method, c.FullPath(), c.Writer.Status(), lat.Milliseconds())
	}
}
