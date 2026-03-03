package main

import (
	"context"
	"fmt"
	"math/rand"
	"time"
)

// ============================================================
// 简易链路追踪 (Distributed Tracing)
// ============================================================
// 关键点:
//   - TraceID: 一次完整请求的唯一标识（从入口到最终响应）
//   - SpanID:  每个服务/操作的唯一标识
//   - ParentSpanID: 父操作的SpanID，形成调用树
//
// 实际生产中使用: OpenTelemetry + Jaeger/Zipkin
// 这里用纯Go实现核心原理

type Span struct {
	TraceID      string
	SpanID       string
	ParentSpanID string
	ServiceName  string
	OperationName string
	StartTime    time.Time
	EndTime      time.Time
	Tags         map[string]string
}

func (s *Span) Duration() time.Duration {
	return s.EndTime.Sub(s.StartTime)
}

type traceKey struct{}

// Tracer 简易追踪器
type Tracer struct {
	spans []*Span // 收集所有span（生产中应异步发送到Jaeger）
}

func NewTracer() *Tracer {
	return &Tracer{}
}

func (t *Tracer) StartSpan(ctx context.Context, service, operation string) (context.Context, *Span) {
	span := &Span{
		SpanID:        generateID(),
		ServiceName:   service,
		OperationName: operation,
		StartTime:     time.Now(),
		Tags:          make(map[string]string),
	}

	// 从context中获取父span
	if parent, ok := ctx.Value(traceKey{}).(*Span); ok {
		span.TraceID = parent.TraceID
		span.ParentSpanID = parent.SpanID
	} else {
		span.TraceID = generateID() // 新的trace
	}

	t.spans = append(t.spans, span)
	return context.WithValue(ctx, traceKey{}, span), span
}

func (t *Tracer) FinishSpan(span *Span) {
	span.EndTime = time.Now()
}

func (t *Tracer) PrintTrace() {
	if len(t.spans) == 0 {
		return
	}
	traceID := t.spans[0].TraceID
	fmt.Printf("\n  === 链路追踪 TraceID: %s ===\n", traceID)
	for _, span := range t.spans {
		indent := "  "
		if span.ParentSpanID != "" {
			indent = "    "
		}
		fmt.Printf("%s[%s] %s.%s  耗时: %v\n",
			indent, span.SpanID[:8], span.ServiceName, span.OperationName, span.Duration())
		for k, v := range span.Tags {
			fmt.Printf("%s  tag: %s=%s\n", indent, k, v)
		}
	}
}

func generateID() string {
	return fmt.Sprintf("%016x", rand.Int63())
}

// ============================================================
// 演示: 模拟微服务调用链
// ============================================================
// 调用链:
//   API Gateway → 用户服务 → 订单服务 → 库存服务
//                                    → 支付服务

func RunTraceDemo() {
	fmt.Println("--- 示例: 链路追踪 — 追踪一次完整的微服务调用 ---")
	fmt.Println("  调用链: Gateway → UserService → OrderService → (StockService + PayService)")

	tracer := NewTracer()
	ctx := context.Background()

	// API Gateway
	ctx, gwSpan := tracer.StartSpan(ctx, "api-gateway", "HandleRequest")
	gwSpan.Tags["http.method"] = "POST"
	gwSpan.Tags["http.url"] = "/api/v1/orders"

	// 调用用户服务
	ctx2, userSpan := tracer.StartSpan(ctx, "user-service", "GetUserInfo")
	time.Sleep(20 * time.Millisecond)
	userSpan.Tags["user.id"] = "12345"
	tracer.FinishSpan(userSpan)

	// 调用订单服务
	ctx3, orderSpan := tracer.StartSpan(ctx2, "order-service", "CreateOrder")
	time.Sleep(30 * time.Millisecond)

	// 订单服务内部并行调用: 库存服务 + 支付服务
	stockDone := make(chan struct{})
	payDone := make(chan struct{})

	go func() {
		_, stockSpan := tracer.StartSpan(ctx3, "stock-service", "DeductStock")
		time.Sleep(25 * time.Millisecond)
		stockSpan.Tags["product.id"] = "SKU-001"
		stockSpan.Tags["quantity"] = "1"
		tracer.FinishSpan(stockSpan)
		close(stockDone)
	}()

	go func() {
		_, paySpan := tracer.StartSpan(ctx3, "pay-service", "ProcessPayment")
		time.Sleep(40 * time.Millisecond)
		paySpan.Tags["amount"] = "99.99"
		paySpan.Tags["method"] = "alipay"
		tracer.FinishSpan(paySpan)
		close(payDone)
	}()

	<-stockDone
	<-payDone
	orderSpan.Tags["order.id"] = "ORD-20240101"
	tracer.FinishSpan(orderSpan)

	gwSpan.Tags["http.status"] = "200"
	tracer.FinishSpan(gwSpan)

	// 打印完整链路
	tracer.PrintTrace()

	fmt.Println("\n  === 链路追踪的价值 ===")
	fmt.Println("  1. 定位慢请求: 一眼看出哪个服务/操作最耗时")
	fmt.Println("  2. 排查故障: 请求经过了哪些服务，在哪里失败")
	fmt.Println("  3. 优化依据: 哪些调用可以并行化")
	fmt.Println("  4. 生产工具: OpenTelemetry + Jaeger/Zipkin")
}
