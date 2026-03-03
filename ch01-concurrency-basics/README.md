# 第1章：Go 并发基础

## 🎯 学习目标

掌握 Go 并发编程的四大核心原语，理解它们的适用场景和陷阱。

## 📚 知识点总览

```
Go 并发基础
├── 1.1 Goroutine — 轻量级协程
│   ├── 创建与调度原理 (GMP模型)
│   ├── goroutine 泄漏问题
│   └── 生命周期管理
├── 1.2 Channel — 通信机制
│   ├── 无缓冲 vs 有缓冲
│   ├── select 多路复用
│   ├── 单向 channel
│   └── channel 使用陷阱
├── 1.3 sync 包 — 同步原语
│   ├── Mutex / RWMutex 互斥锁
│   ├── WaitGroup 等待组
│   ├── Once 单次执行
│   ├── Map 并发安全Map
│   └── Pool 对象池 (第8章深入)
└── 1.4 Context — 上下文控制
    ├── 超时控制
    ├── 取消传播
    └── 值传递
```

---

## 1.1 Goroutine 深入理解

### GMP 调度模型（面试高频）

```
┌──────────────────────────────────────────┐
│              Go Runtime Scheduler         │
│                                          │
│  G (Goroutine)  M (Machine/OS Thread)    │
│  ┌──┐ ┌──┐     ┌──┐ ┌──┐              │
│  │G1│ │G2│     │M1│ │M2│              │
│  └──┘ └──┘     └──┘ └──┘              │
│  ┌──┐ ┌──┐       │     │               │
│  │G3│ │G4│       │     │               │
│  └──┘ └──┘       │     │               │
│       │          │     │               │
│  P (Processor/逻辑处理器)                │
│  ┌────────┐  ┌────────┐                │
│  │   P1   │  │   P2   │                │
│  │LocalQ: │  │LocalQ: │                │
│  │[G1,G3] │  │[G2,G4] │                │
│  └────────┘  └────────┘                │
│       ↓           ↓                     │
│  GlobalQueue: [G5, G6, G7...]           │
└──────────────────────────────────────────┘
```

**核心要点：**
- **G (Goroutine)**：用户态协程，初始栈只有 **2KB**（线程通常 1MB），可以轻松创建百万级
- **M (Machine)**：操作系统线程，真正执行代码的载体
- **P (Processor)**：逻辑处理器，默认数量 = CPU核心数（`GOMAXPROCS`）
- **调度策略**：Work Stealing — 当P的本地队列为空时，会从其他P或全局队列偷取G

### Goroutine 泄漏 — 最常见的并发Bug

goroutine 泄漏是指 goroutine 被创建后永远无法退出，导致内存不断增长。

**常见原因：**
1. channel 读写阻塞，没有超时或取消机制
2. 死循环没有退出条件
3. 启动了 goroutine 但没有管理其生命周期

---

## 1.2 Channel 核心规则

| 操作 | nil channel | 已关闭 channel | 正常 channel |
|------|------------|---------------|-------------|
| 读 `<-ch` | 永久阻塞 | 返回零值 | 阻塞或成功 |
| 写 `ch<-` | 永久阻塞 | **panic** | 阻塞或成功 |
| 关闭 `close(ch)` | **panic** | **panic** | 成功 |

**黄金法则：**
- 只有发送方关闭 channel，接收方永远不关闭
- 有多个发送方时，不要关闭 channel，用 context 通知退出

---

## 1.3 sync 包选择指南

| 场景 | 推荐方案 | 原因 |
|------|---------|------|
| 多个goroutine读写共享变量 | `sync.Mutex` | 简单直接 |
| 读多写少的共享数据 | `sync.RWMutex` | 读不互斥，性能更好 |
| 等待一组goroutine完成 | `sync.WaitGroup` | 标准做法 |
| 只执行一次的初始化 | `sync.Once` | 线程安全的单例 |
| 并发安全的KV存储 | `sync.Map` | 读多写少时性能好 |
| 频繁创建销毁临时对象 | `sync.Pool` | 减少GC压力 |

---

## 1.4 Context 使用规范

```
context.Background()          ← 根 context，main/init/test 中使用
    │
    ├── context.WithCancel()  ← 手动取消（父取消子也取消）
    ├── context.WithTimeout() ← 超时自动取消
    ├── context.WithDeadline()← 到达截止时间自动取消
    └── context.WithValue()   ← 传递请求范围的值（少用）
```

**规范：**
- context 作为函数第一个参数，命名为 `ctx`
- 不要把 context 存储在 struct 中
- 不要传递 nil context，不确定就用 `context.TODO()`

---

## 🏃 运行示例

```bash
cd ch01-concurrency-basics

# 运行所有示例
go run main.go

# 运行特定示例（通过命令行参数）
go run main.go -demo goroutine
go run main.go -demo channel
go run main.go -demo sync
go run main.go -demo context
go run main.go -demo leak       # goroutine泄漏演示与修复
```

## 📁 文件结构

```
ch01-concurrency-basics/
├── README.md          ← 你正在读的文档
├── main.go            ← 入口，选择运行哪个示例
├── goroutine_demo.go  ← goroutine 基础 + GMP 理解
├── channel_demo.go    ← channel 各种用法
├── sync_demo.go       ← sync 包实战
├── context_demo.go    ← context 超时取消控制
└── leak_demo.go       ← goroutine 泄漏演示与修复
```
