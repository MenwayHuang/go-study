package main

import (
	"flag"
	"fmt"
)

func main() {
	demo := flag.String("demo", "all", "选择: mq, pubsub, order, all")
	flag.Parse()

	switch *demo {
	case "mq":
		RunMQDemo()
	case "pubsub":
		RunPubSubDemo()
	case "order":
		RunOrderDemo()
	case "all":
		fmt.Println("========== 4.1 内存消息队列 ==========")
		RunMQDemo()
		fmt.Println("\n========== 4.2 发布/订阅 ==========")
		RunPubSubDemo()
		fmt.Println("\n========== 4.3 订单异步处理实战 ==========")
		RunOrderDemo()
	default:
		fmt.Println("未知示例")
	}
}
