package main

import (
	"context"
	"fmt"
	"runtime"
	"time"
)

func RunLeakDemo() {
	fmt.Println("--- 示例1: Goroutine 泄漏 (错误示范) ---")
	leakExample()

	fmt.Println("\n--- 示例2: 修复泄漏 (正确做法) ---")
	fixedExample()
}

// leakExample 展示常见的 goroutine 泄漏场景
// 关键点: 这是生产环境中最常见的内存泄漏原因之一
func leakExample() {
	before := runtime.NumGoroutine()

	// 错误示范1: channel 没有接收方，发送方永远阻塞
	leak1 := func() {
		ch := make(chan int)
		go func() {
			ch <- 42 // 没人读这个channel，goroutine永远阻塞在这里
		}()
		// 函数返回了，但goroutine还活着，永远无法退出 = 泄漏!
	}

	// 错误示范2: 函数提前返回，忘记关闭channel
	leak2 := func() {
		ch := make(chan int)
		go func() {
			for val := range ch { // 等待channel关闭
				_ = val
			}
		}()
		// ch 永远不会被关闭，goroutine永远阻塞在 range = 泄漏!
	}

	// 制造几个泄漏
	for i := 0; i < 5; i++ {
		leak1()
		leak2()
	}

	time.Sleep(50 * time.Millisecond)
	after := runtime.NumGoroutine()
	fmt.Printf("  泄漏前 goroutine: %d, 泄漏后: %d, 泄漏了: %d 个!\n",
		before, after, after-before)
	fmt.Println("  ⚠️  这些 goroutine 永远不会退出，内存永远不会释放")
}

// fixedExample 展示如何正确避免泄漏
// 关键点: 每个goroutine都必须有明确的退出路径
func fixedExample() {
	before := runtime.NumGoroutine()

	// 修复方案1: 用 context 控制退出
	fix1 := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		ch := make(chan int, 1) // 用有缓冲channel，即使没人读也不会阻塞发送方
		go func() {
			select {
			case ch <- 42:
			case <-ctx.Done(): // 超时自动退出
				return
			}
		}()

		select {
		case val := <-ch:
			_ = val
		case <-ctx.Done():
		}
	}

	// 修复方案2: 确保channel被正确关闭
	fix2 := func() {
		ch := make(chan int)
		go func() {
			defer close(ch) // 关键点: 确保关闭channel
			for i := 0; i < 3; i++ {
				ch <- i
			}
		}()

		for val := range ch { // channel关闭后，range自动退出
			_ = val
		}
	}

	for i := 0; i < 5; i++ {
		fix1()
		fix2()
	}

	time.Sleep(100 * time.Millisecond)
	after := runtime.NumGoroutine()
	fmt.Printf("  修复前 goroutine: %d, 修复后: %d, 泄漏: %d 个 ✓\n",
		before, after, after-before)
	fmt.Println("  ✅ 所有 goroutine 正确退出，无泄漏")

	fmt.Println("\n  === Goroutine 泄漏防治清单 ===")
	fmt.Println("  1. 每个 goroutine 必须有退出路径 (context/channel close/done signal)")
	fmt.Println("  2. 启动 goroutine 前想好: 谁来关闭它?")
	fmt.Println("  3. 用 go vet / goleak 检测泄漏")
	fmt.Println("  4. 监控 runtime.NumGoroutine() 指标")
	fmt.Println("  5. 有缓冲 channel 可以避免发送方阻塞")
}
