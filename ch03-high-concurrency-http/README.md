# 第3章：高并发 HTTP 服务

## 🎯 学习目标

构建一个生产级别的高并发HTTP服务，掌握中间件链、连接池、超时控制、优雅关闭等核心技术。

## 📚 知识点总览

```
高并发HTTP服务
├── 3.1 HTTP Server 调优
│   ├── 超时参数配置 (ReadTimeout/WriteTimeout/IdleTimeout)
│   ├── 最大连接数控制
│   └── Keep-Alive 长连接
├── 3.2 中间件链 (Middleware Chain)
│   ├── 什么是中间件
│   ├── 洋葱模型执行顺序
│   ├── 常用中间件: 日志/恢复/限流/跨域/鉴权
│   └── 中间件的组合复用
├── 3.3 优雅关闭 (Graceful Shutdown)
│   ├── 为什么需要优雅关闭
│   ├── 信号监听
│   └── 等待请求处理完成后再退出
├── 3.4 HTTP Client 连接池
│   ├── 默认 Client 的问题
│   ├── Transport 连接池配置
│   └── 超时和重试
└── 3.5 并发安全的请求处理
    ├── 共享状态管理
    └── 请求级别的 Context
```

---

## 3.1 HTTP Server 超时配置 (面试高频 + 生产必备)

```
               ┌──────────────────────────────────────────┐
               │          完整的请求生命周期                 │
               │                                          │
  Client ──→ [ReadHeaderTimeout] ──→ [ReadTimeout]        │
               │   读取请求头          读取请求体            │
               │                                          │
             [Handler处理请求]                              │
               │                                          │
             [WriteTimeout] ──→ Response ──→ Client       │
               │   写入响应                                │
               │                                          │
             [IdleTimeout]                                │
               │   Keep-Alive 空闲等待下一个请求            │
               └──────────────────────────────────────────┘
```

**关键参数:**
| 参数 | 推荐值 | 说明 |
|------|--------|------|
| ReadTimeout | 5s | 读取请求的最大时间（包括body） |
| WriteTimeout | 10s | 写入响应的最大时间 |
| IdleTimeout | 120s | Keep-Alive 空闲超时 |
| MaxHeaderBytes | 1MB | 请求头最大大小 |
| ReadHeaderTimeout | 2s | 只读请求头的超时 |

### 超时参数原理详解

#### 为什么需要超时？

没有超时的 HTTP Server 会面临以下风险：
1. **慢客户端攻击 (Slowloris)**：攻击者故意缓慢发送请求，占用连接不释放
2. **资源耗尽**：大量慢连接会耗尽服务器的文件描述符和内存
3. **级联故障**：下游服务慢时，没有超时会导致请求堆积，最终 OOM

#### 各超时参数的精确含义

```
时间线：
  连接建立 ──→ 读取请求头 ──→ 读取请求体 ──→ Handler处理 ──→ 写入响应 ──→ 空闲等待
           │←ReadHeaderTimeout→│
           │←─────────── ReadTimeout ───────────→│
                                              │←── WriteTimeout ──→│
                                                                  │←─ IdleTimeout ─→│
```

- **ReadHeaderTimeout**: 从连接建立到读完请求头的最大时间
  - 防护：慢速发送请求头的攻击
  - 建议：设置较短（1-5s），请求头通常很小

- **ReadTimeout**: 从连接建立到读完整个请求（含body）的最大时间
  - 防护：慢速上传攻击
  - 注意：包含 ReadHeaderTimeout，所以要 >= ReadHeaderTimeout
  - 建议：根据最大允许的上传文件大小和网速估算

- **WriteTimeout**: 从读完请求到写完响应的最大时间
  - 防护：慢速读取响应的客户端
  - 注意：**包含 Handler 处理时间**！如果 Handler 本身需要 5s，WriteTimeout 必须 > 5s
  - 建议：Handler最大耗时 + 网络传输时间 + buffer

- **IdleTimeout**: Keep-Alive 连接空闲等待下一个请求的最大时间
  - 作用：复用 TCP 连接，减少握手开销
  - 建议：120s 是常见值，太短会频繁建连，太长会占用资源

#### 常见错误配置

```go
// ❌ 错误：WriteTimeout < Handler处理时间
server := &http.Server{
    WriteTimeout: 5 * time.Second,  // 只给5秒
}
// 如果 Handler 需要 10 秒处理，客户端会收到连接中断

// ✅ 正确：WriteTimeout 要覆盖 Handler 最大耗时
server := &http.Server{
    WriteTimeout: 30 * time.Second,  // 给足够的时间
}
```

---

## 3.2 中间件 — 洋葱模型

```
  请求 ──→ [Logger] ──→ [Recovery] ──→ [RateLimit] ──→ [Auth] ──→ Handler
  响应 ←── [Logger] ←── [Recovery] ←── [RateLimit] ←── [Auth] ←── Handler

  执行顺序 (洋葱模型):
  Logger.Before → Recovery.Before → RateLimit.Before → Auth.Before
                                                       → Handler
  Logger.After  ← Recovery.After  ← RateLimit.After  ← Auth.After
```

**中间件的本质**: `func(next http.Handler) http.Handler`

