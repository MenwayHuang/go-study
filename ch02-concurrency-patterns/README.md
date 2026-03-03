# 第2章：并发模式 (Concurrency Patterns)

## 🎯 学习目标

掌握 Go 中最实用的并发设计模式，这些是构建高并发系统的核心"武器库"。

## 📚 知识点总览

```
并发模式
├── 2.1 Worker Pool (工作池)
│   ├── 为什么需要 Worker Pool
│   ├── 固定大小 Worker Pool 实现
│   └── 动态调整 Worker 数量
├── 2.2 Fan-out / Fan-in (扇出/扇入)
│   ├── 一个任务分发给多个worker = Fan-out
│   └── 多个worker的结果汇聚到一个channel = Fan-in
├── 2.3 Pipeline (流水线)
│   ├── 多阶段数据处理
│   └── 每个阶段独立并发
├── 2.4 Rate Limiter (限流器)
│   ├── 令牌桶 (Token Bucket)
│   ├── 滑动窗口
│   └── 漏桶 (Leaky Bucket)
└── 2.5 errgroup 错误处理
    ├── 并发任务的错误收集
    └── 第一个错误取消所有任务
```

---

## 2.1 Worker Pool — 最重要的并发模式

### 为什么需要 Worker Pool？

**问题**: 为每个请求创建一个goroutine → 高并发下goroutine爆炸 → OOM

```
❌ 错误做法: 每个请求一个goroutine
   10万请求 → 10万goroutine → 内存暴涨 → 服务崩溃

✅ 正确做法: Worker Pool
   10万请求 → 任务队列 → N个Worker消费 → 可控的资源使用
```

### 实际项目常见疑问与要点

#### 是否必须提前知道 task 数量？

不必须。`close(jobs)` 的语义不是“我知道有多少任务”，而是“生产者确认不会再发送新任务”。

- **批处理/有限任务**: 生产者发完这一批任务后 `close(jobs)`。
- **长期运行/流式任务**: 通常用 `context.Context` 或 `done` 信号控制退出；当服务关闭或上游停止时，让生产者停止并在合适的时机 `close(jobs)`。

#### 谁来 close(channel)？（非常重要）

- **谁创建 channel，通常谁负责关闭**。
- **只有发送方/生产者才能 close**，接收方不应关闭。
- **多生产者场景**: 不要让某个 worker 或某个 producer 直接 `close(jobs)`，常见做法是用 `WaitGroup` 等所有 producer 退出后，由一个统一的协调者 `close(jobs)`。

#### results channel 怎么结束？

常见模式：worker 退出由 `range jobs` 驱动；results 的关闭由协调者在 `wg.Wait()` 后统一 `close(results)`，让消费者可以 `for r := range results` 自然结束。

### 架构图

```
  Producer(s)              Worker Pool              Consumer
  ┌────────┐          ┌───────────────────┐
  │ Task 1 │──┐       │ ┌──────┐ ┌──────┐ │
  │ Task 2 │──┼──→ Jobs Channel │  W1  │ │  W2  │ │──→ Results Channel
  │ Task 3 │──┤       │ └──────┘ └──────┘ │
  │  ...   │──┘       │ ┌──────┐ ┌──────┐ │
  │ Task N │          │ │  W3  │ │  W4  │ │
  └────────┘          │ └──────┘ └──────┘ │
                      └───────────────────┘
```

---

## 2.2 Fan-out / Fan-in

```
Fan-out (扇出): 一个数据源 → 多个处理者

    Source ──┬──→ Worker 1
             ├──→ Worker 2
             └──→ Worker 3

Fan-in (扇入): 多个处理者 → 汇聚到一个通道

    Worker 1 ──┐
    Worker 2 ──┼──→ Merged Channel → Consumer
    Worker 3 ──┘
```

**适用场景**: 批量API调用、并行文件处理、分布式计算

### 实际项目常见疑问：为什么每个 worker 用自己的 out channel 再 merge？

`Fan-in` 常见有两种写法：

- **共享一个 results channel**: 所有 worker 直接往同一个 `results` 里写。
  - 优点: 代码直接。
  - 关键点: 不能由任何一个 worker 来 `close(results)`；通常由协调者 `wg.Wait()` 后统一关闭，否则容易出现 `send on closed channel`。

