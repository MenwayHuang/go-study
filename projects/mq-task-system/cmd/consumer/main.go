package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go-study/projects/mq-task-system/internal/consumer"
	"go-study/projects/mq-task-system/internal/metrics"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := consumer.ConfigFromEnv()

	// metrics http server
	mux := http.NewServeMux()
	mux.Handle("/metrics", metrics.Handler())
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	httpSrv := &http.Server{
		Addr:              ":8081",
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Println("consumer metrics listening on :8081")
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	c, err := consumer.New(cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer c.Close()

	// graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
		ctxShutdown, cancelShutdown := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancelShutdown()
		_ = httpSrv.Shutdown(ctxShutdown)
	}()

	log.Printf("consumer started: workers=%d prefetch=%d", cfg.Workers, cfg.Prefetch)
	if err := c.Run(ctx); err != nil {
		log.Fatal(err)
	}
}
