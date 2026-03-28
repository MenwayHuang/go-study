package main

import (
	"flag"
	"fmt"
)

func main() {
	demo := flag.String("demo", "all", "选择: pool, memory, lock, pprof, optimization, workflow, all")
	flag.Parse()

	switch *demo {
	case "pool":
		RunPoolDemo()
	case "memory":
		RunMemoryDemo()
	case "lock":
		RunLockDemo()
	case "pprof":
		RunPprofDemo()
	case "optimization":
		RunPerformanceOptimizationDemo()
	case "optimized":
		RunOptimizedDemo()
	case "workflow":
		RunCompletePprofWorkflow()
	case "all":
		fmt.Println("========== 8.1 sync.Pool 对象复用 ==========")
		RunPoolDemo()
		fmt.Println("\n========== 8.2 内存优化 ==========")
		RunMemoryDemo()
		fmt.Println("\n========== 8.3 并发锁优化 ==========")
		RunLockDemo()
		fmt.Println("\n========== 8.4 pprof 性能分析 ==========")
		fmt.Println("注意: pprof 演示会启动服务器，需要手动停止")
		fmt.Println("建议单独运行: go run main.go -demo=pprof")
	default:
		fmt.Println("未知示例")
	}
}