- **每个 worker 一个 out channel + merge**（本章代码采用）:
  - 优点: 关闭责任清晰（worker 关闭自己的 out；merge 负责在所有 out 都结束后关闭 merged），也更易复用/组合（更像 pipeline 组件）。
  - 代价: 需要一个 `merge()` 转发层（通常成本很小）。

---

## 2.3 Pipeline

```
Stage 1          Stage 2          Stage 3
┌─────────┐     ┌─────────┐     ┌─────────┐
│ Generate │────→│ Process │────→│ Output  │
│  Data    │ ch1 │  Data   │ ch2 │ Result  │
└─────────┘     └─────────┘     └─────────┘
```

### 实际项目常见疑问与要点

#### Pipeline 为什么强调“每一段都 close 自己的 out”？

Pipeline 的核心是“**上游结束信号能可靠传递到下游**”。通用约定是：

- **每个 stage 创建自己的 out channel，并在 stage 退出前 `close(out)`**。
- 下游用 `for v := range in` 读取，上游 close 后下游自然退出。

#### 背压 (Backpressure) 是好事还是坏事？

在 Pipeline/WorkerPool 里，**下游慢会让上游在发送时阻塞**，这就是背压。

- 好处: 自动限制系统内存/队列增长，避免“无限堆积”。
- 代价: 吞吐由最慢 stage 决定；需要按瓶颈段加 worker 或加 buffer。

#### 取消/超时怎么传播？

生产级 pipeline 常见做法：把 `context.Context` 作为参数传入每个 stage，在 `select { case out <- v: ...; case <-ctx.Done(): return }` 中退出，避免 goroutine 泄漏。

**优势**: 每个阶段独立并发，可以根据瓶颈单独扩展某个阶段

---

## 2.4 Rate Limiter 限流器详解

限流是高并发系统的核心保护机制，防止系统被突发流量打垮。

### 2.4.1 令牌桶 (Token Bucket) 原理详解

```
                    ┌─────────────────────┐
   固定速率添加令牌 ──→│    Token Bucket     │
   (如每100ms加1个)   │    capacity=5       │
                    │    ●●●●●            │  ← 当前有5个令牌
                    └─────────┬───────────┘
                              │
                     请求到来，消耗1个令牌
                              ↓
                    ┌─────────────────────┐
                    │    ●●●●○            │  ← 剩余4个令牌
                    └─────────────────────┘
```

**核心机制**：
1. **令牌生成**：后台以固定速率（rate）向桶中添加令牌
2. **令牌消费**：每个请求消耗一个令牌，有令牌则放行，无令牌则拒绝
3. **容量上限**：桶满后新令牌被丢弃，不会无限累积
4. **突发处理**：桶里攒了令牌时，可以瞬间处理一波突发请求（核心优势）

**参数含义**：
- `capacity`：桶容量，决定最大突发量（capacity=10 表示最多瞬间放行10个请求）
- `rate`：令牌添加间隔，决定稳态吞吐（每100ms加1个 = 10 QPS）

**为什么允许突发是优势？** 真实流量有波动，令牌桶在空闲时"攒"令牌，突发时"花"令牌，自动平滑波动。

### 2.4.2 漏桶 (Leaky Bucket) 原理详解

```
   请求到来 ──→ ┌─────────────────────┐
               │    Leaky Bucket     │  ← 请求先进入桶排队
               │    ○○○○○○○○○○      │
               └─────────┬───────────┘
                         │
            固定速率流出（如每100ms处理1个）
                         ↓
                     处理请求
```

**核心机制**：
1. **请求入桶**：所有请求先进入桶（队列）
2. **固定速率流出**：以恒定速率从桶中取出请求处理
3. **桶满拒绝**：桶满时新请求被拒绝
4. **平滑输出**：无论输入多突发，输出始终平滑

**与令牌桶的关键区别**：
- 令牌桶：允许突发（桶里有令牌就能立即处理）
- 漏桶：强制平滑（即使系统空闲，也按固定速率处理）

### 2.4.3 滑动窗口 (Sliding Window) 原理详解

