# 演练：制造积压 → 恢复时间 → 出报告

本演练目标：让你能真实填充简历里的 `{msg_per_sec}/{backlog}/{minutes}`，并形成可截图的证据链（Grafana曲线 + RabbitMQ面板 + 报告）。

---

## 先记住一条最重要的判断规则

看监控时，先问自己：**我现在想看的是“存量”还是“变化速度”？**

- **存量 / 当前还有多少** → 看 **数量**
- **单位时间内变化多快** → 看 **速率**

### 1. 什么叫“数量”

例如：

- 队列里现在还有多少条消息
- retry queue 现在积压多少条
- 当前熔断器是不是打开

这些问题本质上都在问：

```text
现在这个时刻，系统里还剩多少 / 处于什么状态
```

所以要看：

- `rabbitmq_queue_messages{queue="tasks.q"}`
- `rabbitmq_queue_messages{queue="tasks.retry.q"}`
- `task_circuit_open`

### 2. 什么叫“速率”

例如：

- 每秒成功消费多少条
- 每分钟进入 DLQ 多快
- 每分钟发生 retry 多频繁

这些问题本质上都在问：

```text
最近一小段时间里，这个累计值增长得有多快
```

所以要看：

- `sum(rate(task_consumed_total{result="ok"}[10s]))`
- `sum(rate(task_retry_scheduled_total[1m]))`
- `sum(rate(task_dlq_total[1m]))`

### 3. 为什么 QPS 看 rate，而 backlog 看数量？

因为它们问的根本不是同一个问题：

- **QPS** 问的是：每秒处理多少条
- **Backlog** 问的是：现在还压了多少条

你可以这样记：

```text
吞吐 / 流量 / 失败速度 -> 看 rate
积压 / 当前状态 / 当前深度 -> 看 gauge 数量
```

### 4. 为什么 DLQ 常看 rate 或 increase？

因为你通常不是只关心“历史上总共进过多少条 DLQ”。

你更关心的是：

- 最近 1 分钟是不是突然大量进 DLQ
- 最近 5 分钟是否出现异常失败高峰

所以常见写法是：

- `sum(rate(task_dlq_total[1m]))`
- `sum(increase(task_dlq_total[5m]))`

区别是：

- `rate` 更像“当前速度”
- `increase` 更像“这段时间总共增加了多少”

## 你会得到的产出物

- 1 张 Grafana 截图：
  - `Backlog (Queue Depth)`：`tasks.q` 从上升到回落
  - `Consume QPS`：并发提升后吞吐上升
- 1 张 RabbitMQ 管理台截图：队列深度变化
- 1 份报告：`reports/drill_report.md`

## 前置

在 `projects/mq-task-system` 目录：

1. 启动依赖

```bash
docker compose up -d
```

2. 启动 consumer（低并发）

Windows PowerShell 示例：

```powershell
$env:WORKERS="2"
$env:PREFETCH="2"
go run ./cmd/consumer
```

3. 发送大量任务

```bash
go run ./cmd/producer -n 5000 -concurrency 20
```

---

## RabbitMQ 管理台和 Grafana，各看什么？

这两个页面不要混着看，它们的职责不同。

### RabbitMQ 管理台适合看

- 队列名字是不是对
- 每个队列当前还有多少消息
- 哪个队列有 consumer
- Ready / Unacked 大概是什么情况

你把它理解成：

```text
RabbitMQ 官方后台，偏“MQ 自己的状态”
```

### Grafana 适合看

- 趋势
- 速率
- 延迟
- backlog 是上升还是下降
- retry / DLQ 是不是出现尖峰
- 熔断和限流有没有触发

你把它理解成：

```text
把多个指标组合成“系统行为曲线”
```

### 为什么 RabbitMQ 里好像只能看到所有 message 折线，不是按 MQ 区分？

因为 RabbitMQ 管理台的 Overview 页面通常更偏全局统计。

你要看具体某个队列，应该去：

- `Queues`
- 点进 `tasks.q`
- 点进 `tasks.retry.q`
- 点进 `tasks.dlq`

这样你才是在“按队列”看。

Grafana 这边之所以能按队列区分，是因为我们打到 Prometheus 的指标里带了标签：

```promql
rabbitmq_queue_messages{queue="tasks.q"}
rabbitmq_queue_messages{queue="tasks.retry.q"}
rabbitmq_queue_messages{queue="tasks.dlq"}
```

