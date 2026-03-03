package main

import (
	"flag"
	"fmt"
)

func main() {
	demo := flag.String("demo", "all", "选择: workerpool, fanout, pipeline, ratelimit, errgroup, all")
	flag.Parse()
	RunFanOutFanInDemo()
	*demo = ""
	switch *demo {
	case "workerpool":
		RunWorkerPoolDemo()
	case "fanout":
		RunFanOutFanInDemo()
	case "pipeline":
		RunPipelineDemo()
	case "ratelimit":
		RunRateLimitDemo()
	case "errgroup":
		RunErrGroupDemo()
	case "all":
		fmt.Println("========== 2.1 Worker Pool ==========")
		RunWorkerPoolDemo()
		fmt.Println("\n========== 2.2 Fan-out / Fan-in ==========")
		RunFanOutFanInDemo()
		fmt.Println("\n========== 2.3 Pipeline ==========")
		RunPipelineDemo()
		fmt.Println("\n========== 2.4 Rate Limiter ==========")
		RunRateLimitDemo()
		fmt.Println("\n========== 2.5 ErrGroup ==========")
		RunErrGroupDemo()
	default:
		fmt.Println("未知示例")
	}
}
