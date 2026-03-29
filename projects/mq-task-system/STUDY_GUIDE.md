# MQ Task System 学习指南

> 本文档回答你在学习过程中的所有疑问，从最基础的概念到如何写进简历。

---

## 一、YAML / YML 是什么？怎么读？

### 读音

- **YAML** 读作 **"雅猫"**（/ˈjæməl/），像英文 "camel" 把 c 换成 y
- `.yml` 和 `.yaml` 是**完全相同的格式**，只是后缀名长短不同，没有任何区别

### 本质

YAML 就是一种**配置文件格式**，和 JSON 作用一样，但更适合人类阅读。对比一下：

```json
{"name": "tom", "age": 18, "hobbies": ["coding", "gaming"]}
```

```yaml
name: tom
age: 18
hobbies:
  - coding
  - gaming
```

规则很简单：
- **缩进表示层级**（必须用空格，不能用 Tab）
- `key: value` 表示键值对
- `- item` 表示列表项

### prometheus.yml 逐行解读

```yaml
global:
  scrape_interval: 5s          # 每 5 秒去抓取一次指标数据

scrape_configs:                # 抓取配置列表
  - job_name: "mq_task_consumer"   # 给这个抓取任务起个名字
    metrics_path: /metrics         # 去哪个路径抓（你的 Go 程序暴露的）
    static_configs:
      - targets: ["host.docker.internal:8081"]  # 目标地址
        # host.docker.internal 是 Docker 容器访问宿主机的特殊域名
        # 8081 是你的 consumer 程序监听的端口
```

翻译成人话：**Prometheus 每 5 秒去访问一次 `http://你的电脑:8081/metrics`，把里面的数字抓回来存起来。**

### docker-compose.yml 里的 `:ro` 是什么意思？

```yaml
volumes:
  - ./prometheus/prometheus.yml:/etc/prometheus/prometheus.yml:ro
  #   本机文件路径                  容器内路径                    :ro
```

- `volumes` 是**文件挂载**：把你电脑上的文件"映射"到 Docker 容器里
- `:ro` = **read-only**（只读），容器只能读这个文件，不能修改它
- 如果不加 `:ro`，容器里的程序可以修改这个文件，改动会同步回你的电脑

---

## 二、Prometheus 指标类型详解

### 1. Counter（计数器）— 只增不减

```go
TaskPublishedTotal = prometheus.NewCounter(
    prometheus.CounterOpts{Name: "task_published_total", Help: "Total tasks published."},
)
```

**生活类比：汽车里程表。** 只会往上涨，永远不会倒退。

- 每发一条消息，`TaskPublishedTotal.Inc()` 加 1
- Prometheus 抓到的是累计值，比如 0 → 100 → 250 → 500
- Grafana 用 `rate()` 函数算出"每秒增加多少"，就是 QPS

### 2. CounterVec（带标签的计数器）— NewCounter vs NewCounterVec 的区别

```go
TaskConsumedTotal = prometheus.NewCounterVec(
    prometheus.CounterOpts{Name: "task_consumed_total", Help: "Total tasks consumed."},
    []string{"result"},  // ← 这是标签（label）
)
```

- `NewCounter`：只有一个数字，比如"总共发了多少条消息"
- `NewCounterVec`：**按标签分类**，变成多个数字

```
task_consumed_total{result="ok"}    → 成功消费了多少条
task_consumed_total{result="error"} → 失败了多少条
task_consumed_total{result="dedup"} → 重复消息跳过了多少条
```

**类比：`NewCounter` 是一个计数器，`NewCounterVec` 是一排计数器，每个标签值一个。**

代码中使用时要指定标签值：
```go
metrics.TaskConsumedTotal.WithLabelValues("ok").Inc()    // ok 的计数器 +1
metrics.TaskConsumedTotal.WithLabelValues("error").Inc() // error 的计数器 +1
```

### 3. Histogram（直方图）— 用来算 P95 / P99

```go
TaskConsumeDuration = prometheus.NewHistogram(
    prometheus.HistogramOpts{
        Name:    "task_consume_duration_seconds",
        Help:    "Task consume duration in seconds.",
        Buckets: prometheus.DefBuckets,
    },
)
```

**为什么需要 Histogram？** 如果只记录"平均耗时"，你看不到慢请求。比如：
- 99 个请求 1ms，1 个请求 10s → 平均 101ms，看起来还行
- 但那 1 个 10s 的请求，用户已经骂娘了

Histogram 的工作方式是把耗时分到**桶（Bucket）** 里：

