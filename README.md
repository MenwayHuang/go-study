# Go 高并发架构实战教程

> 面向 Go 后端工程师的高并发、高可用架构实战学习路径

## 📖 目录

| 章节 | 主题 | 核心知识点 | 依赖 | 难度 |
|------|------|-----------|------|------|
| [第1章](./ch01-concurrency-basics/) | Go并发基础 | goroutine、channel、sync原语、context | 无 | ⭐⭐ |
| [第2章](./ch02-concurrency-patterns/) | 并发模式 | Worker Pool、Fan-out/Fan-in、Pipeline、限流器 | 无 | ⭐⭐⭐ |
| [第3章](./ch03-high-concurrency-http/) | 高并发HTTP服务 | 优雅关闭、超时控制、中间件链 | 无 | ⭐⭐⭐ |
| [第4章](./ch04-message-queue/) | 消息队列解耦 | 内存MQ、Pub/Sub、异步处理 | 无 | ⭐⭐⭐ |
| [第5章](./ch05-cache-strategy/) | 缓存策略 | 本地缓存、LRU、穿透/雪崩/击穿、SingleFlight | 无 | ⭐⭐⭐ |
| [第6章](./ch06-microservice/) | 微服务架构 | 服务注册发现、负载均衡、熔断器、链路追踪 | 无 | ⭐⭐⭐⭐ |
| [第7章](./ch07-high-availability/) | 高可用设计 | 重试策略、优雅降级、幂等设计、健康检查 | 无 | ⭐⭐⭐⭐ |
| [第8章](./ch08-performance/) | 性能优化 | pprof、sync.Pool、内存优化、锁优化 | 无 | ⭐⭐⭐⭐ |
| [第9章](./ch09-seckill-system/) | 实战：秒杀系统 | 综合运用全部知识 + 内置压测 | 无 | ⭐⭐⭐⭐⭐ |
| [第10章](./ch10-middleware-practice/) | 中间件实战 | Redis缓存/锁/计数器 + NATS消息队列 | **Docker** | ⭐⭐⭐⭐ |

> **第1-9章**：纯Go实现，零外部依赖，专注理解原理
> **第10章**：使用Docker启动真实Redis + NATS，学习生产级中间件操作

---

## 🏗️ 架构全景图

```
                        ┌─────────────┐
                        │   Client    │
                        └──────┬──────┘
                               │
                        ┌──────▼──────┐
                        │  API Gateway │  ← 限流、鉴权、路由 (第2,3章)
                        │  (Nginx/Kong)│
                        └──────┬──────┘
                               │
              ┌────────────────┼────────────────┐
              │                │                │
       ┌──────▼──────┐ ┌──────▼──────┐ ┌──────▼──────┐
       │  Service A  │ │  Service B  │ │  Service C  │  ← 微服务 (第6章)
       │  熔断+重试   │ │  降级+幂等   │ │  健康检查    │  ← 高可用 (第7章)
       └──────┬──────┘ └──────┬──────┘ └──────┬──────┘
              │                │                │
              └────────────────┼────────────────┘
                               │
              ┌────────────────┼────────────────┐
              │                │                │
       ┌──────▼──────┐ ┌──────▼──────┐ ┌──────▼──────┐
       │    Redis     │ │  MQ (NATS)  │ │    MySQL    │  ← 中间件 (第10章)
       │  缓存+锁     │ │  异步解耦    │ │  持久存储    │
       └─────────────┘ └─────────────┘ └─────────────┘
```

---

## 🚀 快速开始

### 第1-9章（无需Docker）
```bash
cd go-study

# 每一章独立运行，进入目录执行 go run .
cd ch01-concurrency-basics
go run . -demo all           # 运行全部示例
go run . -demo goroutine     # 只运行某个主题

# 第3章启动的是HTTP服务，运行后用curl测试
cd ch03-high-concurrency-http
go run .
# 另开终端: curl http://localhost:8080/api/data

# 第9章秒杀系统：两个终端配合
cd ch09-seckill-system
go run . -mode server        # 终端1: 启动服务
go run . -mode stress        # 终端2: 运行压测
```

