package main

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// ============================================================
// 用 Redis 改造秒杀库存扣减
// ============================================================
//
// 对比第9章:
//   第9章: atomic.AddInt64(&product.Stock, -1)  ← 单进程，内存操作
//   本章:  redis DECR seckill:stock:PROD-001    ← 多进程共享，分布式
//
// 为什么需要Redis?
//   - 单进程 atomic 只在当前进程有效，多实例部署时各自有各自的库存
//   - Redis 是多进程共享的，所有实例都从同一个 Redis 读写库存
//   - Redis DECR 是原子操作，天然防超卖
//
// 完整流程:
//   1. 秒杀前: DB库存(100) → Redis SET seckill:stock:PROD-001 100 (预热)
//   2. 秒杀中: Redis DECR → 返回剩余库存 → <0则回滚 → 发消息到MQ
//   3. 秒杀后: Redis库存=0, 标记售罄

func RunSeckillRedisDemo() {
	client := newRedisClient()
	ctx := context.Background()

	if err := client.Ping(ctx).Err(); err != nil {
		fmt.Printf("❌ Redis连接失败: %v\n", err)
		fmt.Println("   请先运行: docker-compose up -d")
		return
	}

	fmt.Println("=== Redis 秒杀库存扣减演示 ===")

	// ─── 阶段1: 库存预热 ───
	productKey := "seckill:stock:PROD-001"
	soldOutKey := "seckill:soldout:PROD-001"
	stockCount := 100

	client.Set(ctx, productKey, stockCount, 0)  // 预热库存到Redis
	client.Del(ctx, soldOutKey)                   // 清除售罄标记
	fmt.Printf("\n📦 库存预热: %s = %d\n", productKey, stockCount)

	// ─── 阶段2: 模拟500人并发抢购 ───
	fmt.Println("\n🔥 500个用户并发抢购 (库存100)...")

	var (
		wg           sync.WaitGroup
		successCount int32
		soldOutCount int32
		stockEmpty   int32
	)

	start := time.Now()

	for i := 0; i < 500; i++ {
		wg.Add(1)
		go func(userID int) {
			defer wg.Done()

			// 第1层: 检查售罄标记 (GET 比 DECR 更轻量)
			// 关键点: 售罄后的请求直接在这层拦截，不走 DECR
			exists, _ := client.Exists(ctx, soldOutKey).Result()
			if exists == 1 {
				atomic.AddInt32(&soldOutCount, 1)
				return
			}

			// 第2层: 原子减库存 — Redis DECR (核心!)
			// 关键点: DECR 是原子操作，并发安全，不需要锁
			remaining, err := client.Decr(ctx, productKey).Result()
			if err != nil {
				return
			}

			if remaining < 0 {
				// 库存不足，回滚
				client.Incr(ctx, productKey) // 回滚+1
				// 标记售罄 (后续请求不再走DECR)
				client.Set(ctx, soldOutKey, "1", 0)
				atomic.AddInt32(&stockEmpty, 1)
				return
			}

			// 抢购成功!
			atomic.AddInt32(&successCount, 1)
			// 实际项目: 这里发消息到NATS/Kafka → Worker创建订单
		}(i)
	}

	wg.Wait()
	elapsed := time.Since(start)

	// ─── 阶段3: 查看结果 ───
	finalStock, _ := client.Get(ctx, productKey).Int()

	fmt.Println("\n📊 抢购结果:")
	fmt.Printf("  总请求:     500\n")
	fmt.Printf("  抢购成功:   %d (应该 = %d)\n", successCount, stockCount)
	fmt.Printf("  售罄拦截:   %d (本地售罄标记拦截)\n", soldOutCount)
	fmt.Printf("  库存不足:   %d (DECR后<0，回滚)\n", stockEmpty)
	fmt.Printf("  Redis库存:  %d (应该 = 0)\n", finalStock)
	fmt.Printf("  总耗时:     %v\n", elapsed)

	// 验证
	fmt.Println("\n🔍 关键验证:")
	if successCount == int32(stockCount) {
		fmt.Printf("  ✅ 抢购成功数(%d) == 库存(%d) → 没有超卖!\n", successCount, stockCount)
	} else if successCount < int32(stockCount) {
		fmt.Printf("  ⚠️ 抢购成功数(%d) < 库存(%d) → 有少卖(可接受)\n", successCount, stockCount)
	} else {
		fmt.Printf("  ❌ 抢购成功数(%d) > 库存(%d) → 发生超卖!\n", successCount, stockCount)
	}

	fmt.Println("\n💡 关键点:")
	fmt.Println("  1. Redis DECR 是原子操作，天然防超卖")
	fmt.Println("  2. 售罄标记(soldout key)拦截后续无效请求")
	fmt.Println("  3. 多实例部署时，所有实例共享同一个Redis库存")
	fmt.Println("  4. 对比第9章: atomic只能单进程，Redis支持分布式")

	// Lua脚本版本说明 (更严谨)
	fmt.Println("\n📝 生产推荐: 用Lua脚本保证 检查+扣减 的原子性")
	fmt.Println(`  local stock = redis.call('GET', KEYS[1])
  if tonumber(stock) > 0 then
      redis.call('DECR', KEYS[1])
      return 1  -- 成功
  else
      return 0  -- 售罄
  end`)

	client.FlushDB(ctx)
	client.Close()
}