```
Buckets: [0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10]
         5ms   10ms  25ms   50ms 100ms 250ms 500ms 1s  2.5s  5s  10s
```

每次 `.Observe(耗时)` 时，所有 ≥ 该耗时的桶都 +1。最终 Prometheus 可以算出：
- **P95**：95% 的请求在多少秒内完成
- **P99**：99% 的请求在多少秒内完成

**Buckets 的作用**：定义"桶的边界"。`prometheus.DefBuckets` 是默认值，适合大多数场景。如果你的业务耗时特别短（微秒级）或特别长（分钟级），可以自定义。

### 4. GaugeVec（仪表盘 — 可增可减）

```go
QueueMessages = prometheus.NewGaugeVec(
    prometheus.GaugeOpts{Name: "rabbitmq_queue_messages", Help: "RabbitMQ queue messages (depth)."},
    []string{"queue"},
)
```

**类比：温度计或油量表。** 值可以上下波动。

- Counter 只能涨（消息总数）
- Gauge 可以涨也可以跌（队列里当前有多少条消息）

`Vec` 同理：按 `queue` 标签分类，`tasks.q` 一个值，`tasks.dlq` 一个值。

### 指标类型总结

| 类型 | 特点 | 使用场景 | 类比 |
|------|------|----------|------|
| **Counter** | 只增不减 | 请求总数、消息发送总数 | 里程表 |
| **CounterVec** | 按标签分类的 Counter | 按状态码/结果分类的请求数 | 一排里程表 |
| **Gauge** | 可增可减 | 队列深度、内存使用量、goroutine 数 | 温度计 |
| **GaugeVec** | 按标签分类的 Gauge | 多个队列各自的深度 | 一排温度计 |
| **Histogram** | 分桶统计分布 | 请求延迟（可算 P95/P99） | 考试成绩分数段统计 |

---

## 三、后端需要懂 Prometheus 查询语句吗？

**需要，但不用一上来就学得很深。**

你至少要懂 3 件事：

- 你的代码暴露出来的指标名是什么
- 这些指标属于 Counter / Gauge / Histogram 哪一类
- Grafana 面板里的 `expr` 大概在算什么

原因很简单：

- 运维知道怎么部署 Prometheus / Grafana
- 但只有后端最清楚“什么指标才真正能反映业务健康”

例如这个项目里：

- `task_consumed_total{result="ok"}` 代表成功消费总数
- `rabbitmq_queue_messages{queue="tasks.q"}` 代表主队列积压
- `task_retry_scheduled_total` 代表进入延迟重试的次数

如果后端完全不懂这些查询，就很难回答：

- 为什么 QPS 是 0
- 为什么 backlog 在涨
- 为什么 DLQ 在增加

### 你现在最容易混淆的这条语句，拆开看

```promql
sum(rate(task_consumed_total{result="ok"}[10s]))
```

不要把它一次性理解成一大串，拆成 4 层：

#### 第 1 层：`task_consumed_total{result="ok"}`

意思是：

- 找到指标 `task_consumed_total`
- 只取 `result="ok"` 的那部分时间序列

你可以把它理解成：

```text
只看“成功消费”这个计数器
```

假设 Prometheus 最近抓到的数据是：

```text
12:00:00 -> 1000
12:00:05 -> 1100
12:00:10 -> 1200
```

这表示：

- 到 12:00:00 为止，累计成功消费 1000 条
- 到 12:00:10 为止，累计成功消费 1200 条

#### 第 2 层：`rate(...[10s])`

意思是：

- 看最近 10 秒内，这个 Counter 增长得有多快
- 结果是“每秒平均增长多少”

沿用上面的例子：

- 10 秒内从 `1000` 增长到 `1200`
- 一共增加了 `200`
- 平均每秒就是 `20`

所以：

```text
rate(task_consumed_total{result="ok"}[10s]) ≈ 20
```

这就是为什么 `rate(counter[窗口])` 经常被用来算 QPS。

#### 第 3 层：`sum(...)`

为什么还要再 `sum`？

因为 Prometheus 里一个指标可能不止一条时间序列。

例如以后你加了：

- 多个 consumer 实例
- 不同 worker 标签
- 不同 pod 标签

那么 `task_consumed_total{result="ok"}` 可能会有多条线。

`sum(...)` 的意思就是：

```text
把这些“成功消费速率”全部加起来，得到系统总吞吐
```

#### 第 4 层：最终含义

所以这条语句完整翻译成人话就是：

