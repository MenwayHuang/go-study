package main

import (
	"fmt"
	"math/rand"
	"sync"
	"time"
)

func RunFanOutFanInDemo() {
	fmt.Println("--- 示例: 并行调用多个API并汇总结果 ---")
	fanOutFanInDemo()
	//fanOutFanInDemo2()
}

// fanOutFanInDemo 模拟: 同时调用多个下游服务，汇总所有结果
// 关键点:
//   - Fan-out: 一个数据源分发到多个goroutine并行处理
//   - Fan-in:  多个goroutine的结果通过一个channel汇聚
//   - 实际场景: 商品详情页需要同时查 商品信息+价格+库存+评论
func fanOutFanInDemo() {
	// 模拟数据源: 一批用户ID需要查询详细信息
	userIDs := []int{101, 102, 103, 104, 105, 106, 107, 108}

	start := time.Now()

	// Fan-out: 启动多个worker并行处理
	numWorkers := 3
	idChan := make(chan int, len(userIDs))
	for _, id := range userIDs {
		idChan <- id
	}
	close(idChan)

	// 每个 worker 都产出结果到自己的 channel（而不是所有 worker 共享一个 results channel）。
	// 这不是因为“channel 太小会阻塞”，而是为了让关闭职责清晰、组件更可组合:
	//   - fetchUserInfo 创建 out，因此它自己负责 close(out)，不会影响其他 worker。
	//   - merge 负责在所有 out 都结束后 close(merged)，消费者 range merged 能自然退出。
	// 如果用共享 results channel 也可以，但要额外保证:
	//   - 任何一个 worker 都不能 close(results)，否则其他 worker 发送会 panic。
	//   - 通常由协调者在 wg.Wait() 后统一 close(results)。
	workerChans := make([]<-chan string, numWorkers)
	for i := 0; i < numWorkers; i++ {
		workerChans[i] = fetchUserInfo(i, idChan)
	}

	// Fan-in: 把所有worker的结果合并到一个channel
	merged := merge(workerChans...)

	// 消费合并后的结果
	count := 0
	for result := range merged {
		fmt.Printf("  %s\n", result)
		count++
	}

	elapsed := time.Since(start)
	fmt.Printf("  --- %d个任务, %d个Worker并行, 总耗时: %v ---\n",
		count, numWorkers, elapsed)
	fmt.Println("  (如果串行处理每个50ms, 需要400ms; 并行处理约133ms)")
}

// fetchUserInfo 模拟查询用户信息的worker
func fetchUserInfo(workerID int, ids <-chan int) <-chan string {
	out := make(chan string)
	go func() {
		defer close(out)
		for id := range ids {
			// 模拟API调用
			time.Sleep(time.Duration(30+rand.Intn(40)) * time.Millisecond)
			out <- fmt.Sprintf("Worker%d: 用户%d的信息={name:user%d, age:%d}",
				workerID, id, id, 20+rand.Intn(30))
		}
	}()
	return out
}

// merge 将多个channel合并为一个 (Fan-in核心函数)
// 关键点: 这是一个通用的Fan-in实现，可以在项目中复用
func merge(channels ...<-chan string) <-chan string {
	merged := make(chan string)
	var wg sync.WaitGroup

	// 为每个输入channel启动一个goroutine，转发到merged
	for _, ch := range channels {
		wg.Add(1)
		go func(c <-chan string) {
			defer wg.Done()
			for val := range c {
				merged <- val
			}
		}(ch)
	}

	// 所有输入channel都关闭后，关闭merged
	go func() {
		wg.Wait()
		close(merged)
	}()

	return merged
}

//func fanOutFanInDemo2() {
//	// 模拟数据源: 一批用户ID需要查询详细信息
//	userIDs := []int{101, 102, 103, 104, 105, 106, 107, 108}
//
//	// Fan-out: 启动多个worker并行处理
//	numWorkers := 3
//	idChan := make(chan int, len(userIDs))
//	for _, id := range userIDs {
//		idChan <- id
//	}
//	close(idChan)
//
//	workersChan := make([]<-chan string, 3)
//	for i := 0; i < numWorkers; i++ {
//		workersChan[i] = fetch2(i, idChan)
//	}
//	m := merge2(workersChan)
//
//	count := 0
//	for s := range m {
//		count++
//		fmt.Println(s)
//	}
//
//}
//
//func fetch2(i int, idChan <-chan int) <-chan string {
//	res := make(chan string)
//	go func() {
//		defer close(res)
//		for id := range idChan {
//			res <- fmt.Sprintf("%d 乃一组特 %d", i, id)
//		}
//	}()
//
//	return res
//}
//
//func merge2(wcs []<-chan string) <-chan string {
//	var wg sync.WaitGroup
//	mergeRes := make(chan string)
//	for i := 0; i < len(wcs); i++ {
//		wg.Add(1)
//		go func(j int) {
//			defer wg.Done()
//			for s := range wcs[j] {
//				mergeRes <- s
//			}
//		}(i)
//	}
//
//	go func() {
//		wg.Wait()
//		close(mergeRes)
//	}()
//
//	return mergeRes
//}
