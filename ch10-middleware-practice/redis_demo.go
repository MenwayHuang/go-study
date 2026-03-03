package main

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

// ============================================================
// Redis 实战 — 高并发系统中的 5 大用法
// ============================================================
//
// 前置条件: docker-compose up -d (启动Redis)
// 观测方式: 另开终端执行 docker exec -it go-redis redis-cli monitor
//          可以实时看到所有 Redis 命令

func newRedisClient() *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr:         "localhost:6379",
		Password:     "",
		DB:           0,
		PoolSize:     20,              // 连接池大小 (生产根据并发量调整)
		MinIdleConns: 5,               // 最小空闲连接 (避免冷启动延迟)
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
	})
}

func RunRedisDemo() {
	client := newRedisClient()
	ctx := context.Background()

	// 测试连接
	if err := client.Ping(ctx).Err(); err != nil {
		fmt.Printf("❌ Redis连接失败: %v\n", err)
		fmt.Println("   请先运行: docker-compose up -d")
		return
	}
	fmt.Println("✅ Redis连接成功")

	// 清理测试数据
	client.FlushDB(ctx)

	fmt.Println("\n--- 用法1: 缓存 (最基本) ---")
	cacheDemo(client, ctx)

	fmt.Println("\n--- 用法2: 分布式锁 ---")
	distributedLockDemo(client, ctx)

	fmt.Println("\n--- 用法3: 计数器 + 限流 ---")
	counterDemo(client, ctx)

	fmt.Println("\n--- 用法4: 排行榜 (Sorted Set) ---")
	leaderboardDemo(client, ctx)

	client.FlushDB(ctx) // 清理
	client.Close()
}

// ============================================================
// 用法1: 缓存
// ============================================================
// 关键点:
//   SET key value EX seconds    — 写缓存 + 设过期时间
//   GET key                      — 读缓存
//   缓存命中 → 直接返回 (微秒级)
//   缓存未命中 → 查DB → 写缓存 → 返回

func cacheDemo(client *redis.Client, ctx context.Context) {
	// 模拟: 查询用户信息
	userKey := "cache:user:1001"

	// 第一次: 缓存未命中
	val, err := client.Get(ctx, userKey).Result()
	if err == redis.Nil {
		fmt.Println("  缓存未命中 → 查数据库")
		// 模拟查DB
		userData := `{"id":1001,"name":"张三","level":"VIP"}`
		// 写入缓存，TTL=5分钟
		client.Set(ctx, userKey, userData, 5*time.Minute)
		fmt.Printf("  已写入缓存: %s (TTL=5m)\n", userData)
	}

	// 第二次: 缓存命中
	val, err = client.Get(ctx, userKey).Result()
	if err == nil {
		fmt.Printf("  缓存命中 ✓: %s\n", val)
	}

	// 查看TTL
	ttl := client.TTL(ctx, userKey).Val()
	fmt.Printf("  剩余TTL: %v\n", ttl)

	// 关键点: 空值缓存防穿透
	emptyKey := "cache:user:99999"
	client.Set(ctx, emptyKey, "NULL", 1*time.Minute) // 不存在的数据缓存空值
	fmt.Println("  空值缓存(防穿透): cache:user:99999 = NULL (TTL=1m)")
}

// ============================================================
// 用法2: 分布式锁
// ============================================================
// 关键点:
//   SET lock_key owner_id NX EX 10
//   NX = 只在key不存在时设置 (保证互斥)
//   EX = 设过期时间 (防止死锁: 持锁进程崩了，锁自动释放)
//
// 生产注意:
//   - 必须设过期时间! 否则进程崩了锁永远不释放
//   - 释放锁时要验证 owner，防止释放别人的锁
//   - 生产推荐用 Redlock 或 Redisson