```text
统计最近 10 秒里，所有 consumer 每秒平均成功消费了多少条消息
```

这就是 `Consume QPS`。

### 你写 PromQL 时的思考模板

以后你看到任何 `expr`，都按这个顺序看：

1. **数据源是谁**
   - 例如 `task_consumed_total`

2. **筛选了谁**
   - 例如 `{result="ok"}`

3. **做了什么计算**
   - `rate`、`increase`、`histogram_quantile`

4. **最终想看什么业务问题**
   - 吞吐？积压？失败？时延？

---

## 四、怎么设计 PromQL，不再混沌？

你可以按“业务问题 -> 指标类型 -> 查询写法”的顺序设计。

### 1. 如果你想看吞吐

问题：

```text
每秒成功消费多少条？
```

选指标：

- Counter：`task_consumed_total{result="ok"}`

写法：

```promql
sum(rate(task_consumed_total{result="ok"}[10s]))
```

### 2. 如果你想看最近 5 分钟总共失败了多少条

问题：

```text
最近 5 分钟失败总量是多少？
```

选指标：

- Counter：`task_consumed_total{result="error"}`

写法：

```promql
sum(increase(task_consumed_total{result="error"}[5m]))
```

### 3. 如果你想看当前积压

问题：

```text
现在主队列里还有多少条消息？
```

选指标：

- Gauge：`rabbitmq_queue_messages{queue="tasks.q"}`

写法：

```promql
rabbitmq_queue_messages{queue="tasks.q"}
```

### 4. 如果你想看延迟

问题：

```text
95% 的消息在多长时间内处理完成？
```

选指标：

- Histogram：`task_consume_duration_seconds_bucket`

写法：

```promql
histogram_quantile(0.95, sum by (le) (rate(task_consume_duration_seconds_bucket[1m])))
```

你暂时不用一口气全背下来，只要记住：

- Counter 看 `rate` / `increase`
- Gauge 直接看当前值
- Histogram 用 `histogram_quantile`

---

## 五、学习这套项目，我更推荐哪个方案？

对你当前阶段，**两个都需要，但先后顺序不同**。

### 方案 A：先按 Dashboard 面板理解 PromQL

适合你现在这种状态：

- 代码能看一点
- 图也想看
- 但看到 `expr` 会晕

这个方案的目标是：

```text
看到一条 PromQL，能说出它在看什么业务问题
```

#### 推荐学习顺序

1. `Consume QPS`
   - `sum(rate(task_consumed_total{result="ok"}[10s]))`
   - 学会 Counter + `rate`

2. `Backlog (Queue Depth)`
   - `rabbitmq_queue_messages{queue="tasks.q"}`
   - 学会 Gauge 直接看当前值

3. `Retries / DLQ Rate`
   - `sum(rate(task_retry_scheduled_total[1m]))`
   - `sum(rate(task_retry_total[1m]))`
   - 学会重试链路怎么看

4. `Consume Latency`
   - `histogram_quantile(...)`
   - 学会 Histogram 是怎么服务于 P95/P99 的

5. `Circuit / DLQ / Rate Limit`
   - `task_circuit_open`
   - `increase(task_dlq_total[5m])`
   - 学会“治理指标”怎么设计

#### 这个方案的好处

- 你会更懂 `mq_task.json`
- 你会知道每个 panel 是从哪来的
- 你以后自己也能写简单 dashboard

### 方案 B：先做实验，再反过来看图

适合你想更快建立直觉。

这个方案的目标是：

```text
先看到图怎么变化，再倒推出 PromQL 为什么要这么写
```

#### 推荐实验顺序

1. 正常消费
   - 看 `QPS`
   - 看 `tasks.q` 是否快速回落

2. 制造积压
   - 降低 `WORKERS` / `PREFETCH`
   - 看 `Backlog` 上升

3. 制造失败
   - 让消息 `payload=fail`
   - 看 `tasks.retry.q`
   - 看 `Retries / DLQ Rate`

4. 打开限流
   - 设置 `RATE_LIMIT_PER_SEC`
   - 看 backlog 是否升高、`rate_limit_wait` 是否增加

5. 触发熔断
   - 连续失败到阈值
   - 看 `task_circuit_open` 是否变成 1

#### 这个方案的好处

- 你先建立“现象”和“图”的连接
- 再学 PromQL 时更容易懂
- 更适合你当前想边做边学的方式

### 我的建议

对你来说，最合适的是：

