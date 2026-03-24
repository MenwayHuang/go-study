package main

import (
	"context"
	"flag"
	"log"
	"time"

	"go-study/projects/mq-task-system/internal/producer"
)

func main() {
	var (
		n           = flag.Int("n", 100, "number of tasks")
		concurrency = flag.Int("concurrency", 5, "concurrency")
	)
	flag.Parse()

	p, err := producer.New(producer.ConfigFromEnv())
	if err != nil {
		log.Fatal(err)
	}
	defer p.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	log.Printf("publishing tasks: n=%d concurrency=%d", *n, *concurrency)
	if err := p.PublishMany(ctx, *n, *concurrency); err != nil {
		log.Fatal(err)
	}
	log.Println("done")
}
