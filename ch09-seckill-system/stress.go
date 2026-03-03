package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// ============================================================
// 内置压测工具 — 不需要安装第三方工具
// ============================================================
// 使用: go run . -mode stress
//
// 压测流程:
//   1. 先启动秒杀服务: go run . (默认模式)
//   2. 再开一个终端运行: go run . -mode stress
//
// 压测指标:
//   - QPS: 每秒请求数
//   - 成功率/失败率
//   - 平均响应时间 / P99响应时间
//   - 库存消耗情况

type StressConfig struct {
	BaseURL       string // 服务地址
	ProductID     string // 秒杀商品ID
	TotalUsers    int    // 总用户数 (模拟多少人抢)
	Concurrency   int    // 并发数 (同时发起多少请求)
}

type StressResult struct {
	TotalRequests   int64
	SuccessCount    int64
	FailedCount     int64
	TotalDuration   time.Duration
	AvgLatency      time.Duration
	MaxLatency      time.Duration
	MinLatency      time.Duration
	StatusCodes     map[int]int64
	FailReasons     map[string]int64
	Latencies       []time.Duration // 用于计算P99
}

func RunStressTest() {
	config := StressConfig{
		BaseURL:     "http://localhost:8080",
		ProductID:   "PROD-001",
		TotalUsers:  500,   // 500个用户抢购
		Concurrency: 50,    // 50个并发
	}

	fmt.Println("╔══════════════════════════════════════════╗")
	fmt.Println("║       秒杀系统压力测试工具                 ║")
	fmt.Println("╚══════════════════════════════════════════╝")

	// 1. 先查询初始库存
	fmt.Println("\n📦 [Phase 1] 查询初始库存...")
	checkStock(config.BaseURL, config.ProductID)

	// 2. 执行压测
	fmt.Printf("\n🔥 [Phase 2] 开始压测: %d用户, %d并发, 抢购商品 %s\n",
		config.TotalUsers, config.Concurrency, config.ProductID)
	result := runConcurrentBuy(config)

	// 3. 打印压测报告
	printStressReport(result)

	// 4. 查询最终库存
	fmt.Println("\n📦 [Phase 3] 查询最终库存...")
	checkStock(config.BaseURL, config.ProductID)

	// 5. 查询系统统计
	fmt.Println("\n📊 [Phase 4] 服务端统计...")
	checkStats(config.BaseURL)

	// 6. 查询订单数量
	fmt.Println("\n📋 [Phase 5] 订单验证...")
	checkOrders(config.BaseURL)
}

func runConcurrentBuy(config StressConfig) *StressResult {
	result := &StressResult{
		StatusCodes: make(map[int]int64),
		FailReasons: make(map[string]int64),
		MinLatency:  time.Hour, // 初始设为很大
	}

	var mu sync.Mutex
	var wg sync.WaitGroup

	// 令牌通道控制并发度
	semaphore := make(chan struct{}, config.Concurrency)

	start := time.Now()

	for i := 0; i < config.TotalUsers; i++ {
		wg.Add(1)
		go func(userID int) {
			defer wg.Done()
			semaphore <- struct{}{}        // 获取令牌
			defer func() { <-semaphore }() // 释放令牌

			// 构造请求
			body := fmt.Sprintf(`{"product_id":"%s","user_id":"stress-user-%d"}`,
				config.ProductID, userID)

			reqStart := time.Now()
			resp, err := http.Post(
				config.BaseURL+"/seckill/buy",
				"application/json",
				bytes.NewBufferString(body),
			)
			latency := time.Since(reqStart)

			atomic.AddInt64(&result.TotalRequests, 1)

			if err != nil {
				atomic.AddInt64(&result.FailedCount, 1)
				mu.Lock()
				result.FailReasons["network_error"]++
				mu.Unlock()
				return
			}
			defer resp.Body.Close()

			// 读取响应
			respBody, _ := io.ReadAll(resp.Body)
			var respData map[string]interface{}
			json.Unmarshal(respBody, &respData)

			mu.Lock()
			result.Latencies = append(result.Latencies, latency)
			result.StatusCodes[resp.StatusCode]++

			if latency > result.MaxLatency {
				result.MaxLatency = latency
			}
			if latency < result.MinLatency {
				result.MinLatency = latency
			}

			status, _ := respData["status"].(string)
			if status == "success" {
				atomic.AddInt64(&result.SuccessCount, 1)
			} else {
				atomic.AddInt64(&result.FailedCount, 1)
				if reason, ok := respData["reason"].(string); ok {
					result.FailReasons[reason]++
				}
			}
			mu.Unlock()
		}(i)
	}

	wg.Wait()
	result.TotalDuration = time.Since(start)

	// 计算平均延迟
	if len(result.Latencies) > 0 {
		var total time.Duration
		for _, l := range result.Latencies {
			total += l
		}
		result.AvgLatency = total / time.Duration(len(result.Latencies))
	}

	return result
}

