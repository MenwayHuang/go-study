package main

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

func RunSyncDemo() {
	fmt.Println("--- 示例1: 竞态条件问题 (Race Condition) ---")
	raceConditionDemo()

	fmt.Println("\n--- 示例2: Mutex 互斥锁 ---")
	mutexDemo()

	fmt.Println("\n--- 示例3: RWMutex 读写锁 ---")
	rwMutexDemo()

	fmt.Println("\n--- 示例4: sync.Once 单次执行 ---")
	onceDemo()

	fmt.Println("\n--- 示例5: sync.Map 并发安全Map ---")
	syncMapDemo()

	fmt.Println("\n--- 示例6: atomic 原子操作 ---")
	atomicDemo()
}

// raceConditionDemo 展示数据竞争问题
// 关键点: 多个goroutine同时读写共享变量，结果不可预测
// 检测方法: go run -race main.go
func raceConditionDemo() {
	counter := 0
	var wg sync.WaitGroup

	for i := 0; i < 1000; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			counter++ // 关键点: 这不是原子操作！实际是 读->改->写 三步
		}()
	}
	wg.Wait()
	// 关键点: 结果几乎不会是1000，因为存在数据竞争
	fmt.Printf("  期望: 1000, 实际: %d (存在数据竞争!)\n", counter)
}

// mutexDemo 用互斥锁解决数据竞争
// 关键点:
//   - Lock/Unlock 必须成对出现，推荐用 defer Unlock
//   - 锁的粒度越小越好，只锁必要的代码
//   - 不要在锁内做耗时操作（如网络IO），会严重影响并发性能
func mutexDemo() {
	var mu sync.Mutex
	counter := 0
	var wg sync.WaitGroup

	for i := 0; i < 1000; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mu.Lock()
			counter++
			mu.Unlock()
		}()
	}
	wg.Wait()
	fmt.Printf("  Mutex保护后: 期望: 1000, 实际: %d ✓\n", counter)
}

// rwMutexDemo 读写锁：读多写少场景的性能优化
// 关键点:
//   - 多个读操作可以同时进行（RLock 不互斥）
//   - 写操作独占锁（Lock 与所有读写互斥）
//   - 适用场景: 配置读取、缓存查询等读多写少的场景
func rwMutexDemo() {
	var rw sync.RWMutex
	data := map[string]string{"key": "value"}
	var wg sync.WaitGroup

	// 模拟10个并发读，只需要 RLock
	start := time.Now()
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			rw.RLock() // 读锁，多个goroutine可以同时持有
			defer rw.RUnlock()
			_ = data["key"]
			time.Sleep(10 * time.Millisecond) // 模拟读操作
		}(i)
	}
	wg.Wait()
	readTime := time.Since(start)

	// 模拟10个串行写，需要 Lock
	start = time.Now()
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			rw.Lock() // 写锁，独占
			defer rw.Unlock()
			data["key"] = fmt.Sprintf("value-%d", id)
			time.Sleep(10 * time.Millisecond) // 模拟写操作
		}(i)
	}
	wg.Wait()
	writeTime := time.Since(start)

	// 关键点: 读操作并发执行，所以总时间约等于单次时间
	//         写操作串行执行，所以总时间约等于N倍单次时间
	fmt.Printf("  10个并发读耗时: %v (并行)\n", readTime)
	fmt.Printf("  10个并发写耗时: %v (串行)\n", writeTime)
}

// onceDemo sync.Once 保证函数只执行一次
// 关键点:
//   - 常用于单例模式、一次性初始化（如数据库连接池）
//   - 即使多个goroutine同时调用，也只会执行一次
//   - 执行函数的goroutine完成前，其他goroutine会等待
func onceDemo() {
	var once sync.Once
	var wg sync.WaitGroup

	initFunc := func() {
		fmt.Println("  初始化操作（只执行一次）")
	}

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			once.Do(initFunc) // 只有第一个到达的goroutine会执行
			fmt.Printf("  goroutine %d: 初始化已完成，继续工作\n", id)
		}(i)
	}
	wg.Wait()
}

// syncMapDemo sync.Map 适用于读多写少的并发场景
// 关键点:
//   - 不需要额外的锁，内部已做优化
//   - 适合: key 相对稳定，读远多于写
//   - 不适合: 频繁写入新key的场景（此时普通map+mutex更快）
//   - 无法获取 len()，需要 Range 遍历计数
func syncMapDemo() {
	var m sync.Map

	// Store 存储
	m.Store("user:1", "Alice")
	m.Store("user:2", "Bob")
	m.Store("user:3", "Charlie")

	// Load 读取
	if val, ok := m.Load("user:1"); ok {
		fmt.Printf("  Load: user:1 = %s\n", val)
	}

	// LoadOrStore 不存在则存储
	actual, loaded := m.LoadOrStore("user:4", "David")
	fmt.Printf("  LoadOrStore: user:4 = %s, 已存在: %v\n", actual, loaded)

	// Range 遍历
	fmt.Print("  所有用户: ")
	m.Range(func(key, value any) bool {
		fmt.Printf("%s=%s ", key, value)
		return true // 返回false提前终止遍历
	})
	fmt.Println()
}

// atomicDemo 原子操作：最轻量的并发安全方案
// 关键点:
//   - 比 Mutex 性能好很多（硬件级别的原子指令）
//   - 只适用于简单的数值操作（加减、存取、CAS）
//   - 复杂操作（如多个变量联合修改）还是得用 Mutex
func atomicDemo() {
	var counter int64
	var wg sync.WaitGroup

	for i := 0; i < 1000; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			atomic.AddInt64(&counter, 1) // 原子自增
		}()
	}
	wg.Wait()
	fmt.Printf("  atomic 计数器: %d (精确无误)\n", atomic.LoadInt64(&counter))

	// CAS (Compare And Swap) 操作
	// 关键点: CAS 是无锁并发的核心，很多并发数据结构都基于CAS
	var val int64 = 100
	swapped := atomic.CompareAndSwapInt64(&val, 100, 200) // 如果val==100，则设为200
	fmt.Printf("  CAS: 旧值100 -> 新值200, 成功: %v, 当前值: %d\n", swapped, val)
}
