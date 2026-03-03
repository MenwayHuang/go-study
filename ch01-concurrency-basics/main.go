package main

import (
	"flag"
	"fmt"
)

func main() {
	demo := flag.String("demo", "all", "选择运行的示例: goroutine, channel, sync, context, leak, all")
	flag.Parse()
	switch *demo {
	case "goroutine":
		RunGoroutineDemo()
	case "channel":
		RunChannelDemo()
	case "sync":
		RunSyncDemo()
	case "context":
		RunContextDemo()
	case "leak":
		RunLeakDemo()
	case "all":
		fmt.Println("========== 1.1 Goroutine 基础 ==========")
		RunGoroutineDemo()
		fmt.Println("\n========== 1.2 Channel 核心用法 ==========")
		RunChannelDemo()
		fmt.Println("\n========== 1.3 Sync 同步原语 ==========")
		RunSyncDemo()
		fmt.Println("\n========== 1.4 Context 上下文控制 ==========")
		RunContextDemo()
		fmt.Println("\n========== 1.5 Goroutine 泄漏演示 ==========")
		RunLeakDemo()
	default:
		fmt.Println("未知示例，可选: goroutine, channel, sync, context, leak, all")
	}
}
