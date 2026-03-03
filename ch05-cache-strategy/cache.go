package main

import (
	"container/list"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"
)

// ============================================================
// 带TTL过期的本地缓存
// ============================================================

type cacheItem struct {
	value     interface{}
	expireAt  time.Time
}

// LocalCache 带过期时间的本地缓存
// 关键点:
//   - 使用 sync.RWMutex 实现读写并发安全
//   - 读多写少场景用 RLock 提升性能
//   - 后台定时清理过期key，避免内存泄漏
type LocalCache struct {
	data map[string]*cacheItem
	mu   sync.RWMutex
	stop chan struct{}
}

func NewLocalCache(cleanupInterval time.Duration) *LocalCache {
	c := &LocalCache{
		data: make(map[string]*cacheItem),
		stop: make(chan struct{}),
	}
	// 后台定时清理过期数据
	go c.cleanup(cleanupInterval)
	return c
}

func (c *LocalCache) Set(key string, value interface{}, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data[key] = &cacheItem{
		value:    value,
		expireAt: time.Now().Add(ttl),
	}
}

func (c *LocalCache) Get(key string) (interface{}, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	item, ok := c.data[key]
	if !ok {
		return nil, false
	}
	if time.Now().After(item.expireAt) {
		return nil, false // 已过期
	}
	return item.value, true
}

func (c *LocalCache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.data, key)
}

func (c *LocalCache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.data)
}

func (c *LocalCache) cleanup(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			c.mu.Lock()
			now := time.Now()
			for key, item := range c.data {
				if now.After(item.expireAt) {
					delete(c.data, key)
				}
			}
			c.mu.Unlock()
		case <-c.stop:
			return
		}
	}
}

func (c *LocalCache) Close() {
	close(c.stop)
}

// ============================================================
// LRU 缓存 (Least Recently Used)
// ============================================================
// 关键点: 当缓存容量满时，淘汰最久未使用的数据
// 实现: 双向链表 + HashMap, Get和Put都是O(1)
//
//   HashMap: key → 链表节点指针 (O(1)查找)
//   双向链表: 最近使用的在头部，最久未用的在尾部
//
//   访问/更新 → 移到头部
//   容量满了 → 删除尾部

type LRUCache struct {
	capacity int
	cache    map[string]*list.Element
	list     *list.List // 双向链表
	mu       sync.Mutex
}

type lruEntry struct {
	key   string
	value interface{}
}

func NewLRUCache(capacity int) *LRUCache {
	return &LRUCache{
		capacity: capacity,
		cache:    make(map[string]*list.Element),
		list:     list.New(),
	}
}

func (c *LRUCache) Get(key string) (interface{}, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if elem, ok := c.cache[key]; ok {
		c.list.MoveToFront(elem) // 移到头部 (最近使用)
		return elem.Value.(*lruEntry).value, true
	}
	return nil, false
}

func (c *LRUCache) Put(key string, value interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.cache[key]; ok {
		c.list.MoveToFront(elem)
		elem.Value.(*lruEntry).value = value
		return
	}

	// 容量满了，淘汰最久未用的 (尾部)
	if c.list.Len() >= c.capacity {
		oldest := c.list.Back()
		if oldest != nil {
			c.list.Remove(oldest)
			delete(c.cache, oldest.Value.(*lruEntry).key)
		}
	}

	entry := &lruEntry{key: key, value: value}
	elem := c.list.PushFront(entry)
	c.cache[key] = elem
}

func (c *LRUCache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.list.Len()
}

// ============================================================
// 演示
// ============================================================

func RunLocalCacheDemo() {
	fmt.Println("--- 示例1: 带TTL的本地缓存 ---")
	ttlCacheDemo()

	fmt.Println("\n--- 示例2: LRU缓存 ---")
	lruCacheDemo()
}

func ttlCacheDemo() {
	cache := NewLocalCache(1 * time.Second) // 每秒清理一次
	defer cache.Close()

	// 设置不同TTL的缓存
	cache.Set("user:1", "Alice", 500*time.Millisecond)
	cache.Set("user:2", "Bob", 2*time.Second)

	// 立即读取
	if val, ok := cache.Get("user:1"); ok {
		fmt.Printf("  读取 user:1 = %s ✓\n", val)
	}

	// 等待过期
	time.Sleep(600 * time.Millisecond)
	if _, ok := cache.Get("user:1"); !ok {
		fmt.Println("  user:1 已过期 ✓ (TTL=500ms)")
	}
	if val, ok := cache.Get("user:2"); ok {
		fmt.Printf("  user:2 = %s 仍然有效 ✓ (TTL=2s)\n", val)
	}
}

