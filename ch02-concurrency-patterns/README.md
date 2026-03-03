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

---

## 2.3 Pipeline

```
Stage 1          Stage 2          Stage 3
┌─────────┐     ┌─────────┐     ┌─────────┐
│ Generate │────→│ Process │────→│ Output  │
│  Data    │ ch1 │  Data   │ ch2 │ Result  │
└─────────┘     └─────────┘     └─────────┘
```

**优势**: 每个阶段独立并发，可以根据瓶颈单独扩展某个阶段

---

## 2.4 Rate Limiter 对比

| 算法 | 原理 | 优点 | 缺点 | 适用场景 |
|------|------|------|------|---------|
| 令牌桶 | 固定速率放入令牌，请求消耗令牌 | 允许突发流量 | 实现稍复杂 | API限流 |
| 漏桶 | 请求进入桶，固定速率流出 | 输出平滑 | 不允许突发 | 消息推送 |
| 滑动窗口 | 统计窗口内请求数 | 精确 | 内存占用较大 | 精确计数 |

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
