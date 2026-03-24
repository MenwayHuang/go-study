# observability-starter

一个可直接复用的 Go 服务可观测性模板：

- Gin HTTP 服务
- Prometheus 指标（QPS、错误率、P95/P99）
- 结构化日志（带 `trace_id`）
- `pprof` 性能定位入口
- Grafana 看板（Provisioning 自动导入）

## 运行

### 1) 启动依赖（Prometheus + Grafana）

```bash
docker compose up -d
```

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
- `GET /metrics`（Prometheus 抓取）
- `GET /debug/pprof/`（pprof）

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
- trace_id 贯通（日志字段规范）
- pprof：CPU/heap/goroutine/block/mutex 的定位路径
