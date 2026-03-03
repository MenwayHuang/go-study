package main

import (
	"flag"
	"fmt"
)

func main() {
	demo := flag.String("demo", "all", "选择: registry, loadbalance, breaker, trace, all")
	flag.Parse()

	switch *demo {
	case "registry":
		RunRegistryDemo()
	case "loadbalance":
		RunLoadBalanceDemo()
	case "breaker":
		RunBreakerDemo()
	case "trace":
		RunTraceDemo()
	case "all":
		fmt.Println("========== 6.1 服务注册与发现 ==========")
		RunRegistryDemo()
		fmt.Println("\n========== 6.2 负载均衡 ==========")
		RunLoadBalanceDemo()
		fmt.Println("\n========== 6.3 熔断器 ==========")
		RunBreakerDemo()
		fmt.Println("\n========== 6.4 链路追踪 ==========")
		RunTraceDemo()
	default:
		fmt.Println("未知示例")
	}
}
