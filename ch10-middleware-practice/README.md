# 第10章：中间件实战 — Redis + NATS + Docker

## 🎯 学习目标

用 Docker 启动真实的 Redis 和 NATS 消息队列，学习如何在 Go 中连接、使用这些中间件，
并将它们与高并发架构结合，替代前面章节中的内存模拟。

## 📚 为什么需要这一章？

前面1-9章所有的"缓存/消息队列/服务发现"都是**内存模拟**，目的是理解原理。
但在生产中，你必须使用真实的中间件组件：

| 前面章节(内存模拟) | 本章(真实中间件) | 生产中的角色 |
|-------------------|----------------|-------------|
| `sync.Map` 缓存 | **Redis** | 分布式缓存、分布式锁、计数器 |
| `chan` 消息队列 | **NATS** | 微服务通信、事件驱动、削峰 |

## 🐳 Docker 环境准备

### 一键启动所有中间件

```bash
cd ch10-middleware-practice
docker-compose up -d
```

这会启动：
- **Redis**: 端口 6379，高性能KV缓存
- **NATS**: 端口 4222(客户端) + 8222(监控面板)

### 验证服务是否启动

```bash
# Redis
docker exec go-redis redis-cli ping
# 应返回: PONG

# NATS 监控面板 (浏览器打开)
# http://localhost:8222
```

### 停止服务

```bash
docker-compose down
```

---

## 📁 文件结构

```
ch10-middleware-practice/
├── docker-compose.yml     → 一键启动 Redis + NATS
├── main.go                → 入口，选择 demo
├── redis_demo.go          → Redis 实战：缓存/分布式锁/计数器/排行榜
├── nats_demo.go           → NATS 实战：发布订阅/请求响应/队列组
├── seckill_redis.go       → 用Redis改造秒杀系统的库存扣减
└── README.md
```

---

## 📚 知识点详解

### 10.1 Redis — 不只是缓存

```
Redis 在高并发系统中的 5 大用途:

1. 缓存 (Cache)
   SET user:1001 '{"name":"张三"}' EX 300
   → 减少数据库查询，提升读性能

2. 分布式锁 (Distributed Lock)
   SET lock:order:123 "owner1" NX EX 10
   → 多实例部署时，保证同一时间只有一个进程处理

3. 计数器/限流 (Counter / Rate Limit)
   INCR api:rate:user1001
   EXPIRE api:rate:user1001 1
   → 精确统计QPS，实现滑动窗口限流

4. 库存预减 (Stock Decrement)
   DECR seckill:stock:PROD001
   → 原子操作，防止超卖

5. 排行榜 (Sorted Set)
   ZADD leaderboard 95 "player1"
   ZREVRANGE leaderboard 0 9 WITHSCORES
   → 实时排名
```

### 10.2 NATS — 云原生消息系统

```
NATS 三种通信模式:

1. Pub/Sub (发布订阅)
   发布者 → NATS → 所有订阅者
   场景: 配置变更通知、事件广播

2. Queue Group (队列组)
   发布者 → NATS → 队列组中随机一个消费者
   场景: 任务分发、负载均衡消费 (类似Kafka消费者组)

3. Request/Reply (请求响应)
   客户端 → NATS → 服务端 → NATS → 客户端
   场景: 微服务间RPC调用
```

### NATS vs Kafka vs RabbitMQ 选型

| 特性 | NATS | Kafka | RabbitMQ |
|------|------|-------|----------|
| 延迟 | **微秒级** | 毫秒级 | 毫秒级 |
| 吞吐 | 高 | **极高** | 中 |
| 持久化 | JetStream | ✓ | ✓ |
| 复杂度 | **极简** | 高 | 中 |
| 适用 | 微服务通信 | 大数据/日志 | 企业级 |
| Go生态 | ⭐⭐⭐⭐⭐ | ⭐⭐⭐ | ⭐⭐⭐ |

> NATS 是 Go 编写的，与 Go 生态契合度最高，也是 CNCF 毕业项目。
> 本教程选 NATS 是因为它**轻量、易学、高性能**，适合入门。

---

## 🏃 运行示例

```bash
# 0. 先启动 Docker 中间件
cd ch10-middleware-practice
docker-compose up -d

# 1. 安装 Go 依赖
go mod tidy

# 2. 运行各个 demo
go run . -demo redis       # Redis 缓存/锁/计数器
go run . -demo nats        # NATS 发布订阅/队列组
go run . -demo seckill     # Redis 版秒杀库存扣减
go run . -demo all         # 全部运行

# 3. 用完后关闭 Docker
docker-compose down
```

## 🔍 观测方式

### Redis 观测
```bash
# 进入 Redis 容器，实时监控所有命令
docker exec -it go-redis redis-cli monitor

# 查看某个 key
docker exec go-redis redis-cli GET "cache:user:1001"

# 查看所有 key
docker exec go-redis redis-cli KEYS "*"
```

### NATS 观测
```
浏览器打开: http://localhost:8222

/varz    → 服务器信息
/connz   → 连接信息
/subz    → 订阅信息
/routez  → 路由信息
```

---

## 💡 学习建议

1. **先跑通**: `docker-compose up -d` → `go run . -demo all`
2. **观测**: 另开终端用 `redis-cli monitor` 看 Redis 命令流
3. **改参数**: 修改并发数、缓存TTL、消息数量，观察行为变化
4. **对比**: 回看第5章(内存缓存) vs 本章(Redis缓存)，理解区别
5. **生产映射**: 每个 demo 都有注释说明"生产中怎么用"
