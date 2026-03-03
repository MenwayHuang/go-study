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
