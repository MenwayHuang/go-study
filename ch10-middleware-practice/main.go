package main

import (
	"flag"
	"fmt"
)

func main() {
	demo := flag.String("demo", "all", "选择: redis, nats, seckill, all")
	flag.Parse()

	switch *demo {
	case "redis":
		RunRedisDemo()
	case "nats":
		RunNatsDemo()
	case "seckill":
		RunSeckillRedisDemo()
	case "all":
		fmt.Println("========== 10.1 Redis 实战 ==========")
		RunRedisDemo()
		fmt.Println("\n========== 10.2 NATS 实战 ==========")
		RunNatsDemo()
		fmt.Println("\n========== 10.3 Redis版秒杀库存 ==========")
		RunSeckillRedisDemo()
	default:
		fmt.Println("未知示例，可选: redis, nats, seckill, all")
	}
}
