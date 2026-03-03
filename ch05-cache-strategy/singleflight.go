package main

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// ============================================================
// SingleFlight — 防缓存击穿的终极武器
// ============================================================
// 原理: 对于相同的 key，多个并发请求只会执行一次函数调用
//       其他请求等待并共享同一个结果
//
// 标准库: golang.org/x/sync/singleflight
// 这里我们自己实现一个，理解原理

// SingleFlight 防击穿组件
type SingleFlight struct {
	mu    sync.Mutex
	calls map[string]*call
}

type call struct {
	wg  sync.WaitGroup
	val interface{}
	err error
}

func NewSingleFlight() *SingleFlight {
	return &SingleFlight{
		calls: make(map[string]*call),
	}
}

// Do 执行函数，相同key的并发调用只执行一次
// 关键点:
//  1. 第一个请求: 创建 call，执行 fn，其他请求等待
//  2. 其他请求: 发现已有 call，等待 wg.Wait() 拿到结果
//  3. 执行完成: 删除 call，返回结果给所有等待者
func (sf *SingleFlight) Do(key string, fn func() (interface{}, error)) (interface{}, error) {
	sf.mu.Lock()

	// 已有相同key的请求在执行，等待结果
	if c, ok := sf.calls[key]; ok {
		sf.mu.Unlock()
		c.wg.Wait() // 等待第一个请求完成
		return c.val, c.err
	}

	// 第一个请求，创建call
	c := &call{}
	c.wg.Add(1)
	sf.calls[key] = c
	sf.mu.Unlock()

	// 执行实际函数
	c.val, c.err = fn()
	c.wg.Done() // 通知所有等待者

	// 清理
	sf.mu.Lock()
	delete(sf.calls, key)
	sf.mu.Unlock()

	return c.val, c.err
}

// ============================================================
// 演示
// ============================================================

func RunSingleFlightDemo() {
	fmt.Println("--- 对比: 无防护 vs SingleFlight ---")

	// 模拟数据库
	var dbQueryCount int64

	queryDB := func(key string) (interface{}, error) {
		atomic.AddInt64(&dbQueryCount, 1)
		time.Sleep(100 * time.Millisecond) // 模拟数据库查询
		return fmt.Sprintf("data-for-%s", key), nil
	}

	// 场景1: 无防护，100个并发全部打到数据库
	fmt.Println("\n  === 无防护 ===")
	atomic.StoreInt64(&dbQueryCount, 0)
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			queryDB("hot-key")
		}()
	}
	wg.Wait()
	fmt.Printf("  100个并发请求, 数据库查询次数: %d 😱\n", atomic.LoadInt64(&dbQueryCount))

	// 场景2: SingleFlight 防护
	fmt.Println("\n  === SingleFlight 防护 ===")
	atomic.StoreInt64(&dbQueryCount, 0)
	sf := NewSingleFlight()

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			val, _ := sf.Do("hot-key", func() (interface{}, error) {
				return queryDB("hot-key")
			})
			_ = val // 所有goroutine都拿到了相同的结果
		}(i)
	}
	wg.Wait()
	fmt.Printf("  100个并发请求, 数据库查询次数: %d 🎉\n", atomic.LoadInt64(&dbQueryCount))
	fmt.Println("  (相同key的并发请求只执行了一次数据库查询，其余共享结果)")

	// 场景3: 不同key不受影响
	fmt.Println("\n  === 不同key各自独立 ===")
	atomic.StoreInt64(&dbQueryCount, 0)
	sf2 := NewSingleFlight()

	keys := []string{"key-a", "key-b", "key-c"}
	for _, key := range keys {
		key := key
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				sf2.Do(key, func() (interface{}, error) {
					return queryDB(key)
				})
			}()
		}
	}
	wg.Wait()
	fmt.Printf("  3个key各10个并发, 数据库查询次数: %d (每个key查1次)\n",
		atomic.LoadInt64(&dbQueryCount))
}
