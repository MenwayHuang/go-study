package main

import (
	"fmt"
	"runtime"
	"sync"
	"time"
)

func RunGoroutineDemo() {
	fmt.Println("--- 示例1: Goroutine 基本创建 ---")
	basicGoroutine()

	fmt.Println("\n--- 示例2: 大量 Goroutine 创建 ---")
	massiveGoroutines()

	fmt.Println("\n--- 示例3: Goroutine 与 WaitGroup 配合 ---")
	goroutineWithWaitGroup()
}

// basicGoroutine 展示最基本的 goroutine 创建
// 关键点: go 关键字启动协程，main 退出时所有协程被杀死
func basicGoroutine() {
	// 打印当前 GOMAXPROCS（逻辑处理器P的数量）
	fmt.Printf("GOMAXPROCS = %d (CPU核心数)\n", runtime.GOMAXPROCS(0))
	fmt.Printf("当前 goroutine 数量: %d\n", runtime.NumGoroutine())

	go func() {
		fmt.Println("  [goroutine] 我是一个匿名goroutine")
	}()

	go sayHello("goroutine-1")
	go sayHello("goroutine-2")

	// 关键点: 这里用 Sleep 等待是 **错误做法**，仅做演示
	// 正确做法应该用 WaitGroup 或 channel，见下方示例
	time.Sleep(100 * time.Millisecond)
	fmt.Printf("goroutine 执行后数量: %d\n", runtime.NumGoroutine())
}

func sayHello(name string) {
	fmt.Printf("  [goroutine] Hello from %s\n", name)
}

// massiveGoroutines 展示 goroutine 的轻量级特性
// 关键点: goroutine 初始栈仅 2KB，可以轻松创建 10万个
func massiveGoroutines() {
	numGoroutines := 100_000
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	start := time.Now()
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			// 模拟一点工作
			_ = 1 + 1
			time.Sleep(1 * time.Second)
		}()
	}
	wg.Wait()
	elapsed := time.Since(start)

	fmt.Printf("创建并运行 %d 个 goroutine 耗时: %v\n", numGoroutines, elapsed)
	fmt.Printf("当前 goroutine 数量: %d (已全部回收)\n", runtime.NumGoroutine())
}

// goroutineWithWaitGroup 展示正确的 goroutine 等待方式
// 关键点: 永远不要用 time.Sleep 等待 goroutine，用 WaitGroup
func goroutineWithWaitGroup() {
	var wg sync.WaitGroup

	tasks := []string{"下载文件A", "下载文件B", "下载文件C"}

	for _, task := range tasks {
		wg.Add(1)
		go func(t string) {
			defer wg.Done()
			// 模拟耗时操作
			time.Sleep(time.Duration(50+len(t)) * time.Millisecond)
			fmt.Printf("  ✓ 任务完成: %s\n", t)
		}(task) // 关键点: 必须传参，否则闭包捕获的是循环变量的引用（Go 1.22前）
	}

	wg.Wait()
	fmt.Println("所有任务完成!")
}
