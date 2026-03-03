package main

import (
	"fmt"
	"hash/fnv"
	"sync"
	"sync/atomic"
	"time"
)

// ============================================================
// 并发锁优化
// ============================================================

func RunLockDemo() {
	fmt.Println("--- 示例1: Mutex vs RWMutex (读多写少场景) ---")
	mutexVsRWMutex()

	fmt.Println("\n--- 示例2: 分段锁 (Sharded Lock) ---")
	shardedLockDemo()

	fmt.Println("\n--- 示例3: atomic vs Mutex (简单计数) ---")
	atomicVsMutex()
}

// mutexVsRWMutex 对比 Mutex 和 RWMutex
// 关键点: 读多写少场景下，RWMutex 性能远优于 Mutex
//   Mutex: 读写都互斥，同时只能一个goroutine操作
//   RWMutex: 读不互斥(多个可以同时读), 写互斥
func mutexVsRWMutex() {
	data := make(map[string]string)
	data["key"] = "value"

	readRatio := 100 // 读写比 100:1
	total := 100_000

	// Mutex
	var mu sync.Mutex
	start := time.Now()
	var wg sync.WaitGroup
	for i := 0; i < total; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if i%readRatio == 0 { // 写
				mu.Lock()
				data["key"] = "new-value"
				mu.Unlock()
			} else { // 读
				mu.Lock()
				_ = data["key"]
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()
	mutexTime := time.Since(start)

	// RWMutex
	var rw sync.RWMutex
	start = time.Now()
	for i := 0; i < total; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if i%readRatio == 0 { // 写
				rw.Lock()
				data["key"] = "new-value"
				rw.Unlock()
			} else { // 读
				rw.RLock()
				_ = data["key"]
				rw.RUnlock()
			}
		}(i)
	}
	wg.Wait()
	rwTime := time.Since(start)

	fmt.Printf("  Mutex:   %v\n", mutexTime)
	fmt.Printf("  RWMutex: %v\n", rwTime)
	fmt.Printf("  读写比 %d:1 时, RWMutex 提升: %.1fx\n",
		readRatio, float64(mutexTime)/float64(rwTime))
}

// ============================================================
// 分段锁 (Sharded Lock / Striped Lock)
// ============================================================
// 关键点:
//   一把大锁 → 所有操作串行 → 并发瓶颈
//   分成N把小锁 → 不同key用不同锁 → 并行度提升N倍
//
//   原理: key hash → 分片编号 → 该分片的锁
//   类似: Java ConcurrentHashMap 的分段锁设计

type ShardedMap struct {
	shards    []*shard
	shardMask uint32
}

type shard struct {
	mu   sync.RWMutex
	data map[string]interface{}
}

func NewShardedMap(shardCount int) *ShardedMap {
	// shardCount 必须是2的幂
	shards := make([]*shard, shardCount)
	for i := range shards {
		shards[i] = &shard{data: make(map[string]interface{})}
	}
	return &ShardedMap{
		shards:    shards,
		shardMask: uint32(shardCount - 1),
	}
}

func (sm *ShardedMap) getShard(key string) *shard {
	h := fnv.New32a()
	h.Write([]byte(key))
	return sm.shards[h.Sum32()&sm.shardMask]
}

func (sm *ShardedMap) Set(key string, value interface{}) {
	s := sm.getShard(key)
	s.mu.Lock()
	s.data[key] = value
	s.mu.Unlock()
}

func (sm *ShardedMap) Get(key string) (interface{}, bool) {
	s := sm.getShard(key)
	s.mu.RLock()
	val, ok := s.data[key]
	s.mu.RUnlock()
	return val, ok
}

func shardedLockDemo() {
	total := 100_000
	var wg sync.WaitGroup

	// 普通 map + 单把锁
	normalMap := make(map[string]int)
	var mu sync.Mutex
	start := time.Now()
	for i := 0; i < total; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := fmt.Sprintf("key-%d", i%1000)
			mu.Lock()
			normalMap[key] = i
			mu.Unlock()
		}(i)
	}
	wg.Wait()
	singleLockTime := time.Since(start)

	// 分段锁 map (16个分片)
	shardedMap := NewShardedMap(16)
	start = time.Now()
	for i := 0; i < total; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := fmt.Sprintf("key-%d", i%1000)
			shardedMap.Set(key, i)
		}(i)
	}
	wg.Wait()
	shardedTime := time.Since(start)

	fmt.Printf("  单把锁:       %v\n", singleLockTime)
	fmt.Printf("  分段锁(16片): %v\n", shardedTime)
	fmt.Printf("  提升: %.1fx\n", float64(singleLockTime)/float64(shardedTime))
	fmt.Println("  (分段越多并行度越高，但也不是越多越好，16-256片通常够用)")
}

// atomicVsMutex 原子操作 vs 锁
// 关键点: 简单的数值操作用 atomic，比 Mutex 快很多
func atomicVsMutex() {
	total := 1_000_000
	var wg sync.WaitGroup

	// Mutex
	var muCounter int64
	var mu sync.Mutex
	start := time.Now()
	for i := 0; i < total; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mu.Lock()
			muCounter++
			mu.Unlock()
		}()
	}
	wg.Wait()
	mutexTime := time.Since(start)

	// Atomic
	var atomicCounter int64
	start = time.Now()
	for i := 0; i < total; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			atomic.AddInt64(&atomicCounter, 1)
		}()
	}
	wg.Wait()
	atomicTime := time.Since(start)

	fmt.Printf("  Mutex:  %v (counter=%d)\n", mutexTime, muCounter)
	fmt.Printf("  Atomic: %v (counter=%d)\n", atomicTime, atomicCounter)
	fmt.Printf("  Atomic 提升: %.1fx\n", float64(mutexTime)/float64(atomicTime))

	fmt.Println("\n  === 锁优化选择指南 ===")
	fmt.Println("  简单计数/标志:   atomic (最快)")
	fmt.Println("  读多写少:       sync.RWMutex")
	fmt.Println("  高并发写map:    分段锁 ShardedMap")
	fmt.Println("  复杂临界区:     sync.Mutex (最安全)")
	fmt.Println("  锁粒度原则:     只锁必要的最小范围，不要在锁内做IO")
}