func distributedLockDemo(client *redis.Client, ctx context.Context) {
	lockKey := "lock:order:12345"
	ownerID := "worker-1"

	// 尝试获取锁 (NX = 不存在才设置, EX = 10秒过期)
	acquired, err := client.SetNX(ctx, lockKey, ownerID, 10*time.Second).Result()
	if err != nil {
		fmt.Printf("  获取锁失败: %v\n", err)
		return
	}

	if acquired {
		fmt.Printf("  %s 获取锁成功 ✓ (TTL=10s)\n", ownerID)

		// 模拟处理业务
		time.Sleep(100 * time.Millisecond)
		fmt.Println("  业务处理完成")

		// 释放锁: 必须验证owner! 用Lua脚本保证原子性
		// 关键点: 不能直接 DEL，可能删掉别人的锁
		luaScript := `
			if redis.call("GET", KEYS[1]) == ARGV[1] then
				return redis.call("DEL", KEYS[1])
			else
				return 0
			end`
		result, _ := client.Eval(ctx, luaScript, []string{lockKey}, ownerID).Int()
		if result == 1 {
			fmt.Println("  锁已释放 ✓")
		}
	}

	// 模拟: 另一个Worker尝试获取锁
	acquired2, _ := client.SetNX(ctx, lockKey, "worker-2", 10*time.Second).Result()
	fmt.Printf("  worker-2 获取锁: %v (前一个已释放，所以能拿到)\n", acquired2)
	client.Del(ctx, lockKey)

	// 并发争锁演示
	fmt.Println("  --- 10个goroutine争锁 ---")
	var wg sync.WaitGroup
	var successCount int32
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			ok, _ := client.SetNX(ctx, "lock:concurrent", fmt.Sprintf("g-%d", id), 5*time.Second).Result()
			if ok {
				atomic.AddInt32(&successCount, 1)
				fmt.Printf("  goroutine-%d 抢到锁 ✓\n", id)
				time.Sleep(50 * time.Millisecond)
				client.Del(ctx, "lock:concurrent")
			}
		}(i)
	}
	wg.Wait()
	fmt.Printf("  10个goroutine争锁, %d个成功获取过锁\n", successCount)
}

// ============================================================
// 用法3: 计数器 + 限流
// ============================================================
// 关键点:
//   INCR key     — 原子自增 (类似 atomic.AddInt64)
//   EXPIRE key 1 — 1秒后自动清零
//   组合实现: 每秒最多N次请求 (简易限流)

func counterDemo(client *redis.Client, ctx context.Context) {
	// 基本计数器
	counterKey := "counter:page_views"
	for i := 0; i < 100; i++ {
		client.Incr(ctx, counterKey)
	}
	count, _ := client.Get(ctx, counterKey).Int()
	fmt.Printf("  页面访问量: %d\n", count)

	// 简易限流: 每秒最多10次
	fmt.Println("  --- 简易限流 (10 req/s) ---")
	rateKey := "rate:api:user1001"
	allowed := 0
	denied := 0

	for i := 0; i < 20; i++ {
		// INCR + EXPIRE 实现简易限流
		current, _ := client.Incr(ctx, rateKey).Result()
		if current == 1 {
			client.Expire(ctx, rateKey, 1*time.Second) // 第一次设置过期
		}
		if current <= 10 {
			allowed++
		} else {
			denied++
		}
	}
	fmt.Printf("  20次请求: 允许=%d, 拒绝=%d\n", allowed, denied)

	// 关键点: 生产中更精确的限流用 Redis + Lua 滑动窗口
	fmt.Println("  (生产中用Lua脚本实现滑动窗口限流，更精确)")
}

// ============================================================
// 用法4: 排行榜 (Sorted Set)
// ============================================================
// 关键点:
//   ZADD key score member  — 添加/更新分数
//   ZREVRANGE key 0 N      — 获取TOP N (从高到低)
//   ZINCRBY key delta member — 增加分数
//   时间复杂度: O(log N)，百万数据也很快

func leaderboardDemo(client *redis.Client, ctx context.Context) {
	boardKey := "leaderboard:game"

	// 添加玩家分数
	players := []redis.Z{
		{Score: 1500, Member: "玩家A"},
		{Score: 2200, Member: "玩家B"},
		{Score: 1800, Member: "玩家C"},
		{Score: 3100, Member: "玩家D"},
		{Score: 950, Member: "玩家E"},
		{Score: 2700, Member: "玩家F"},
	}
	client.ZAdd(ctx, boardKey, players...)

	// 获取 TOP 3
	top3, _ := client.ZRevRangeWithScores(ctx, boardKey, 0, 2).Result()
	fmt.Println("  🏆 排行榜 TOP 3:")
	for i, z := range top3 {
		fmt.Printf("    第%d名: %s (分数: %.0f)\n", i+1, z.Member, z.Score)
	}

	// 更新分数 (玩家E加了2000分)
	client.ZIncrBy(ctx, boardKey, 2000, "玩家E")
	fmt.Println("\n  玩家E 得分+2000 后:")
	top3, _ = client.ZRevRangeWithScores(ctx, boardKey, 0, 2).Result()
	for i, z := range top3 {
		fmt.Printf("    第%d名: %s (分数: %.0f)\n", i+1, z.Member, z.Score)
	}

	// 查某个玩家排名
	rank, _ := client.ZRevRank(ctx, boardKey, "玩家C").Result()
	fmt.Printf("\n  玩家C 当前排名: 第%d名\n", rank+1)
}
