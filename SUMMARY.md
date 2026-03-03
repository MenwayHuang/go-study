# Go 高并发架构 — 知识点总结

> 本文档是所有章节知识点的系统性梳理，可作为速查手册和面试准备材料。

---

## 一、并发编程基础

### 1.1 GMP 调度模型
- **G (Goroutine)**: 用户态协程，初始栈 2KB，可创建百万级
- **M (Machine)**: OS线程，真正执行代码的载体
- **P (Processor)**: 逻辑处理器，默认 = CPU核心数 (`GOMAXPROCS`)
- **调度策略**: Work Stealing — P本地队列为空时从其他P偷取G
- **面试要点**: goroutine vs 线程的区别、GMP工作流程、goroutine泄漏排查

### 1.2 Channel 核心规则
| 操作 | nil channel | 已关闭 channel | 正常 channel |
|------|-----------|--------------|-------------|
| 读 | 永久阻塞 | 返回零值 | 阻塞或成功 |
| 写 | 永久阻塞 | **panic** | 阻塞或成功 |
| 关闭 | **panic** | **panic** | 成功 |

**黄金法则**: 只有发送方关闭channel；多发送方时用context通知退出

### 1.3 sync 包选择
| 场景 | 方案 |
|------|------|
| 共享变量读写 | `sync.Mutex` |
| 读多写少 | `sync.RWMutex` |
| 等待一组goroutine | `sync.WaitGroup` |
| 单次初始化 | `sync.Once` |
| 并发安全KV | `sync.Map` (读多写少) |
| 临时对象复用 | `sync.Pool` |
| 简单计数 | `sync/atomic` |

### 1.4 Context 使用规范
- 作为函数**第一个参数**，命名 `ctx`
- 不存 struct，不传 nil
- `WithCancel` 手动取消，`WithTimeout` 超时取消
- 父取消 → 所有子取消（树形传播）

---

## 二、并发模式

### 2.1 Worker Pool（最重要）
```
生产者 → Jobs Channel → [Worker1, Worker2, ... WorkerN] → Results Channel
```
- **作用**: 控制并发度，防止goroutine爆炸
- **关键**: 固定Worker数量 + channel做任务队列 + WaitGroup等完成

### 2.2 Fan-out / Fan-in
- **Fan-out**: 一个数据源分发到多个Worker并行处理
- **Fan-in**: 多个Worker结果合并到一个channel
- **场景**: 并行API调用、批量数据处理

### 2.3 Pipeline（流水线）
- 多阶段处理，每阶段通过channel连接
- 瓶颈阶段可单独扩展Worker数
- 内存友好：流式处理，不需全量加载

### 2.4 限流算法对比
| 算法 | 允许突发 | 输出平滑 | 适用 |
|------|---------|---------|------|
| 令牌桶 | ✓ | ✗ | API限流（最常用） |
| 漏桶 | ✗ | ✓ | 消息推送 |
| 滑动窗口 | ✗ | ✗ | 精确计数 |

---

## 三、高并发HTTP服务

### 3.1 Server超时配置（生产必备）
| 参数 | 推荐值 | 说明 |
|------|--------|------|
| ReadTimeout | 5s | 读取请求最大时间 |
| WriteTimeout | 10s | 写入响应最大时间 |
| IdleTimeout | 120s | Keep-Alive空闲超时 |
| ReadHeaderTimeout | 2s | 读取请求头超时 |

### 3.2 中间件洋葱模型
```
请求 → Logger → Recovery → RateLimit → Auth → Handler
响应 ← Logger ← Recovery ← RateLimit ← Auth ← Handler
```
**必备中间件**: 日志、Panic恢复、限流、鉴权、跨域

### 3.3 优雅关闭
1. 监听 SIGINT/SIGTERM
2. 停止接受新连接
3. 等待已有请求完成（设超时）
4. 关闭数据库连接等资源

---

## 四、消息队列

### 核心价值
1. **解耦**: 生产者不关心谁消费
2. **削峰填谷**: 高峰流量进队列，后端匀速消费
3. **最终一致性**: 下游故障不丢消息

### 消息队列 vs 发布订阅
| 模式 | 消息分发 | 场景 |
|------|---------|------|
| 消息队列 | 一条消息一个消费者 | 任务分发、订单处理 |
| 发布订阅 | 一条消息所有订阅者 | 事件通知、配置同步 |

### 技术选型
| 方案 | 特点 | 适用 |
|------|------|------|
| Redis Stream | 轻量 | 中小规模 |
| NATS | 高性能、云原生 | 微服务通信 |
| Kafka | 高吞吐、持久化 | 大数据、日志 |
| RabbitMQ | 功能丰富 | 企业级 |

---

## 五、缓存策略

### 缓存三大问题

| 问题 | 原因 | 防护方案 |
|------|------|---------|
| **穿透** | 查不存在的数据 | 空值缓存 + 布隆过滤器 |
| **雪崩** | 大量key同时过期 | 随机TTL + 多级缓存 |
| **击穿** | 热点key过期 | SingleFlight + 互斥锁 |

### SingleFlight 原理
相同key的并发请求只执行一次函数，其余等待并共享结果。

### Cache Aside 模式（最常用）
- **读**: 缓存命中→返回；未命中→查DB→写缓存→返回
- **写**: 更新DB→**删除**缓存（不是更新！）

---

## 六、微服务架构

### 6.1 服务发现
- 服务启动注册、定期心跳、注册中心健康检查
- 调用方通过服务名获取实例列表
- 生产方案: etcd / Consul / Nacos

