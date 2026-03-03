package main

import (
	"fmt"
	"math/rand"
	"sync"
	"time"
)

// Task 代表一个工作任务
type Task struct {
	ID      int
	Payload string
}

// Result 代表任务执行结果
type Result struct {
	TaskID  int
	Output  string
	Elapsed time.Duration
}

func RunWorkerPoolDemo() {
	fmt.Println("--- 示例1: 基础 Worker Pool ---")
	basicWorkerPool()

	fmt.Println("\n--- 示例2: 带结果收集的 Worker Pool ---")
	workerPoolWithResults()
}

// basicWorkerPool 最基础的Worker Pool实现
// 关键点: 这是高并发系统中最核心的模式
//   - jobs channel: 任务队列，生产者放入任务
//   - workers: 固定数量的goroutine从jobs中取任务执行
//   - 控制并发度，避免goroutine爆炸
func basicWorkerPool() {
	const numWorkers = 3
	const numJobs = 10

	jobs := make(chan int, numJobs)       // 任务队列（有缓冲，生产者不阻塞）
	results := make(chan string, numJobs) // 结果队列
	// 注意: 这里的 numJobs 只是 demo 为了方便设置的缓冲大小，并不代表 Worker Pool 必须“提前知道任务数量”。
	// 实际项目里常见两类任务源:
	//   1) 批处理/有限任务: producer 发完一批任务后 close(jobs)，worker 用 range jobs 自然退出。
	//   2) 长期运行/流式任务: 往往用 context/done 信号让 producer 停止产出；当确认不会再发送任务时再 close(jobs)。
	// 关键规则:
	//   - 只有发送方/生产者才能 close(channel)，接收方不要 close。
	//   - results 通常由“协调者”在 wg.Wait() 之后统一 close，避免 worker 误关导致 send on closed channel。

	// 启动固定数量的 Worker
	var wg sync.WaitGroup
	for w := 1; w <= numWorkers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			// 关键点: range jobs 会在 jobs 关闭后自动退出
			for job := range jobs {
				time.Sleep(time.Duration(rand.Intn(50)) * time.Millisecond)
				results <- fmt.Sprintf("Worker%d 完成任务%d", workerID, job)
			}
		}(w)
	}

	// 生产者: 发送任务
	for j := 1; j <= numJobs; j++ {
		jobs <- j
	}
	close(jobs) // 关键点: 关闭jobs通知worker没有更多任务了

	// 等待所有worker完成后关闭results
	go func() {
		wg.Wait()
		close(results)
	}()

	// 收集结果
	for r := range results {
		fmt.Printf("  %s\n", r)
	}

	//workerNums := 3
	//jobQueue := make(chan Task, 10)
	//resultQueue := make(chan string, 10)
	//for i := 0; i < workerNums; i++ {
	//	wg.Add(1)
	//	go func(id int) {
	//		defer wg.Done()
	//		for t := range jobQueue {
	//			resultQueue <- fmt.Sprintf("worker %d  processing, task id is %d", id, t.ID)
	//		}
	//	}(i)
	//}
	//go func() {
	//	for i := 0; i < 100000; i++ {
	//		jobQueue <- Task{ID: i}
	//	}
	//	close(jobQueue)
	//}()
	//
	//go func() {
	//	wg.Wait()
	//	close(resultQueue)
	//}()
	//
	//for r := range resultQueue {
	//	fmt.Println(r)
	//}

}

// workerPoolWithResults 带结构化结果的Worker Pool
// 关键点: 实际项目中的Worker Pool通常需要:
//  1. 结构化的任务和结果
//  2. 错误处理
//  3. 超时控制
//  4. 可观测性（知道每个任务的耗时）
func workerPoolWithResults() {
	const numWorkers = 5
	const numTasks = 15

	tasks := make(chan Task, numTasks)
	results := make(chan Result, numTasks)
	// 注意: 这里用 numTasks 作为缓冲仅是为了 demo 输出更直观。
	// 如果任务数量未知/任务持续产生:
	//   - tasks 由 producer 在“不会再发送任务”时关闭（不要求预先知道总数）。
	//   - 多 producer 时，常见做法是先等所有 producer 退出，再由统一的协调者 close(tasks)。
	//   - results 的关闭不要放在 worker 里做，而是由协调者 wg.Wait() 后统一 close(results)。

	// 启动 Worker
	var wg sync.WaitGroup
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go worker(w, tasks, results, &wg)
	}

	// 发送任务
	for i := 0; i < numTasks; i++ {
		tasks <- Task{ID: i, Payload: fmt.Sprintf("data-%d", i)}
	}
	close(tasks)

	// 后台等待完成
	go func() {
		wg.Wait()
		close(results)
	}()

	// 收集并统计结果
	var totalTime time.Duration
	count := 0
	for r := range results {
		fmt.Printf("  任务%d: %s (耗时: %v)\n", r.TaskID, r.Output, r.Elapsed)
		totalTime += r.Elapsed
		count++
	}
	fmt.Printf("  --- 统计: %d个任务, 平均耗时: %v, %d个Worker并行 ---\n",
		count, totalTime/time.Duration(count), numWorkers)
}

func worker(id int, tasks <-chan Task, results chan<- Result, wg *sync.WaitGroup) {
	defer wg.Done()
	for task := range tasks {
		start := time.Now()
		// 模拟不同耗时的任务处理
		time.Sleep(time.Duration(20+rand.Intn(80)) * time.Millisecond)
		results <- Result{
			TaskID:  task.ID,
			Output:  fmt.Sprintf("Worker%d处理了[%s]", id, task.Payload),
			Elapsed: time.Since(start),
		}
	}
}
