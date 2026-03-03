package main

import (
	"flag"
	"fmt"
)

func main() {
	demo := flag.String("demo", "all", "选择: retry, degrade, idempotent, health, all")
	flag.Parse()

	switch *demo {
	case "retry":
		RunRetryDemo()
	case "degrade":
		RunDegradeDemo()
	case "idempotent":
		RunIdempotentDemo()
	case "health":
		RunHealthCheckDemo()
	case "all":
		fmt.Println("========== 7.1 重试策略 ==========")
		RunRetryDemo()
		fmt.Println("\n========== 7.2 优雅降级 ==========")
		RunDegradeDemo()
		fmt.Println("\n========== 7.3 幂等设计 ==========")
		RunIdempotentDemo()
		fmt.Println("\n========== 7.4 健康检查 ==========")
		RunHealthCheckDemo()
	default:
		fmt.Println("未知示例")
	}
}
