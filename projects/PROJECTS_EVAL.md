# Projects Evaluation Pack

本文件用于“投喂给其他模型”来评审/完善项目，提供统一的信息结构。

## 目录

- `projects/observability-starter`
- `projects/mq-task-system`

---

## 评审目标

- 可运行性：是否一键启动、依赖是否齐全、README 是否清晰
- 面试价值：是否能支撑“中高级后端”的叙事（可靠性/观测/性能/治理）
- 真实性：是否有可验证产出（压测数据、dashboard、pprof 结论）
- 可扩展性：是否容易继续加功能（Jaeger tracing、DLQ 重放、延迟队列、k8s 部署等）

---

## 项目：observability-starter

### 目标

构建可复用的 Go 服务可观测性模板：

- Prometheus 指标：QPS/错误率/P95/P99
- 日志字段：trace_id 贯通
- pprof：CPU/heap/goroutine/mutex/block
- Grafana dashboard：自动导入

### 架构与模块

- `cmd/server`：HTTP 服务入口（Gin）
- `internal/middleware`：trace_id 与 access log
- `internal/metrics`：Prometheus 指标与中间件
- `docker-compose.yml`：Prometheus/Grafana

### 可演练用例

- `GET /api/slow?ms=xxx`：制造延迟上升，观察 P99
- `GET /api/error`：制造 5xx，观察错误率
- `GET /api/cpu?ms=xxx`：制造 CPU hotspot，用 pprof 定位

### 可改进点（给模型输出建议）

- 接入 OpenTelemetry + Jaeger（真正的 tracing）
- 增加指标维度：client、tenant、error_code（注意 label 基数）
- 增加告警规则示例（Prometheus alerting rules）

---

## 项目：mq-task-system

### 目标

构建 RabbitMQ 异步任务系统最小闭环：

- worker pool 并发消费
- 幂等（Redis SETNX + TTL）
- DLQ 重试
- Prometheus 指标 + Grafana dashboard

### 架构与模块

- `cmd/producer`：并发发布任务（使用 errgroup）
- `cmd/consumer`：worker pool 消费 + /metrics
- `internal/mq`：RabbitMQ 连接与声明
- `internal/dedup`：Redis 幂等
- `internal/metrics`：指标
- `docker-compose.yml`：RabbitMQ/Redis/Prometheus/Grafana

### 可演练用例

- 调低 `WORKERS` 制造积压，再提高并发恢复
- 在 `doWork()` 制造固定比例失败，观察 DLQ 与 retry 指标
- Redis 故障时的策略（当前实现：Nack requeue）

### 可改进点（给模型输出建议）

- 增加“DLQ 重放”命令：从 DLQ 重新投递回主队列
- 增加“最大重试次数”与退避策略（指数退避）
- 增加消息 schema 版本、任务状态落库
- 增加 k6 压测脚本（HTTP 控制面 + 指标验证）
