package main

import (
	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof" // 关键点: 空导入，自动注册 /debug/pprof 路由
	"runtime"
	"time"
)

// ============================================================
// pprof 性能分析
// ============================================================
// 关键点:
//   pprof 是 Go 内置的性能分析工具，生产环境必备
//   只需要 import _ "net/http/pprof" 就能使用
//
// 使用流程:
//   1. 代码中启动 pprof HTTP 服务 (通常用独立端口6060)
//   2. 触发业务负载 (压测/正常流量)
//   3. 用 go tool pprof 采集并分析
//
// 常用命令:
//   go tool pprof http://localhost:6060/debug/pprof/profile?seconds=10  # CPU
//   go tool pprof http://localhost:6060/debug/pprof/heap                # 内存
//   go tool pprof http://localhost:6060/debug/pprof/goroutine           # goroutine
//   go tool pprof -http=:8081 <profile文件>                              # 网页版火焰图

func RunPprofDemo() {
	fmt.Println("--- pprof 性能分析服务器 ---")
	fmt.Println("  启动 pprof 服务: http://localhost:6060/debug/pprof/")
	fmt.Println()
	fmt.Println("  浏览器访问:")
	fmt.Println("    http://localhost:6060/debug/pprof/          # 总览")
	fmt.Println("    http://localhost:6060/debug/pprof/heap      # 内存分析")
	fmt.Println("    http://localhost:6060/debug/pprof/goroutine # goroutine分析")
	fmt.Println()
	fmt.Println("  命令行分析:")
	fmt.Println("    go tool pprof http://localhost:6060/debug/pprof/profile?seconds=10")
	fmt.Println("    > top 10        # 查看CPU占用最高的10个函数")
	fmt.Println("    > list funcName # 查看某函数的逐行CPU开销")
	fmt.Println("    > web           # 生成调用图 (需要 graphviz)")
	fmt.Println()

	// 打印运行时信息
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	fmt.Printf("  当前内存: Alloc=%.2f MB, Sys=%.2f MB, NumGC=%d\n",
		float64(m.Alloc)/1024/1024,
		float64(m.Sys)/1024/1024,
		m.NumGC)
	fmt.Printf("  Goroutine数: %d\n", runtime.NumGoroutine())
	fmt.Printf("  GOMAXPROCS: %d\n", runtime.GOMAXPROCS(0))

	// 启动模拟负载 (方便采集到数据)
	go simulateLoad()

	// 启动 pprof HTTP 服务 (独立端口，不影响业务)
	fmt.Println("\n  pprof 服务运行中... (Ctrl+C 退出)")
	log.Fatal(http.ListenAndServe(":6060", nil))
}

// simulateLoad 模拟一些CPU和内存负载，方便pprof采集
func simulateLoad() {
	for {
		// CPU 负载
		sum := 0
		for i := 0; i < 1_000_000; i++ {
			sum += i
		}

		// 内存负载
		data := make([]byte, 1024*1024) // 1MB
		data[0] = byte(sum)
		_ = data

		time.Sleep(100 * time.Millisecond)
	}
}
