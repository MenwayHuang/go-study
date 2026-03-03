package main

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"
)

func RunErrGroupDemo() {
	fmt.Println("--- 示例1: 简易 ErrGroup 实现 ---")
	simpleErrGroupDemo()

	fmt.Println("\n--- 示例2: 带取消的 ErrGroup ---")
	errGroupWithCancelDemo()
}

// SimpleErrGroup 不依赖外部包的 errgroup 实现
// 关键点: 标准库的 errgroup (golang.org/x/sync/errgroup) 原理一样
//   - 并发执行多个任务
//   - 收集第一个错误
//   - 等待所有任务完成
//
// 
// 注意:
//   - “收集第一个错误”并不等于“快速失败”。这里的实现不会主动取消其他任务。
//   - 如果你希望任一失败立刻停止其他任务，需要配合 context 取消（见 CancelErrGroup）。
type SimpleErrGroup struct {
	wg      sync.WaitGroup
	errOnce sync.Once
	err     error
}

func (g *SimpleErrGroup) Go(f func() error) {
	g.wg.Add(1)
	go func() {
		defer g.wg.Done()
		if err := f(); err != nil {
			g.errOnce.Do(func() {
				g.err = err // 只记录第一个错误
			})
		}
	}()
}

func (g *SimpleErrGroup) Wait() error {
	g.wg.Wait()
	return g.err
}

// simpleErrGroupDemo 并发执行多个任务并收集错误
// 关键点: 实际项目中经常遇到 "并发做N件事，任一失败则整体失败"
func simpleErrGroupDemo() {
	g := &SimpleErrGroup{}

	// 模拟3个并发任务
	services := []struct {
		name    string
		latency time.Duration
		fail    bool
	}{
		{"用户服务", 50 * time.Millisecond, false},
		{"订单服务", 80 * time.Millisecond, true}, // 这个会失败
		{"库存服务", 60 * time.Millisecond, false},
	}

	for _, svc := range services {
		svc := svc // 捕获循环变量
		g.Go(func() error {
			time.Sleep(svc.latency)
			if svc.fail {
				return fmt.Errorf("%s 调用失败: connection refused", svc.name)
			}
			fmt.Printf("  ✓ %s 调用成功\n", svc.name)
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		fmt.Printf("  ✗ 聚合调用失败: %v\n", err)
	} else {
		fmt.Println("  所有服务调用成功")
	}
}

// CancelErrGroup 带取消功能: 一个任务失败，取消其他所有任务
// 关键点: 这是 golang.org/x/sync/errgroup.WithContext 的核心行为
// 注意:
//   - 取消只是“发出信号”，其他任务必须在逻辑里监听 ctx.Done() 才能尽快退出。
//   - 被取消的任务通常会返回 ctx.Err()；但我们只记录第一个触发 cancel 的错误，避免 ctx.Err 覆盖真实根因。
type CancelErrGroup struct {
	wg      sync.WaitGroup
	errOnce sync.Once
	err     error
	ctx     context.Context
	cancel  context.CancelFunc
}

func NewCancelErrGroup(ctx context.Context) *CancelErrGroup {
	ctx, cancel := context.WithCancel(ctx)
	return &CancelErrGroup{ctx: ctx, cancel: cancel}
}

func (g *CancelErrGroup) Go(f func(ctx context.Context) error) {
	g.wg.Add(1)
	go func() {
		defer g.wg.Done()
		if err := f(g.ctx); err != nil {
			g.errOnce.Do(func() {
				g.err = err
				g.cancel() // 关键点: 第一个错误出现时，取消所有其他任务
			})
		}
	}()
}

// 扩展思路: 并发上限（常见需求）
// 真实项目经常需要“并发做 N 件事，但最多同时跑 K 个”。
// 常见做法是在 Go(f) 前后加一个 semaphore：
//   sem := make(chan struct{}, K)
//   sem <- struct{}{}  // acquire
//   g.Go(func(...) error { defer func(){<-sem}(); ... })
// 这样可以避免同时起太多 goroutine 或下游请求。

func (g *CancelErrGroup) Wait() error {
	g.wg.Wait()
	g.cancel() // 确保资源释放
	return g.err
}

// errGroupWithCancelDemo 展示"快速失败"模式
// 关键点: 微服务调用中，一个下游失败了，就没必要等其他的了
func errGroupWithCancelDemo() {
	g := NewCancelErrGroup(context.Background())

	// 任务1: 正常但耗时
	g.Go(func(ctx context.Context) error {
		select {
		case <-time.After(200 * time.Millisecond):
			fmt.Println("  任务1: 完成")
			return nil
		case <-ctx.Done():
			fmt.Println("  任务1: 被取消 (快速失败)")
			return ctx.Err()
		}
	})

	// 任务2: 很快就失败
	g.Go(func(ctx context.Context) error {
		time.Sleep(time.Duration(30+rand.Intn(20)) * time.Millisecond)
		return fmt.Errorf("任务2失败: 数据库连接超时")
	})

	// 任务3: 正常
	g.Go(func(ctx context.Context) error {
		select {
		case <-time.After(100 * time.Millisecond):
			fmt.Println("  任务3: 完成")
			return nil
		case <-ctx.Done():
			fmt.Println("  任务3: 被取消 (快速失败)")
			return ctx.Err()
		}
	})

	if err := g.Wait(); err != nil {
		fmt.Printf("  ✗ 结果: %v\n", err)
		fmt.Println("  (其他运行中的任务被立即取消，避免浪费资源)")
	}
}
