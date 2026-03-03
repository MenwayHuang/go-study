package main

import (
	"fmt"
	"strings"
	"time"
	"unsafe"
)

// ============================================================
// 内存优化技巧
// ============================================================

func RunMemoryDemo() {
	fmt.Println("--- 示例1: slice 预分配 ---")
	slicePreallocDemo()

	fmt.Println("\n--- 示例2: 字符串拼接优化 ---")
	stringConcatDemo()

	fmt.Println("\n--- 示例3: 结构体内存对齐 ---")
	structAlignDemo()

	fmt.Println("\n--- 示例4: map 预分配 ---")
	mapPreallocDemo()
}

// slicePreallocDemo 预分配slice容量，避免多次扩容
// 关键点:
//   append 扩容机制: 容量不足时，分配新数组(通常2倍)，拷贝旧数据
//   每次扩容都是一次内存分配 + 数据拷贝 → 频繁扩容非常昂贵
func slicePreallocDemo() {
	n := 1_000_000

	// ❌ 不预分配
	start := time.Now()
	s1 := make([]int, 0)
	for i := 0; i < n; i++ {
		s1 = append(s1, i)
	}
	noPrealloc := time.Since(start)

	// ✅ 预分配
	start = time.Now()
	s2 := make([]int, 0, n) // 预分配容量
	for i := 0; i < n; i++ {
		s2 = append(s2, i)
	}
	prealloc := time.Since(start)

	fmt.Printf("  不预分配: %v\n", noPrealloc)
	fmt.Printf("  预分配:   %v\n", prealloc)
	fmt.Printf("  提升: %.1fx\n", float64(noPrealloc)/float64(prealloc))
}

// stringConcatDemo 字符串拼接优化
// 关键点: Go中string是不可变的，每次 + 拼接都会创建新的string对象
func stringConcatDemo() {
	n := 100_000

	// ❌ 用 + 拼接 (每次创建新字符串)
	start := time.Now()
	s1 := ""
	for i := 0; i < n; i++ {
		s1 += "a"
	}
	plusConcat := time.Since(start)

	// ✅ 用 strings.Builder (内部是 []byte, 减少分配)
	start = time.Now()
	var builder strings.Builder
	builder.Grow(n) // 预分配
	for i := 0; i < n; i++ {
		builder.WriteString("a")
	}
	_ = builder.String()
	builderConcat := time.Since(start)

	fmt.Printf("  + 拼接:          %v\n", plusConcat)
	fmt.Printf("  strings.Builder: %v\n", builderConcat)
	fmt.Printf("  提升: %.0fx\n", float64(plusConcat)/float64(builderConcat))
	fmt.Println("  (strings.Builder 是拼接字符串的标准做法)")
}

// structAlignDemo 结构体内存对齐
// 关键点:
//   CPU 按字长(8 bytes)读取内存，字段没对齐就需要填充(padding)
//   合理排列字段顺序可以减少padding，节省内存
//   高并发下，百万个结构体 × 每个省16字节 = 省几十MB
func structAlignDemo() {
	// ❌ 差的排列: 字段大小交错，产生大量padding
	type BadLayout struct {
		a bool   // 1 byte + 7 padding
		b int64  // 8 bytes
		c bool   // 1 byte + 7 padding
		d int64  // 8 bytes
		e bool   // 1 byte + 7 padding
	}

	// ✅ 好的排列: 大字段放前面，小字段放后面
	type GoodLayout struct {
		b int64 // 8 bytes
		d int64 // 8 bytes
		a bool  // 1 byte
		c bool  // 1 byte
		e bool  // 1 byte + 5 padding
	}

	fmt.Printf("  BadLayout  大小: %d bytes\n", unsafe.Sizeof(BadLayout{}))
	fmt.Printf("  GoodLayout 大小: %d bytes\n", unsafe.Sizeof(GoodLayout{}))
	fmt.Println("  原则: 按字段大小从大到小排列")
	fmt.Println("  工具: go vet -fieldalignment 可以自动检测")
}

// mapPreallocDemo map 预分配
// 关键点: map 扩容比 slice 更昂贵 (需要重新哈希所有key)
func mapPreallocDemo() {
	n := 1_000_000

	// ❌ 不预分配
	start := time.Now()
	m1 := make(map[int]int)
	for i := 0; i < n; i++ {
		m1[i] = i
	}
	noPrealloc := time.Since(start)

	// ✅ 预分配
	start = time.Now()
	m2 := make(map[int]int, n)
	for i := 0; i < n; i++ {
		m2[i] = i
	}
	prealloc := time.Since(start)

	fmt.Printf("  不预分配: %v\n", noPrealloc)
	fmt.Printf("  预分配:   %v\n", prealloc)
	fmt.Printf("  提升: %.1fx\n", float64(noPrealloc)/float64(prealloc))
}