### 第10章（需要Docker）
```bash
cd ch10-middleware-practice

# 1. 一键启动 Redis + NATS
docker-compose up -d

# 2. 验证服务
docker exec go-redis redis-cli ping       # 应返回 PONG
# 浏览器打开 http://localhost:8222        # NATS监控面板

# 3. 安装依赖并运行
go mod tidy
go run . -demo all

# 4. 实时观测Redis命令 (另开终端)
docker exec -it go-redis redis-cli monitor

# 5. 用完关闭
docker-compose down
```

---

## 📚 每章运行方式速查

| 章节 | 命令 | 说明 |
|------|------|------|
| 第1章 | `go run . -demo all` | goroutine/channel/sync/context/泄漏 |
| 第2章 | `go run . -demo all` | WorkerPool/FanOut/Pipeline/限流/errgroup |
| 第3章 | `go run .` | 启动HTTP服务 → curl测试 |
| 第4章 | `go run . -demo all` | 消息队列/发布订阅/订单处理 |
| 第5章 | `go run . -demo all` | 缓存/LRU/穿透雪崩击穿/SingleFlight |
| 第6章 | `go run . -demo all` | 注册发现/负载均衡/熔断器/链路追踪 |
| 第7章 | `go run . -demo all` | 重试/降级/幂等/健康检查 |
| 第8章 | `go run . -demo all` | sync.Pool/内存优化/锁优化 |
| 第8章 | `go run . -demo pprof` | 启动pprof服务 → 浏览器分析 |
| 第9章 | `go run . -mode server` | 启动秒杀服务 |
| 第9章 | `go run . -mode stress` | 压测客户端(需先启动server) |
| 第10章 | `docker-compose up -d` | 先启动Redis+NATS |
| 第10章 | `go run . -demo all` | Redis/NATS/秒杀库存 |

---

## 🛠️ 环境要求

- **Go** 1.21+ (已安装即可)
- **Docker Desktop** (Windows) — 仅第10章需要
- **16GB RAM** — 足够运行所有示例
- **curl** — 测试HTTP接口 (Windows自带或用PowerShell的Invoke-WebRequest)

---

## 📝 学习方法 — 如何高效掌握

### 🔄 每章学习流程（建议按此循环）

```
1. 读 README.md
   ↓ 先理解原理和架构图，知道"为什么要这样做"
2. 运行 demo
   ↓ go run . -demo all, 观察输出
3. 读源码
   ↓ 重点看带 "关键点:" 注释的代码，那是核心
4. 改参数
   ↓ 改并发数/超时/容量等参数，观察行为变化
5. 自己写
   ↓ 合上代码，尝试自己实现一遍
```

### 📌 代码中的标记说明

- **`关键点:`** — 最核心的知识，面试常问
- **`生产对应:`** — 对应到真实生产环境的组件
- **`❌` / `✅`** — 错误做法 vs 正确做法对比

### 🗓️ 建议学习节奏

| 阶段 | 时间 | 内容 | 目标 |
|------|------|------|------|
| 第1周 | 第1-3章 | 并发基础 + 模式 + HTTP | 能写出并发安全的HTTP服务 |
| 第2周 | 第4-6章 | MQ + 缓存 + 微服务 | 理解分布式系统核心组件 |
| 第3周 | 第7-8章 | 高可用 + 性能优化 | 掌握生产级技巧 |
| 第4周 | 第9-10章 | 秒杀实战 + 中间件 | 综合运用 + 真实中间件 |

### 💡 进阶建议（学完本教程后）

1. **读源码**: `net/http` 标准库、`sync.Pool` 实现
2. **框架实战**: 用 `Gin` + `gRPC` 重写第3/6章
3. **真实中间件**: 在第10章基础上，接入 MySQL + Kafka
4. **压测工具**: 学习 `wrk`、`vegeta`、`k6` 进行真正的性能测试
5. **可观测性**: 接入 Prometheus + Grafana 监控
6. **部署**: 用 Docker + K8s 部署微服务集群

---

## 📖 知识总结

详见 [SUMMARY.md](./SUMMARY.md) — 全部知识点的速查手册，适合复习和面试准备。