---

## 你应该重点认识哪些指标

### 1. `task_consumed_total{result="ok"}`

含义：

- 成功消费总数
- 是 Counter，只会越来越大

常见看法：

- `sum(rate(task_consumed_total{result="ok"}[10s]))` → 成功消费 QPS

### 2. `task_consumed_total{result="error"}`

含义：

- 失败消费总数

常见看法：

- `sum(rate(task_consumed_total{result="error"}[1m]))` → 最近失败速率
- `sum(increase(task_consumed_total{result="error"}[5m]))` → 最近 5 分钟失败总量

### 3. `task_consume_duration_seconds`

含义：

- 单条消息处理耗时分布

常见看法：

- `P95`
- `P99`

这类图回答的问题是：

```text
消息处理是不是变慢了
```

### 4. `rabbitmq_queue_messages{queue="..."}`

含义：

- 某个队列当前还剩多少消息

常见看法：

- 看 `tasks.q` → 主队列积压
- 看 `tasks.retry.q` → 延迟重试堆积
- 看 `tasks.dlq` → 死信队列当前存量

### 5. `task_retry_scheduled_total`

含义：

- 被安排进入 retry queue 的累计次数

常见看法：

- `sum(rate(task_retry_scheduled_total[1m]))`

回答的问题：

```text
最近系统有多频繁地在重试
```

### 6. `task_dlq_total`

含义：

- 最终进入 DLQ 的累计次数

常见看法：

- `sum(rate(task_dlq_total[1m]))`
- `sum(increase(task_dlq_total[5m]))`

回答的问题：

```text
最近有没有消息彻底失败
```

### 7. `task_rate_limit_wait_total`

含义：

- 有多少条消息在真正开始处理前被限流器阻塞等待过

回答的问题：

```text
当前消费能力是不是被限流主动压住了
```

### 8. `task_circuit_open`

含义：

- 熔断器当前状态
- `1` = open
- `0` = closed

回答的问题：

```text
当前是不是已经进入“先别打下游”的保护状态
```

## 观察积压

- 打开 RabbitMQ 管理台： http://localhost:15672
- 查看 `tasks.q` 的 `Messages` 是否持续上升
- 打开 Grafana： http://localhost:3000
  - Dashboard: `mq-task-system`
  - 看 `Backlog (Queue Depth)` 面板，`tasks.q` 应该上升

记录此时的峰值积压：`{backlog_peak}`

---

## 实验 1：先理解“数量”和“QPS”不是一回事

### 目标

理解：

- 为什么 `Backlog` 看数量
- 为什么 `Consume QPS` 看 rate

### 操作

1. 启动 consumer，使用较低并发：

```bash
WORKERS=2 PREFETCH=2 go run ./cmd/consumer
```

2. 另开一个终端发送大量任务：

```bash
go run ./cmd/producer -n 2000 -concurrency 20
```

### 你要看什么

#### 在 RabbitMQ 管理台

- `tasks.q` 当前消息数是不是上涨

#### 在 Grafana

- `Backlog (Queue Depth)`：应当明显上涨
- `Consume QPS`：会有一条相对平稳的处理速率

### 你应该怎么理解

- `Backlog` 上涨，说明“来得比处理得快”
- `QPS` 不是 0，说明 consumer 不是没工作，而是“处理速度不够快”

这两个图一起看，你才知道问题在哪。

---

## 实验 2：提升并发，观察 backlog 回落

### 操作

停止原 consumer，重新启动：

```bash
WORKERS=20 PREFETCH=20 go run ./cmd/consumer
```

### 你要看什么

- `Consume QPS` 应该上升
- `Backlog (Queue Depth)` 应该下降

### 你应该怎么理解

这说明：

```text
不是系统彻底坏了，而是消费能力不足导致积压
```

---

## 实验 3：制造失败，观察 retry 和 DLQ

### 目标

理解：

- retry queue 看的是“当前积压数量”
- retry / dlq total 看的是“最近失败流量”

### 操作

发送会失败的消息。

如果 producer 目前没有直接传 `payload=fail` 的参数，就手工构造或临时改造一小批失败消息；因为 `consumer.go` 里 `payload=fail` 会触发模拟失败。

### 你要看什么

#### RabbitMQ 管理台