func printStressReport(r *StressResult) {
	fmt.Println("\n╔══════════════════════════════════════════╗")
	fmt.Println("║             压 测 报 告                   ║")
	fmt.Println("╠══════════════════════════════════════════╣")
	fmt.Printf("║  总请求数:     %6d                      \n", r.TotalRequests)
	fmt.Printf("║  成功数:       %6d                      \n", r.SuccessCount)
	fmt.Printf("║  失败数:       %6d                      \n", r.FailedCount)
	fmt.Printf("║  总耗时:       %v                    \n", r.TotalDuration.Round(time.Millisecond))
	fmt.Printf("║  QPS:          %.0f req/s               \n",
		float64(r.TotalRequests)/r.TotalDuration.Seconds())
	fmt.Printf("║  平均延迟:     %v                    \n", r.AvgLatency.Round(time.Microsecond))
	fmt.Printf("║  最小延迟:     %v                    \n", r.MinLatency.Round(time.Microsecond))
	fmt.Printf("║  最大延迟:     %v                    \n", r.MaxLatency.Round(time.Microsecond))

	// P99
	if len(r.Latencies) > 0 {
		// 简单排序计算P99
		sorted := make([]time.Duration, len(r.Latencies))
		copy(sorted, r.Latencies)
		sortDurations(sorted)
		p99Index := int(float64(len(sorted)) * 0.99)
		if p99Index >= len(sorted) {
			p99Index = len(sorted) - 1
		}
		fmt.Printf("║  P99延迟:      %v                    \n", sorted[p99Index].Round(time.Microsecond))
	}

	fmt.Println("╠══════════════════════════════════════════╣")
	fmt.Println("║  HTTP状态码分布:                          ")
	for code, count := range r.StatusCodes {
		fmt.Printf("║    %d: %d次                            \n", code, count)
	}
	fmt.Println("╠══════════════════════════════════════════╣")
	fmt.Println("║  失败原因统计:                            ")
	for reason, count := range r.FailReasons {
		fmt.Printf("║    %s: %d次                  \n", reason, count)
	}
	fmt.Println("╚══════════════════════════════════════════╝")

	// 关键验证
	fmt.Println("\n🔍 关键验证:")
	fmt.Printf("  抢购成功数(%d) ≤ 商品库存(100)? ", r.SuccessCount)
	if r.SuccessCount <= 100 {
		fmt.Println("✅ 通过 — 没有超卖!")
	} else {
		fmt.Println("❌ 失败 — 发生了超卖!")
	}
}

// sortDurations 简单冒泡排序 (数据量小，不需要复杂排序)
func sortDurations(d []time.Duration) {
	n := len(d)
	for i := 0; i < n-1; i++ {
		for j := 0; j < n-i-1; j++ {
			if d[j] > d[j+1] {
				d[j], d[j+1] = d[j+1], d[j]
			}
		}
	}
}

func checkStock(baseURL, productID string) {
	resp, err := http.Get(fmt.Sprintf("%s/seckill/stock?product_id=%s", baseURL, productID))
	if err != nil {
		fmt.Printf("  ❌ 请求失败: %v (请确认秒杀服务已启动)\n", err)
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("  %s\n", string(body))
}

func checkStats(baseURL string) {
	resp, err := http.Get(baseURL + "/seckill/stats")
	if err != nil {
		fmt.Printf("  ❌ 请求失败: %v\n", err)
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var data map[string]interface{}
	json.Unmarshal(body, &data)
	pretty, _ := json.MarshalIndent(data, "  ", "  ")
	fmt.Printf("  %s\n", string(pretty))
}

func checkOrders(baseURL string) {
	resp, err := http.Get(baseURL + "/seckill/orders")
	if err != nil {
		fmt.Printf("  ❌ 请求失败: %v\n", err)
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var data map[string]interface{}
	json.Unmarshal(body, &data)
	total, _ := data["total"].(float64)
	fmt.Printf("  订单总数: %.0f\n", total)
	fmt.Printf("  订单数(%0.f) == 抢购成功数? → 验证消息队列是否全部消费完成\n", total)
}
