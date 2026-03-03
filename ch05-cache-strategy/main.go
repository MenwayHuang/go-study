package main

import (
	"flag"
	"fmt"
)

func main() {
	demo := flag.String("demo", "all", "选择: local, problems, singleflight, all")
	flag.Parse()

	switch *demo {
	case "local":
		RunLocalCacheDemo()
	case "problems":
		RunCacheProblemsDemo()
	case "singleflight":
		RunSingleFlightDemo()
	case "all":
		fmt.Println("========== 5.1 本地缓存 ==========")
		RunLocalCacheDemo()
		fmt.Println("\n========== 5.2 缓存穿透/雪崩/击穿防护 ==========")
		RunCacheProblemsDemo()
		fmt.Println("\n========== 5.3 SingleFlight 防击穿 ==========")
		RunSingleFlightDemo()
	default:
		fmt.Println("未知示例")
	}
}