- `tasks.retry.q` 是否先出现消息
- 若超过最大重试次数，`tasks.dlq` 是否出现消息

#### Grafana

- `rabbitmq_queue_messages{queue="tasks.retry.q"}` 对应的 backlog 是否上升
- `Retries` 面板是否出现速率
- `DLQ` 面板是否出现速率或增量

### 你应该怎么理解

- `tasks.retry.q` 的**数量**表示“当前有多少消息正在等待下次重试”
- `task_retry_scheduled_total` 的 **rate** 表示“最近单位时间内有多频繁地发生重试”
- `task_dlq_total` 的 **rate/increase** 表示“最近有多少消息彻底失败了”

所以：

- **队列 backlog** 看当前数量
- **失败流量** 看 rate / increase

---

## 实验 4：打开限流，观察为什么 backlog 会涨

### 操作

用较高 worker，但故意限制每秒处理速率：

```bash
WORKERS=20 PREFETCH=20 RATE_LIMIT_PER_SEC=5 go run ./cmd/consumer
```

然后发送任务：

```bash
go run ./cmd/producer -n 500 -concurrency 20
```

### 你要看什么

- `Consume QPS` 会被压在一个较低水平
- `Backlog` 容易上涨
- `task_rate_limit_wait_total` 会持续增长

### 你应该怎么理解

这时 backlog 不是因为 RabbitMQ 出故障，而是因为：

```text
consumer 主动把自己处理速度压低了
```

---

## 实验 5：触发熔断

### 操作

让失败连续发生，达到 `CircuitThreshold`。

### 你要看什么

- `task_circuit_open` 是否变成 1
- retry 指标是否上升

### 你应该怎么理解

熔断打开后，consumer 不是完全不收消息，而是：

```text
先别继续执行业务处理，把消息延后再试
```

---

## 为什么 RabbitMQ 里会一直残留几十条消息？

这是你现在最容易疑惑的现象，常见原因有 5 类。

### 1. 这些消息其实在 `retry queue`

最常见。

消息失败后被放进 `tasks.retry.q`，它不会立刻被消费，而是等 TTL 到期后再回到主队列。

所以你会看到：

- 主队列快空了
- 但 retry queue 里还有几十条

### 2. 这些消息已经进了 `DLQ`

也就是：

- 它们不是“没被处理”
- 而是“处理了很多次，最终失败，被放弃到死信队列里”

### 3. 有 `Unacked` 消息

可能 consumer 已经拿到消息了，但还没 ack。

典型情况：

- 正在处理
- 处理很慢
- consumer 异常退出前还没 ack

这时你要在 RabbitMQ 队列详情里区分：

- `Ready`
- `Unacked`

### 4. consumer 速率太低

例如：

- `WORKERS` 太小
- `PREFETCH` 太低
- 打开了 `RATE_LIMIT_PER_SEC`

这时系统不是“不能消费”，只是“消费不过来”。

### 5. 失败消息在反复重试

如果消息内容本身有问题，或者下游一直异常，它会不断进入：

```text
主队列 -> 重试队列 -> 主队列 -> 重试队列
```

直到超过最大重试次数再进 DLQ。

所以短时间内你会觉得：

```text
怎么老有几十条一直在那里
```

其实它们是在“来回折返”。

## 恢复：提升 consumer 并发

停止 consumer（Ctrl+C），然后以更高并发启动：

```powershell
$env:WORKERS="20"
$env:PREFETCH="20"
go run ./cmd/consumer
```

观察：

- `Consume QPS` 上升
- `Backlog (Queue Depth)` 开始回落

记录恢复时间：

- 从积压峰值开始到 `tasks.q` 回落到接近 0 的时间差：`{minutes}`

---

## 一个最实用的观测顺序

以后你看到“好像有问题”，按这个顺序看：

1. **RabbitMQ 管理台看队列名和 Ready / Unacked**
2. **Grafana 看 `tasks.q` backlog 是涨还是跌**
3. **Grafana 看 `Consume QPS` 有没有值**
4. **Grafana 看 `tasks.retry.q` / `task_retry_scheduled_total`**
5. **Grafana 看 `task_dlq_total`**
6. **Grafana 看 `task_circuit_open` 和 `task_rate_limit_wait_total`**

如果你按这个顺序看，基本就不会乱。

## 报告模板

将你的数据填入：`reports/drill_report.md`
