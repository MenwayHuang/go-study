package main

import (
	"fmt"
	"math"
	"time"
)

func RunPipelineDemo() {
	fmt.Println("--- 示例: 三阶段数据处理流水线 ---")
	pipelineDemo()
}

// pipelineDemo 展示Pipeline模式
// 场景模拟: 数据处理流水线
//   Stage1: 生成原始数据
//   Stage2: 数据清洗/转换 (可以并行多个worker)
//   Stage3: 聚合输出结果
//
// 关键点:
//   - 每个阶段通过channel连接，独立并发运行
//   - 瓶颈阶段可以单独扩展worker数量
//   - 整体吞吐量取决于最慢的阶段
//   - 内存友好: 数据流式处理，不需要全部加载到内存
func pipelineDemo() {
	start := time.Now()

	// Stage 1: 生成数据
	numbers := generate(1, 20)

	// Stage 2: 数据处理 (计算是否为质数 — 模拟CPU密集型操作)
	// 关键点: 可以启动多个Stage2 worker来加速瓶颈阶段
	processed := process(numbers)

	// Stage 3: 消费结果
	primeCount := 0
	totalCount := 0
	for result := range processed {
		if result.IsPrime {
			fmt.Printf("  ✓ %d 是质数\n", result.Number)
			primeCount++
		}
		totalCount++
	}

	fmt.Printf("  --- 流水线完成: %d个数据, %d个质数, 耗时: %v ---\n",
		totalCount, primeCount, time.Since(start))
}

// PipelineResult 流水线阶段间传递的数据
type PipelineResult struct {
	Number  int
	IsPrime bool
}

// generate 是流水线的第一阶段: 生成数据
// 关键点: 返回 <-chan (只读channel)，限制下游只能读取
func generate(from, to int) <-chan int {
	out := make(chan int)
	go func() {
		defer close(out) // 关键点: 阶段完成后关闭channel，通知下游
		for i := from; i <= to; i++ {
			out <- i
		}
	}()
	return out
}

// process 是流水线的第二阶段: 数据处理
// 关键点: 接收上游的只读channel，返回新的只读channel
func process(in <-chan int) <-chan PipelineResult {
	out := make(chan PipelineResult)
	go func() {
		defer close(out)
		for n := range in {
			// 模拟耗时的计算
			time.Sleep(10 * time.Millisecond)
			out <- PipelineResult{
				Number:  n,
				IsPrime: isPrime(n),
			}
		}
	}()
	return out
}

func isPrime(n int) bool {
	if n < 2 {
		return false
	}
	for i := 2; i <= int(math.Sqrt(float64(n))); i++ {
		if n%i == 0 {
			return false
		}
	}
	return true
}
