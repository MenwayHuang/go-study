# 第8章：性能优化

## 🎯 学习目标

掌握 Go 程序性能分析和优化的核心技术，让程序在高并发下更快、更省内存。

## 📚 知识点总览

```
性能优化
├── 8.1 pprof 性能分析
│   ├── CPU Profiling — 找出CPU热点
│   ├── Memory Profiling — 找出内存分配热点
│   ├── Goroutine Profiling — 发现goroutine泄漏
│   └── 火焰图分析
├── 8.2 sync.Pool — 对象复用，减少GC压力
│   ├── 原理: 临时对象池
│   ├── 适用场景: 频繁创建销毁的临时对象
│   └── 注意事项: Pool不是缓存!
├── 8.3 内存优化
│   ├── 预分配 slice/map 容量
│   ├── strings.Builder 拼接字符串
│   ├── 减少逃逸 (escape analysis)
│   └── 结构体字段对齐 (内存对齐)
├── 8.4 并发优化
│   ├── 减小锁粒度
│   ├── 分段锁 (Sharded Lock)
│   ├── 无锁编程 (atomic/CAS)
│   └── 读写分离 (RWMutex)
└── 8.5 GC 调优
    ├── GOGC 参数
    ├── 减少堆分配
    └── ballast 内存压舱石技巧
```

---

## 8.1 pprof 使用方法

```bash
# 在代码中引入 net/http/pprof (详见示例代码)
# 启动服务后访问:
#   http://localhost:6060/debug/pprof/          # 总览
#   http://localhost:6060/debug/pprof/heap      # 内存
#   http://localhost:6060/debug/pprof/goroutine # goroutine

# 命令行分析:
go tool pprof http://localhost:6060/debug/pprof/profile?seconds=10  # CPU
go tool pprof http://localhost:6060/debug/pprof/heap                 # 内存

# 生成火焰图 (需要安装 graphviz):
go tool pprof -http=:8081 http://localhost:6060/debug/pprof/profile?seconds=10
```

## 8.2 sync.Pool 原理

```
  正常流程 (无Pool):
  请求1: new(Object) → 使用 → GC回收
  请求2: new(Object) → 使用 → GC回收  ← 频繁分配+GC
  请求3: new(Object) → 使用 → GC回收

  使用 sync.Pool:
  请求1: Pool.Get() → 使用 → Pool.Put() → 放回池子
  请求2: Pool.Get() → 复用! → Pool.Put() → 放回池子  ← 零分配
  请求3: Pool.Get() → 复用! → Pool.Put() → 放回池子
```

## 8.3 内存优化关键点

```
  // ❌ 动态扩容，多次内存分配
  s := make([]int, 0)
  for i := 0; i < 10000; i++ { s = append(s, i) }

  // ✅ 预分配，一次分配
  s := make([]int, 0, 10000)
  for i := 0; i < 10000; i++ { s = append(s, i) }
```

```
  // 结构体字段对齐 — 相同字段不同顺序，内存差很大
  // ❌ 48 bytes (有填充)
  type Bad struct {
      a bool    // 1 byte + 7 padding
      b int64   // 8 bytes
      c bool    // 1 byte + 7 padding
      d int64   // 8 bytes
      e bool    // 1 byte + 7 padding
  }

  // ✅ 32 bytes (紧凑排列)
  type Good struct {
      b int64   // 8 bytes
      d int64   // 8 bytes
      a bool    // 1 byte
      c bool    // 1 byte
      e bool    // 1 byte + 5 padding
  }
```

---

## 🏃 运行示例

```bash
cd ch08-performance
go run . -demo pool       # sync.Pool
go run . -demo memory     # 内存优化
go run . -demo lock       # 并发锁优化
go run . -demo pprof      # 启动pprof服务器 (后台运行)
go run . -demo all        # 全部 (不含pprof服务器)
```
