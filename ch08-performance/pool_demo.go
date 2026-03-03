package main

import (
	"bytes"
	"fmt"
	"sync"
	"time"
)

// ============================================================
// sync.Pool — 对象复用，减少GC压力
// ============================================================
// 关键点:
//   - Pool 是临时对象池，不是缓存! GC时池中对象可能被回收
//   - 适用场景: 频繁创建销毁的短生命周期对象 (如 buffer、临时结构体)
//   - 不适用: 需要持久保存的对象
//   - 经典案例: fmt包、encoding/json包内部都大量使用sync.Pool

func RunPoolDemo() {
	fmt.Println("--- 示例1: 无Pool vs 有Pool 性能对比 ---")
	poolBenchmark()

	fmt.Println("\n--- 示例2: bytes.Buffer 对象池 (实战常用) ---")
	bufferPoolDemo()
}

// poolBenchmark 对比有无Pool的性能差异
func poolBenchmark() {
	const iterations = 1_000_000

	// 场景1: 每次 new 一个大对象
	start := time.Now()
	for i := 0; i < iterations; i++ {
		buf := make([]byte, 1024) // 每次分配 1KB
		buf[0] = 1                // 使用一下防止被优化掉
		_ = buf
		// buf 在这里变成垃圾，等GC回收
	}
	noPoolTime := time.Since(start)

	// 场景2: 使用 sync.Pool 复用对象
	pool := &sync.Pool{
		New: func() interface{} {
			return make([]byte, 1024)
		},
	}

	start = time.Now()
	for i := 0; i < iterations; i++ {
		buf := pool.Get().([]byte) // 从池中获取
		buf[0] = 1
		pool.Put(buf) // 用完放回池中
	}
	withPoolTime := time.Since(start)

	fmt.Printf("  无 Pool: %v (每次都分配新内存)\n", noPoolTime)
	fmt.Printf("  有 Pool: %v (复用对象)\n", withPoolTime)
	speedup := float64(noPoolTime) / float64(withPoolTime)
	fmt.Printf("  性能提升: %.1fx\n", speedup)
}

// bufferPoolDemo 实战: bytes.Buffer 对象池
// 关键点: 在HTTP服务中，每个请求都需要Buffer来构建响应
//   高并发下，每秒创建销毁大量Buffer → GC压力大
//   使用Pool复用Buffer → 几乎零分配
func bufferPoolDemo() {
	// 关键点: 使用 sync.Pool 复用 bytes.Buffer
	bufPool := &sync.Pool{
		New: func() interface{} {
			return new(bytes.Buffer)
		},
	}

	// 模拟高并发使用
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			// 从池中获取 buffer
			buf := bufPool.Get().(*bytes.Buffer)

			// 关键点: 使用前必须 Reset! 因为Pool中的对象可能有上次的数据
			buf.Reset()

			// 使用 buffer
			fmt.Fprintf(buf, `{"request_id": %d, "status": "ok"}`, id)

			// 模拟处理
			_ = buf.String()

			// 关键点: 用完放回池中
			// 放回前可以检查容量，太大的不放回（防止内存浪费）
			if buf.Cap() < 64*1024 { // 小于64KB才放回
				bufPool.Put(buf)
			}
		}(i)
	}
	wg.Wait()

	fmt.Println("  100个并发请求完成，Buffer被高效复用 ✓")
	fmt.Println("\n  === sync.Pool 使用规范 ===")
	fmt.Println("  1. Get 后必须 Reset/清零对象")
	fmt.Println("  2. Put 前检查对象大小，太大的不放回")
	fmt.Println("  3. Pool 不是缓存，GC时对象可能被回收")
	fmt.Println("  4. 适用: Buffer、临时结构体、编解码器")
	fmt.Println("  5. 不适用: 数据库连接、需要持久化的对象")
}
