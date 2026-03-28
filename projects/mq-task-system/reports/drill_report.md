# MQ 积压演练报告

- 日期：{date}
- 环境：本地 docker compose（RabbitMQ + Redis + Prometheus + Grafana）
- 目标：验证 worker pool 并发对吞吐与积压恢复的影响，形成可观测证据链

## 1. 场景与假设

- 场景：异步任务消费在高峰期出现积压
- 假设：提升 consumer 并发（workers/prefetch）后，消费吞吐提高，积压能在可控时间内恢复

## 2. 演练配置

- Producer：`-n {n} -concurrency {producer_concurrency}`
- Consumer（低并发）：`WORKERS={low_workers}, PREFETCH={low_prefetch}`
- Consumer（高并发）：`WORKERS={high_workers}, PREFETCH={high_prefetch}`

## 3. 观测口径

- 积压：`rabbitmq_queue_messages{queue="tasks.q"}`
- 吞吐：`sum(rate(task_consumed_total{result="ok"}[1m]))`
- 重试：`rate(task_retry_total[5m])`
- 延迟：P95/P99（`task_consume_duration_seconds`）

## 4. 结果

- 峰值积压（tasks.q）：{backlog_peak}
- 恢复时间（峰值→接近0）：{minutes} 分钟
- 峰值消费吞吐：{msg_per_sec} msg/s

## 5. 结论

- 在低并发下：积压持续上升，吞吐无法覆盖生产速率
- 在高并发下：吞吐显著提升，积压在 {minutes} 分钟内恢复

## 6. 截图证据

- Grafana：Backlog 曲线截图（粘贴链接或图片）
- Grafana：Consume QPS 曲线截图
- RabbitMQ 管理台队列截图

## 7. 可改进点

- 引入最大重试次数、指数退避、poison message 隔离
- 增加 DLQ 重放命令与审计
- 增加队列长度/积压告警（Alert rules）
