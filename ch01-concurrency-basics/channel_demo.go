package main

import (
	"fmt"
	"math/rand"
	"time"
)

func RunChannelDemo() {
	fmt.Println("--- 示例1: 无缓冲 channel (同步通信) ---")
	unbufferedChannel()

	fmt.Println("\n--- 示例2: 有缓冲 channel (异步通信) ---")
	bufferedChannel()

	fmt.Println("\n--- 示例3: channel 方向限制 ---")
	directionalChannel()

	fmt.Println("\n--- 示例4: select 多路复用 ---")
	selectDemo()

	fmt.Println("\n--- 示例5: 用 channel 实现信号量 ---")
	semaphoreDemo()
}

// unbufferedChannel 无缓冲channel: 发送和接收必须同时就绪
// 关键点: 无缓冲channel是同步的，常用于goroutine之间的同步信号
func unbufferedChannel() {
	ch := make(chan string) // 无缓冲

	go func() {
		time.Sleep(50 * time.Millisecond) // 模拟工作
		ch <- "任务结果"                      // 发送会阻塞，直到有人接收
	}()

	result := <-ch // 接收会阻塞，直到有人发送
	fmt.Printf("  收到: %s\n", result)
}

// bufferedChannel 有缓冲channel: 缓冲区满了才阻塞发送，空了才阻塞接收
// 关键点: 缓冲大小的选择很重要
//   - 太小: 频繁阻塞，失去异步的意义
//   - 太大: 占用内存，可能掩盖消费速度慢的问题
func bufferedChannel() {
	ch := make(chan int, 3) // 缓冲大小为3

	// 生产者: 快速发送，不阻塞（直到缓冲满）
	go func() {
		for i := 1; i <= 5; i++ {
			fmt.Printf("  发送: %d\n", i)
			ch <- i
		}
		close(ch) // 关键点: 发送完毕后关闭channel，通知接收方
	}()

	// 消费者: 用 range 遍历channel，channel关闭后自动退出循环
	for val := range ch {
		fmt.Printf("  接收: %d\n", val)
		time.Sleep(30 * time.Millisecond) // 模拟处理
	}
}

// directionalChannel 展示单向channel的使用
// 关键点: 用类型系统限制channel的使用方向，避免误用
//   - chan<- T  只能发送
//   - <-chan T  只能接收
func directionalChannel() {
	ch := make(chan int, 1)

	// producer 只能往 channel 发送
	producer := func(out chan<- int) {
		out <- 42
		close(out)
	}

	// consumer 只能从 channel 接收
	consumer := func(in <-chan int) {
		val := <-in
		fmt.Printf("  单向channel接收: %d\n", val)
	}

	producer(ch)
	consumer(ch)
}

// selectDemo 展示 select 多路复用
// 关键点: select 是并发编程的核心控制结构
//   - 同时监听多个channel
//   - 哪个先就绪就执行哪个
//   - default 分支实现非阻塞操作
//   - 常与 time.After/time.Tick/context.Done 配合
func selectDemo() {
	ch1 := make(chan string)
	ch2 := make(chan string)

	go func() {
		time.Sleep(time.Duration(rand.Intn(100)) * time.Millisecond)
		ch1 <- "来自服务A的响应"
	}()
	go func() {
		time.Sleep(time.Duration(rand.Intn(100)) * time.Millisecond)
		ch2 <- "来自服务B的响应"
	}()

	// 关键点: 同时等待多个channel，先到先处理
	for i := 0; i < 2; i++ {
		select {
		case msg := <-ch1:
			fmt.Printf("  %s\n", msg)
		case msg := <-ch2:
			fmt.Printf("  %s\n", msg)
		case <-time.After(200 * time.Millisecond):
			// 关键点: 超时保护，防止永久阻塞
			fmt.Println("  超时!")
		}
	}

	// 非阻塞读取示例
	ch3 := make(chan int, 1)
	select {
	case val := <-ch3:
		fmt.Printf("  读到: %d\n", val)
	default:
		// 关键点: default 让 select 变成非阻塞的
		fmt.Println("  channel 为空，非阻塞跳过")
	}
}

// semaphoreDemo 用有缓冲channel实现信号量，控制并发数
// 关键点: 这是限制并发度的经典做法，比如限制同时只能有N个goroutine执行
// 在实际项目中非常实用：限制数据库连接数、API调用并发数等
func semaphoreDemo() {
	maxConcurrency := 3
	sem := make(chan struct{}, maxConcurrency) // 信号量

	tasks := 10
	done := make(chan struct{})

	go func() {
		for i := 0; i < tasks; i++ {
			sem <- struct{}{} // 获取信号量（满了就阻塞）
			go func(id int) {
				defer func() { <-sem }() // 释放信号量
				fmt.Printf("  任务 %d 执行中 (并发数限制为 %d)\n", id, maxConcurrency)
				time.Sleep(50 * time.Millisecond)
			}(i)
		}
		// 等待所有信号量归还
		//  聪明， 没有这一步的话，会提前退出
		for i := 0; i < maxConcurrency; i++ {
			sem <- struct{}{}
		}
		close(done)
	}()

	<-done
	fmt.Println("  所有任务完成（并发受控）")

	//workers := make(chan struct{}, 10)
	//done := make(chan struct{}, 10)
	//var count int32
	//go func() {
	//	for i := 0; i < 100; i++ {
	//		workers <- struct{}{}
	//		go func(id int) {
	//			fmt.Printf("当前执行任务: %d \n", id)
	//			atomic.AddInt32(&count, 1)
	//			<-workers
	//		}(i)
	//	}
	//
	//	for j := 0; j < 10; j++ {
	//		workers <- struct{}{}
	//	}
	//	close(done)
	//}()
	//<-done
	//fmt.Println(count)
}