```text
先用方案 B 建立直觉，再用方案 A 把图和 PromQL 读懂
```

也就是：

- 先跑实验，看图怎么变
- 再回来看 `mq_task.json` 里的 `expr`

这比纯背 PromQL 更适合你。

---

## 六、一个微服务可以对 MQ 发起多个消费者连接吗？

**可以！这正是本项目在演示的核心功能。**

看 `internal/consumer/consumer.go` 的 `Run()` 方法：

```go
// 1. 从 RabbitMQ 获取消息流
deliveries, err := c.r.Ch.Consume(c.cfg.Queue, "", false, false, false, false, nil)

// 2. 启动 N 个 worker 并发消费
for i := 0; i < c.cfg.Workers; i++ {
    g.Go(func() error {
        for d := range jobs {
            c.handle(ctx, d)
        }
    })
}
```

### 如何加大消费数量？

有三种方式，从简单到复杂：

**方式 1：增加 Worker 数（进程内并发）** — 本项目演示的方式

```bash
WORKERS=20 PREFETCH=20 go run ./cmd/consumer
```

- `WORKERS=20`：启动 20 个 goroutine 并发处理消息
- `PREFETCH=20`：告诉 RabbitMQ 一次推 20 条消息过来（不用等前一条处理完）

**方式 2：启动多个 Consumer 实例（水平扩容）**

在不同的终端窗口各启动一个 consumer：
```bash
# 终端 1
WORKERS=10 go run ./cmd/consumer
# 终端 2（换个 metrics 端口避免冲突）
WORKERS=10 METRICS_ADDR=:8082 go run ./cmd/consumer
```

RabbitMQ 会自动把消息**轮询分发**给多个消费者。

**方式 3：真实生产环境 — K8s HPA 自动扩容**

根据队列深度（积压量）自动增减 Pod 数量，这是面试加分项。

### Prefetch 是什么？

- RabbitMQ 默认会把消息一股脑推给消费者
- `Prefetch=5` 意思是"我最多同时处理 5 条，处理完一条再给我下一条"
- 防止一个慢消费者囤积太多消息，导致其他消费者空闲

---

## 四、docker-compose.yml 完整解读

```yaml
services:
  rabbitmq:                              # 服务名
    image: rabbitmq:3.13-management      # 使用的 Docker 镜像（带管理界面版本）
    container_name: mq_rabbitmq          # 容器名字
    ports:
      - "5672:5672"                      # AMQP 协议端口（程序连接用）
      - "15672:15672"                    # 管理界面端口（浏览器打开用）

  redis:
    image: redis:7.4-alpine              # alpine = 精简版 Linux，镜像更小
    container_name: mq_redis
    ports:
      - "6379:6379"                      # Redis 默认端口

  prometheus:
    image: prom/prometheus:v2.54.1
    container_name: mq_prometheus
    ports:
      - "9091:9090"                      # 宿主机 9091 映射到容器 9090
    volumes:
      - ./prometheus/prometheus.yml:/etc/prometheus/prometheus.yml:ro
      # 把你本地的配置文件挂载到容器里，:ro = 只读

  grafana:
    image: grafana/grafana:11.1.4
    container_name: mq_grafana
    ports:
      - "3001:3000"                      # 宿主机 3001 映射到容器 3000
    environment:
      - GF_SECURITY_ADMIN_USER=admin     # 设置 Grafana 登录账号
      - GF_SECURITY_ADMIN_PASSWORD=admin # 设置 Grafana 登录密码
    volumes:
      - ./grafana/provisioning:/etc/grafana/provisioning:ro   # 自动配置数据源
      - ./grafana/dashboards:/var/lib/grafana/dashboards:ro   # 自动导入看板
    depends_on:
      - prometheus                       # 等 prometheus 启动后再启动 grafana
```

---

## 五、整体数据流

```
┌──────────┐     发消息      ┌──────────┐    收消息     ┌──────────┐
│ Producer │ ──AMQP──────→ │ RabbitMQ │ ──AMQP────→ │ Consumer │
│ cmd/     │               │ (Docker) │              │ cmd/     │
│ producer │               │          │              │ consumer │
└──────────┘               └──────────┘              └────┬─────┘
                                                          │
                           ┌──────────┐  幂等检查(SETNX)  │
                           │  Redis   │ ←─────────────────┤
                           │ (Docker) │                   │
                           └──────────┘                   │
                                                          │ 暴露 /metrics
                           ┌──────────┐  每5秒抓一次      │
                           │Prometheus│ ←─HTTP GET────────┤
                           │ (Docker) │                   │
                           └────┬─────┘              ┌────┴─────┐
                                │ 查询数据            │ HTTP :8081│
                           ┌────┴─────┐              └──────────┘
                           │ Grafana  │
                           │ (Docker) │ ← 你在浏览器看图表
                           └──────────┘
```