### 洋葱模型原理详解

#### 为什么叫"洋葱模型"？

```
        ┌─────────────────────────────────────┐
        │  Logger                             │
        │  ┌─────────────────────────────┐    │
        │  │  Recovery                   │    │
        │  │  ┌─────────────────────┐    │    │
        │  │  │  RateLimit          │    │    │
        │  │  │  ┌─────────────┐    │    │    │
        │  │  │  │   Handler   │    │    │    │
        │  │  │  └─────────────┘    │    │    │
        │  │  └─────────────────────┘    │    │
        │  └─────────────────────────────┘    │
        └─────────────────────────────────────┘

请求从外向内穿透每一层，响应从内向外穿透每一层
```

#### 中间件的代码结构

```go
func SomeMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // ===== Before: 请求进入时执行 =====
        // 例如：记录开始时间、检查权限、限流判断
        
        next.ServeHTTP(w, r)  // 调用下一层（核心！）
        
        // ===== After: 响应返回时执行 =====
        // 例如：记录耗时、记录响应状态码
    })
}
```

#### 中间件链的组装原理

```go
// Chain(handler, A, B, C) 的结果是: A(B(C(handler)))
func ChainMiddleware(handler http.Handler, middlewares ...Middleware) http.Handler {
    // 从后往前包裹，这样第一个中间件在最外层
    for i := len(middlewares) - 1; i >= 0; i-- {
        handler = middlewares[i](handler)
    }
    return handler
}
```

**执行顺序**：
- 请求：A.Before → B.Before → C.Before → Handler
- 响应：Handler → C.After → B.After → A.After

#### 常见中间件及其位置

| 中间件 | 位置 | 原因 |
|--------|------|------|
| Logger | 最外层 | 记录所有请求，包括被其他中间件拦截的 |
| Recovery | 第二层 | 捕获所有 panic，防止服务崩溃 |
| RateLimit | 中间 | 在鉴权前限流，减少无效计算 |
| Auth | 靠近 Handler | 鉴权通过后才执行业务逻辑 |

#### 中间件的"短路"

中间件可以不调用 `next.ServeHTTP()`，直接返回响应，这叫"短路"：

```go
func RateLimitMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if !limiter.Allow() {
            w.WriteHeader(http.StatusTooManyRequests)
            return  // 短路！不调用 next，后续中间件和 Handler 都不执行
        }
        next.ServeHTTP(w, r)
    })
}
```

---

## 3.3 优雅关闭

```
  正常运行 ──→ 收到SIGTERM ──→ 停止接受新连接 ──→ 等待已有请求完成 ──→ 退出
                                     │
                             超时(如30s)后强制退出
```

**为什么需要**: K8s/Docker 发送 SIGTERM 后等待一段时间再 SIGKILL，优雅关闭确保：
- 正在处理的请求能正常响应
- 数据库连接正确关闭
- 缓冲区数据刷盘

### 实际项目常见疑问与要点

#### Shutdown 到底保证了什么？

`server.Shutdown(ctx)` 的关键语义是：

- **停止接受新连接/新请求**（已有连接会被标记为要关闭）。
- **等待正在处理的请求返回**（直到 handler 返回）。
- **ctx 超时后退出并返回错误**。

但它**不会自动帮你**：

- 停止你在 handler 里启动的后台 goroutine（需要你用 `context`/close 信号自己收敛）。
- 等待你自己的队列/worker pool/drain 逻辑（需要额外的 `WaitGroup` 或组件级 Stop()）。

#### 如何感知“客户端断开/服务关闭”？

在 handler 里优先使用 **请求级 `r.Context()`**：

- 客户端断开连接
- 服务进入 shutdown

都会触发 `r.Context().Done()`，适合用在慢请求、下游调用、队列等待等场景，避免无意义的继续计算。

---

## 3.4 HTTP Client 连接池（概念要点）

标准库的 `http.Client` 只有在复用同一个 `Transport` 时才会复用连接。

- **不要每次请求都 new 一个 `http.Client{}`**（会破坏连接复用）。
- 常见做法: 全局/按域名复用 `http.Client`，并配置 `Transport`：
  - `MaxIdleConns` / `MaxIdleConnsPerHost`
  - `IdleConnTimeout`
  - `TLSHandshakeTimeout`

---

## 3.5 中间件链（工程要点）

- **顺序即语义**：Logger/Recovery 往往放最外层，确保任何情况都有日志、有恢复。
- **限流器的范围**：本章示例的限流是“进程内限流”。
  - 多实例部署时，每个实例各自限流，整体限流值会被实例数放大。
  - 分布式限流通常需要 Redis/网关层（Nginx/Envoy/Kong）配合。

---

## 🏃 运行示例

```bash
cd ch03-high-concurrency-http
go run .

# 另一个终端测试:
# 普通请求
curl http://localhost:8080/api/hello

# 压力测试 (观察限流效果)
# Windows PowerShell:
for ($i=0; $i -lt 20; $i++) { Invoke-WebRequest -Uri http://localhost:8080/api/hello -UseBasicParsing | Select-Object StatusCode }

# 测试优雅关闭: 在服务运行时按 Ctrl+C
```
