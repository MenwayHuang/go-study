# observability-starter

一个可直接复用的 Go 服务可观测性模板：

- Gin HTTP 服务
- **Jaeger 分布式链路追踪**（OpenTelemetry + OTLP）
- Prometheus 指标（QPS、错误率、P95/P99）
- 结构化日志（带 `trace_id`）
- **跨服务 trace context 传播**（traceparent Header）
- `pprof` 性能定位入口
- Grafana 看板（Provisioning 自动导入）

## 运行

### 1) 启动依赖（Jaeger + Prometheus + Grafana）

```bash
docker compose up -d
```

- **Jaeger UI**: http://localhost:16686 （查看链路追踪）
- Grafana: http://localhost:3000 （默认账号密码：admin/admin）
- Prometheus: http://localhost:9090

### 2) 启动服务

```bash
go run ./cmd/server
```

服务监听： http://localhost:8080

## 接口

- `GET /healthz`
- `GET /api/echo?msg=hello`
- `GET /api/slow?ms=200`（模拟慢请求）
- `GET /api/cpu?ms=200`（模拟 CPU hotspot）
- `GET /api/error`（模拟 5xx 错误）
- `GET /api/chain?target=URL`（演示跨服务调用的 trace 传播）
- `GET /metrics`（Prometheus 抓取）
- `GET /debug/pprof/`（pprof）

## Jaeger 链路追踪

启动服务后，访问任意接口，然后打开 Jaeger UI http://localhost:16686 ：

1. 在 Service 下拉选择 `observability-starter`
2. 点击 `Find Traces`
3. 点击某个 trace 可以看到完整的 span 树

### 演示跨服务传播

```bash
curl "http://localhost:8080/api/chain?target=http://localhost:8080/api/echo?msg=chained"
```

在 Jaeger 里会看到一个 trace 包含两个 span：
- `GET /api/chain`（server span）
- `HTTP GET /api/echo`（client span，调用下游）

这就是分布式链路追踪的核心：**trace_id 和 span_id 通过 HTTP Header `traceparent` 传递给下游服务**。

## 常用 PromQL（在 Grafana Explore 里可直接跑）

### QPS

```promql
sum(rate(http_requests_total[1m]))
```

### 错误率（5xx 占比）

```promql
sum(rate(http_requests_total{status=~"5.."}[5m])) / sum(rate(http_requests_total[5m]))
```

### P99

```promql
histogram_quantile(0.99, sum by (le) (rate(http_request_duration_seconds_bucket[5m])))
```

## 压测

你可以用 `k6` 或 `wrk`：

- `wrk -t4 -c50 -d30s http://localhost:8080/api/echo?msg=hello`

## 你可以在面试里讲的点

- 指标口径：QPS/错误率/P95/P99
- 直方图 `bucket` 的使用与 `histogram_quantile`
- **分布式链路追踪**：OpenTelemetry + Jaeger，trace context 通过 W3C traceparent Header 跨服务传播
- trace_id 贯通（日志字段规范）
- pprof：CPU/heap/goroutine/block/mutex 的定位路径