### 6.2 负载均衡选择
| 算法 | 适用 |
|------|------|
| 轮询 | 节点性能一致 |
| 加权轮询 | 节点性能不一致 |
| 一致性哈希 | 缓存场景（相同key→相同节点） |
| 最少连接 | 长连接场景 |

### 6.3 熔断器三态
```
CLOSED(正常) → 连续失败达阈值 → OPEN(熔断)
OPEN → 超时后 → HALF-OPEN(探测)
HALF-OPEN → 探测成功 → CLOSED
HALF-OPEN → 探测失败 → OPEN
```

### 6.4 链路追踪
- **TraceID**: 一次请求的唯一标识
- **SpanID**: 每个操作的标识
- 生产工具: OpenTelemetry + Jaeger/Zipkin

---

## 七、高可用设计

### 7.1 重试策略
| 策略 | 特点 | 推荐场景 |
|------|------|---------|
| 固定间隔 | 简单 | 重试少的场景 |
| 指数退避 | 给对方恢复时间 | 调用外部服务 |
| **指数退避+抖动** | **避免重试风暴** | **高并发（推荐）** |

**注意**: 只重试临时性错误（超时、服务不可用），不重试永久性错误（参数错、权限不足）

### 7.2 优雅降级
- 核心功能不降级（下单、支付）
- 非核心可降级（推荐、评论、积分）
- 降级方案提前设计，支持动态开关

### 7.3 幂等设计
| 方案 | 原理 |
|------|------|
| Token去重 | 客户端UUID + Redis SETNX |
| 数据库唯一索引 | 订单号唯一约束 |
| 状态机 | 状态单向流转 |
| 乐观锁 | version字段检查 |

### 7.4 健康检查
- **Liveness**: 进程活着吗？失败→重启
- **Readiness**: 能接流量吗？失败→摘除
- 数据库断了→Readiness=false（摘流量），Liveness=true（不重启）

---

## 八、性能优化

### 8.1 pprof 分析流程
```bash
# 引入: import _ "net/http/pprof"
# CPU分析: go tool pprof http://localhost:6060/debug/pprof/profile?seconds=10
# 内存分析: go tool pprof http://localhost:6060/debug/pprof/heap
# 火焰图: go tool pprof -http=:8081 <profile>
```

### 8.2 内存优化清单
- ✅ slice/map 预分配容量
- ✅ strings.Builder 拼接字符串
- ✅ 结构体字段按大小排列（减少padding）
- ✅ sync.Pool 复用临时对象
- ✅ 减少逃逸到堆上的分配

### 8.3 锁优化选择
| 场景 | 方案 | 性能 |
|------|------|------|
| 简单计数 | `atomic` | ⭐⭐⭐⭐⭐ |
| 读多写少 | `sync.RWMutex` | ⭐⭐⭐⭐ |
| 高并发写map | 分段锁 | ⭐⭐⭐⭐ |
| 复杂临界区 | `sync.Mutex` | ⭐⭐⭐ |

**原则**: 锁粒度越小越好，不要在锁内做IO

---

## 九、秒杀系统设计要点

### 多级过滤
```
10万请求 → 限流(1000) → 本地缓存(500) → Redis预减(100) → MQ(100) → DB(100)
```

### 防超卖三道防线
1. Redis DECR 原子减库存
2. 数据库乐观锁 `WHERE count > 0`
3. 消息队列顺序消费

### 关键技术
- 库存预热（DB→Redis→本地缓存）
- 令牌桶限流
- 幂等Token防重复下单
- 异步MQ削峰
- Worker Pool消费订单

---

## 十、中间件实战 (Redis + NATS)

### 10.1 Redis 五大用法
| 用法 | 命令 | 高并发中的角色 |
|------|------|---------------|
| 缓存 | `SET key val EX 300` | 减少DB查询 |
| 分布式锁 | `SET lock owner NX EX 10` | 多实例互斥 |
| 计数器/限流 | `INCR key` + `EXPIRE` | API限流 |
| 库存预减 | `DECR stock_key` | 秒杀防超卖 |
| 排行榜 | `ZADD` / `ZREVRANGE` | 实时排名 |

### Redis 关键注意
- **分布式锁必须设TTL**，防止死锁
- **释放锁用Lua脚本验证owner**，不能直接DEL
- **空值缓存防穿透**，`SET key NULL EX 60`
- **连接池配置**: `PoolSize`=20, `MinIdleConns`=5

### 10.2 NATS 三种模式
| 模式 | 特点 | 场景 |
|------|------|------|
| Pub/Sub | 广播，所有订阅者收到 | 配置变更通知 |
| Queue Group | 负载均衡，组内只一个收到 | 订单处理 |
| Request/Reply | 同步RPC | 微服务调用 |

### Docker 启动命令
```bash
cd ch10-middleware-practice
docker-compose up -d        # 启动 Redis + NATS
docker-compose down         # 停止
docker exec go-redis redis-cli monitor  # 实时监控Redis
# http://localhost:8222     # NATS监控面板
```

---

## 十一、高并发系统设计口诀

### 高并发三板斧
1. **缓存** — 减少数据库压力
2. **异步** — 消息队列削峰填谷
3. **限流** — 保护系统不被打垮

### 高可用四要素
1. **冗余** — 多实例部署
2. **熔断** — 快速失败防雪崩
3. **降级** — 核心优先
4. **监控** — 全链路可观测

### 性能优化四步法
1. **Benchmark** — 先测量，不要猜
2. **Profile** — pprof找热点
3. **Optimize** — 针对性优化
4. **Verify** — 优化后再次测量

### 微服务设计原则
- 单一职责，服务粒度适中
- 接口幂等，容错重试
- 异步解耦，事件驱动
- 可观测: 日志 + 指标 + 链路追踪
