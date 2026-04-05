# mq-task-system

一个用于面试展示的 RabbitMQ 异步任务系统最小实现，包含：

- 生产者 Producer：发布任务
- 消费者 Consumer：worker pool 并发消费
- 幂等：Redis `SETNX` + TTL
- 重试：失败消息进入 DLQ（死信队列），并提供重放（replay）命令
- 可观测性：Prometheus 指标 + Grafana 看板（自动导入）

> 目标：你可以用它来讲清楚“at-least-once + 幂等 + 重试 + 积压治理 + 指标体系”。

## 运行

### 1) 启动依赖（RabbitMQ + Redis + Prometheus + Grafana）

```bash
docker compose up -d
```

- RabbitMQ 管理台: http://localhost:15672 （guest/guest）
- Grafana: http://localhost:3001 （admin/admin）
- Prometheus: http://localhost:9091

### 2) 启动 Consumer（会暴露 /metrics）

```bash
go run ./cmd/consumer
```

- metrics: http://localhost:8081/metrics

### 3) 发送任务（Producer）

```bash
go run ./cmd/producer -n 1000 -concurrency 10
```

### 4) DLQ 重放（可选）

将 `tasks.dlq` 中的消息重新投递回 `tasks.q`：

```bash
go run ./cmd/dlq-replay -n 200
```

### 5) 积压演练（建议必做）

见 `DRILL.md`，会指导你：

- 低并发启动 consumer 制造积压
- 提升并发恢复
- 产出 Grafana 曲线截图 + 演练报告

## 队列/交换机设计

- Exchange: `tasks.ex`（direct）
- Queue: `tasks.q`
- DLQ: `tasks.dlq`（`tasks.q` 的死信队列）

## 幂等口径（Redis）

- key: `task:dedup:{task_id}`
- 使用 `SETNX` 写入，写入成功才执行任务
- TTL 用于防止 key 永久占用

## 指标（示例）

- `task_published_total`
- `task_consumed_total{result="ok|error|dedup"}`
- `task_consume_duration_seconds`（histogram，可算 P95/P99）
- `task_retry_total`

## 面试可讲的关键点

- at-least-once 与幂等之间的关系
- ack 时机：什么时候 ack，什么时候 nack/reject
- poison message 处理：最大重试次数、DLQ、人工介入
- 积压治理：提升 consumer 并发、水平扩容、限流与降级
- 指标口径：吞吐、延迟分位数、失败率、重试次数