func lruCacheDemo() {
	cache := NewLRUCache(3) // 容量只有3

	cache.Put("a", 1)
	cache.Put("b", 2)
	cache.Put("c", 3)
	fmt.Printf("  缓存已满 (容量3): a=1, b=2, c=3\n")

	// 访问a，使a成为"最近使用"
	cache.Get("a")

	// 加入d，容量满了 → b是最久未使用的 → 淘汰b
	cache.Put("d", 4)

	if _, ok := cache.Get("b"); !ok {
		fmt.Println("  插入d后，b被淘汰 ✓ (最久未使用)")
	}
	if val, ok := cache.Get("a"); ok {
		fmt.Printf("  a = %v 仍然存在 ✓ (最近访问过)\n", val)
	}
	if val, ok := cache.Get("d"); ok {
		fmt.Printf("  d = %v 新加入 ✓\n", val)
	}

	fmt.Println("  LRU淘汰策略: 容量满时，淘汰最久没被访问的数据")
}

// ============================================================
// 缓存穿透/雪崩/击穿防护
// ============================================================

// simulateDB 模拟数据库查询
func simulateDB(key string) (string, bool) {
	time.Sleep(50 * time.Millisecond) // 模拟数据库延迟
	db := map[string]string{
		"user:1": "Alice",
		"user:2": "Bob",
		"user:3": "Charlie",
	}
	val, ok := db[key]
	return val, ok
}

func RunCacheProblemsDemo() {
	fmt.Println("--- 示例1: 缓存穿透防护 (空值缓存) ---")
	cachePenetrationDemo()

	fmt.Println("\n--- 示例2: 缓存雪崩防护 (随机TTL) ---")
	cacheAvalancheDemo()

	fmt.Println("\n--- 示例3: 缓存击穿防护 (互斥锁) ---")
	cacheBreakdownDemo()
}

// cachePenetrationDemo 缓存穿透防护
// 关键点: 查询不存在的数据时，缓存一个空值，避免每次都打到数据库
func cachePenetrationDemo() {
	cache := NewLocalCache(10 * time.Second)
	defer cache.Close()

	dbQueryCount := 0

	// 带穿透防护的查询函数
	queryWithProtection := func(key string) (string, bool) {
		// 1. 先查缓存
		if val, ok := cache.Get(key); ok {
			if val == "<nil>" {
				return "", false // 空值缓存，直接返回不存在
			}
			return val.(string), true
		}

		// 2. 缓存未命中，查数据库
		dbQueryCount++
		val, exists := simulateDB(key)
		if exists {
			cache.Set(key, val, 5*time.Minute)
		} else {
			// 关键点: 不存在也缓存! 但TTL短一些
			cache.Set(key, "<nil>", 30*time.Second)
		}
		return val, exists
	}

	// 模拟攻击: 查询不存在的key
	for i := 0; i < 5; i++ {
		val, exists := queryWithProtection("user:99999")
		fmt.Printf("  查询 user:99999: 存在=%v, 值=%q\n", exists, val)
	}
	fmt.Printf("  数据库实际查询次数: %d (5次请求只查了1次数据库) ✓\n", dbQueryCount)
}

// cacheAvalancheDemo 缓存雪崩防护
// 关键点: 给TTL加随机偏移，避免大量key同时过期
func cacheAvalancheDemo() {
	cache := NewLocalCache(1 * time.Second)
	defer cache.Close()

	baseTTL := 1 * time.Hour

	// 模拟批量写入缓存
	for i := 0; i < 10; i++ {
		key := fmt.Sprintf("product:%d", i)
		// 关键点: 加随机偏移，每个key的过期时间不同
		randomOffset := time.Duration(rand.Intn(300)) * time.Second
		ttl := baseTTL + randomOffset
		cache.Set(key, fmt.Sprintf("data-%d", i), ttl)
		fmt.Printf("  %s TTL = %v (基础1h + 随机%v)\n", key, ttl, randomOffset)
	}
	fmt.Println("  ✓ 所有key的过期时间分散在 1h~1h5min 之间，避免同时过期")
}

// cacheBreakdownDemo 缓存击穿防护 (互斥锁方式)
// 关键点: 热点key过期时，用锁保证只有一个请求去查数据库
func cacheBreakdownDemo() {
	cache := NewLocalCache(10 * time.Second)
	defer cache.Close()

	var mu sync.Mutex
	dbHits := int64(0)

	// 带互斥锁的查询
	queryWithMutex := func(key string) string {
		// 1. 先查缓存
		if val, ok := cache.Get(key); ok {
			return val.(string)
		}

		// 2. 缓存未命中，加锁查数据库
		mu.Lock()
		defer mu.Unlock()

		// 关键点: 双重检查! 拿到锁后再查一次缓存
		// 因为等待锁期间，可能已经有别的goroutine填充了缓存
		if val, ok := cache.Get(key); ok {
			return val.(string)
		}

		// 3. 确实没有，查数据库
		atomic.AddInt64(&dbHits, 1)
		val, _ := simulateDB(key)
		cache.Set(key, val, 5*time.Minute)
		return val
	}

	// 模拟: 100个并发请求同时查询同一个key
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			queryWithMutex("user:1")
		}()
	}
	wg.Wait()

	fmt.Printf("  100个并发请求, 数据库只被查询了 %d 次 ✓\n", atomic.LoadInt64(&dbHits))
	fmt.Println("  (互斥锁 + 双重检查 防止了缓存击穿)")
}