---

## 六、Mac 环境变量补充说明

在 Mac（zsh）下设置环境变量的三种方式：

```bash
# 方式 1：只对这一条命令生效（推荐！最安全）
WORKERS=2 PREFETCH=2 go run ./cmd/consumer

# 方式 2：对当前终端窗口生效，关窗口就没了
export WORKERS=2
export PREFETCH=2
go run ./cmd/consumer

# 方式 3：永久生效（写入 ~/.zshrc，一般不需要）
echo 'export WORKERS=2' >> ~/.zshrc
```

**结论：方式 1 和 2 绝对不会污染系统。只有方式 3 才会永久生效。**

注意：DRILL.md 里的 `$env:WORKERS="2"` 是 Windows PowerShell 语法，Mac 上不能用。

---

## 七、推荐阅读顺序（建议动手跑）

### Step 1：理解基础设施（10 分钟）

1. 读 `docker-compose.yml` — 知道启动了哪些服务
2. 读 `prometheus/prometheus.yml` — 知道 Prometheus 从哪抓数据

### Step 2：看最简单的 Producer（15 分钟）

1. `cmd/producer/main.go` — 入口，解析命令行参数
2. `internal/producer/producer.go` — 并发发消息的逻辑
3. `internal/mq/rabbitmq.go` — RabbitMQ 连接、声明队列、发消息

### Step 3：看核心的 Consumer（30 分钟）

1. `cmd/consumer/main.go` — 入口，启动 HTTP metrics 服务器 + consumer
2. `internal/consumer/consumer.go` — **重点文件**
   - `ConfigFromEnv()` — 从环境变量读配置
   - `New()` — 初始化连接
   - `Run()` — dispatcher + worker pool 模式
   - `handle()` — 单条消息处理流程：解析 → 幂等 → 业务 → ack/reject

### Step 4：看辅助模块（15 分钟）

1. `internal/dedup/redis_dedup.go` — Redis SETNX 幂等
2. `internal/metrics/metrics.go` — Prometheus 指标定义
3. `internal/mq/mgmt.go` — RabbitMQ 管理 API（获取队列深度）

### Step 5：实操演练（30 分钟）

按 DRILL.md 跑一遍：制造积压 → 提升并发 → 观察恢复。

---

## 八、如何写进简历 / 工作经历

### 项目名称

**异步任务处理系统（RabbitMQ + Redis）**

### 项目描述模板

> 基于 RabbitMQ 实现的异步任务处理系统，支持 at-least-once 投递语义、Redis 幂等去重、
> DLQ 死信队列兜底、Worker Pool 并发消费。集成 Prometheus + Grafana 实现吞吐量、
> 延迟 P95/P99、队列积压等指标的实时监控。

### 亮点（面试可以主动提的点）

- **消息可靠性**：at-least-once + SETNX 幂等，保证消息不丢且不重复执行
- **积压治理**：通过调整 Worker/Prefetch 数或水平扩容消费者，实测 {backlog_peak} 条积压在 {minutes} 分钟内恢复
- **死信队列**：失败消息进入 DLQ，支持人工介入或自动重放
- **可观测性**：Prometheus 指标 + Grafana 看板，覆盖 QPS、错误率、延迟分位数、队列深度

### 如何结合到你的大型项目

你现在的 Iris 项目可以这样集成：

1. **异步任务解耦** — 把耗时操作（发邮件、生成报表、图片处理）从 HTTP 请求里拆出来，发到 MQ
2. **producer 嵌入 Iris 服务** — 在 API handler 里调用 `producer.Publish()`
3. **consumer 独立部署** — 作为单独的微服务运行
4. **幂等组件直接复用** — `dedup` 包可以直接拿去用
5. **metrics 组件复用** — Prometheus 指标可以加到 Iris 中间件里

---

## 九、关于 observability-starter 项目

详见 `projects/observability-starter/STUDY_GUIDE.md`，那里会详细讲解：

- Jaeger 分布式链路追踪（trace_id + span 如何跨服务传播）
- OpenTelemetry 标准
- 如何把 trace context 放在 HTTP Header 里传递给其他微服务
- 如何结合到你的 Iris 项目中