```
时间轴：
  ──────────────────────────────────────────────→ 时间
  │←────────── 1秒窗口 ──────────→│
  
  窗口内的请求时间戳：
  [0.1s] [0.3s] [0.5s] [0.7s] [0.9s]  ← 5个请求
  
  新请求到来 (1.05s)，窗口向前滑动：
       │←────────── 1秒窗口 ──────────→│
       [0.3s] [0.5s] [0.7s] [0.9s] [1.05s]  ← 0.1s 已滑出窗口
```

**核心机制**：
1. **记录时间戳**：每个请求到来时，记录其时间戳
2. **窗口滑动**：窗口随当前时间向前滑动（不是固定的时间段）
3. **清理过期**：每次检查时，移除窗口外（过期）的时间戳
4. **计数判断**：统计窗口内的请求数，超过阈值则拒绝

**代码实现关键步骤**（对应 `SlidingWindow.Allow()`）：
```go
// 1. 计算窗口起始时间
windowStart := now.Add(-windowSize)  // 当前时间 - 1秒

// 2. 清理窗口外的旧时间戳
valid := timestamps[:0]
for _, ts := range timestamps {
    if ts.After(windowStart) {  // 只保留窗口内的
        valid = append(valid, ts)
    }
}

// 3. 判断是否超过限制
if len(valid) >= maxReqs { return false }

// 4. 记录新请求时间戳
timestamps = append(valid, now)
return true
```

**复杂度分析**：
- 时间：O(n)，n 为窗口内请求数（每次遍历清理）
- 空间：O(n)，存储所有时间戳

**高 QPS 优化方案：分桶计数**
```
  │ 桶1  │ 桶2  │ 桶3  │ 桶4  │ 桶5  │  ← 每桶200ms
  │  3   │  5   │  2   │  4   │  1   │  ← 只存计数
  └──────┴──────┴──────┴──────┴──────┘
  总数 = 3+5+2+4+1 = 15
```
- 把窗口分成多个小桶，每桶只存计数
- 时间复杂度降为 O(桶数)，通常是常数

### 2.4.4 三种算法对比

| 算法 | 原理 | 优点 | 缺点 | 适用场景 |
|------|------|------|------|---------|
| 令牌桶 | 固定速率放入令牌，请求消耗令牌 | 允许突发流量 | 实现稍复杂 | API限流 |
| 漏桶 | 请求进入桶，固定速率流出 | 输出平滑 | 不允许突发 | 消息推送 |
| 滑动窗口 | 统计窗口内请求数 | 精确 | 内存占用较大 | 精确计数 |

### 实际项目常见疑问与要点

#### Token Bucket 的 rate/capacity 各表示什么？

- **rate**: 稳态吞吐（例如每 100ms 补 1 个令牌 ≈ 10 QPS）。
- **capacity**: 允许的突发量（桶里攒满令牌时可以瞬间放行一波）。

#### 为什么要显式 Stop()/Stop Ticker？

- 令牌桶通常有后台 refill goroutine；`time.Ticker` 也会持有资源。
- 不再使用时应停止/退出，否则会造成 goroutine/ticker 泄漏。
- 生产环境常用 `context` 驱动退出：`select { case <-ctx.Done(): return }`。

#### Sliding Window 的成本与取舍

- 简单实现常见是“保存时间戳 + 每次清理窗口外数据”，高 QPS 下可能退化。
- 更常见的工程实现是“分桶计数/环形数组”来把每次清理压到近似 O(1)。

---

## 2.5 errgroup 错误处理

### 实际项目常见疑问与要点

#### "收集第一个错误" vs "快速失败取消其他任务"

- **只收集错误**: 等所有 goroutine 都结束后返回第一个错误。
- **快速失败**: 一旦某个任务失败，立刻 `cancel()`；其他任务需要在逻辑里监听 `ctx.Done()` 并尽快退出，避免浪费资源。

#### 并发上限（非常常见的需求）

真实项目经常需要“并发做 N 件事，但最多同时跑 K 个”。

- 常见做法: 用带缓冲的 semaphore channel 控制并发度。
- 使用外部包时，可用 `errgroup.Group.SetLimit()`。

---

## 🏃 运行示例

```bash
cd ch02-concurrency-patterns
go run . -demo workerpool   # Worker Pool
go run . -demo fanout       # Fan-out/Fan-in
go run . -demo pipeline     # Pipeline
go run . -demo ratelimit    # 限流器
go run . -demo errgroup     # 错误处理
go run . -demo all          # 全部
```
