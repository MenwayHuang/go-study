package main

import (
	"flag"
	"fmt"
)

func main() {
	demo := flag.String("demo", "all", "选择: pool, memory, lock, pprof, all")
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
	case "all":
		fmt.Println("========== 8.1 sync.Pool 对象复用 ==========")
		RunPoolDemo()
		fmt.Println("\n========== 8.2 内存优化 ==========")
		RunMemoryDemo()
		fmt.Println("\n========== 8.3 并发锁优化 ==========")
		RunLockDemo()
	default:
		fmt.Println("未知示例")
	}
}
