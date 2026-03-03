package main

import (
	"fmt"
	"hash/crc32"
	"math/rand"
	"sort"
	"sync"
	"sync/atomic"
)

// ============================================================
// 负载均衡算法合集
// ============================================================

// Balancer 负载均衡器接口
type Balancer interface {
	Pick(nodes []string) string
	Name() string
}

// ============================================================
// 1. 轮询 (Round Robin) — 最简单，均匀分配
// ============================================================

type RoundRobin struct {
	counter uint64
}

func (rr *RoundRobin) Pick(nodes []string) string {
	n := atomic.AddUint64(&rr.counter, 1)
	return nodes[n%uint64(len(nodes))]
}

func (rr *RoundRobin) Name() string { return "RoundRobin(轮询)" }

// ============================================================
// 2. 加权轮询 (Weighted Round Robin) — 性能强的机器分配更多请求
// ============================================================

type WeightedNode struct {
	Addr          string
	Weight        int // 配置权重
	CurrentWeight int // 当前权重（动态变化）
}

type WeightedRoundRobin struct {
	nodes []*WeightedNode
	mu    sync.Mutex
}

func NewWeightedRoundRobin(nodes []*WeightedNode) *WeightedRoundRobin {
	return &WeightedRoundRobin{nodes: nodes}
}

// Pick 使用 Nginx 的平滑加权轮询算法
// 关键点: 不会出现连续选择同一个高权重节点的情况
func (w *WeightedRoundRobin) Pick(_ []string) string {
	w.mu.Lock()
	defer w.mu.Unlock()

	totalWeight := 0
	var best *WeightedNode

	for _, node := range w.nodes {
		totalWeight += node.Weight
		node.CurrentWeight += node.Weight

		if best == nil || node.CurrentWeight > best.CurrentWeight {
			best = node
		}
	}

	best.CurrentWeight -= totalWeight
	return best.Addr
}

func (w *WeightedRoundRobin) Name() string { return "WeightedRoundRobin(加权轮询)" }

// ============================================================
// 3. 随机 (Random) — 简单高效，大量请求下趋于均匀
// ============================================================

type RandomBalancer struct{}

func (r *RandomBalancer) Pick(nodes []string) string {
	return nodes[rand.Intn(len(nodes))]
}

func (r *RandomBalancer) Name() string { return "Random(随机)" }

// ============================================================
// 4. 一致性哈希 (Consistent Hash) — 缓存场景首选
// ============================================================
// 关键点:
//   - 将节点映射到哈希环上
//   - 请求的key顺时针找到最近的节点
//   - 增删节点只影响相邻节点，大部分请求不受影响
//   - 虚拟节点解决数据倾斜问题

type ConsistentHash struct {
	ring     map[uint32]string // hash值 → 节点
	sorted   []uint32          // 排序后的hash值
	replicas int               // 每个节点的虚拟节点数
	mu       sync.RWMutex
}

func NewConsistentHash(replicas int) *ConsistentHash {
	return &ConsistentHash{
		ring:     make(map[uint32]string),
		replicas: replicas,
	}
}

func (ch *ConsistentHash) Add(node string) {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	for i := 0; i < ch.replicas; i++ {
		hash := crc32.ChecksumIEEE([]byte(fmt.Sprintf("%s-%d", node, i)))
		ch.ring[hash] = node
		ch.sorted = append(ch.sorted, hash)
	}
	sort.Slice(ch.sorted, func(i, j int) bool { return ch.sorted[i] < ch.sorted[j] })
}

func (ch *ConsistentHash) Get(key string) string {
	ch.mu.RLock()
	defer ch.mu.RUnlock()
	if len(ch.sorted) == 0 {
		return ""
	}
	hash := crc32.ChecksumIEEE([]byte(key))
	idx := sort.Search(len(ch.sorted), func(i int) bool { return ch.sorted[i] >= hash })
	if idx >= len(ch.sorted) {
		idx = 0
	}
	return ch.ring[ch.sorted[idx]]
}

func (ch *ConsistentHash) Name() string { return "ConsistentHash(一致性哈希)" }

// ============================================================
// 演示
// ============================================================

func RunLoadBalanceDemo() {
	nodes := []string{"10.0.0.1:8080", "10.0.0.2:8080", "10.0.0.3:8080"}

	fmt.Println("--- 示例1: 轮询 ---")
	rr := &RoundRobin{}
	distribution := testBalancer(rr, nodes, 12)
	printDistribution(rr.Name(), distribution)

	fmt.Println("\n--- 示例2: 随机 ---")
	rb := &RandomBalancer{}
	distribution = testBalancer(rb, nodes, 12)
	printDistribution(rb.Name(), distribution)

	fmt.Println("\n--- 示例3: 加权轮询 ---")
	wrr := NewWeightedRoundRobin([]*WeightedNode{
		{Addr: "10.0.0.1:8080 (权重5)", Weight: 5},
		{Addr: "10.0.0.2:8080 (权重3)", Weight: 3},
		{Addr: "10.0.0.3:8080 (权重1)", Weight: 1},
	})
	distribution = testBalancer(wrr, nodes, 18)
	printDistribution(wrr.Name(), distribution)

	fmt.Println("\n--- 示例4: 一致性哈希 ---")
	ch := NewConsistentHash(100) // 100个虚拟节点
	for _, node := range nodes {
		ch.Add(node)
	}
	// 一致性哈希的特点: 相同key总是路由到相同节点
	fmt.Println("  一致性哈希: 相同key总是路由到相同节点")
	keys := []string{"user:101", "user:102", "user:103", "order:201", "order:202"}
	for _, key := range keys {
		fmt.Printf("    %s → %s\n", key, ch.Get(key))
	}
	// 验证: 再次查询相同key
	fmt.Println("  再次查询 (验证稳定性):")
	for _, key := range keys {
		fmt.Printf("    %s → %s ✓\n", key, ch.Get(key))
	}

	fmt.Println("\n  === 负载均衡算法选择指南 ===")
	fmt.Println("  轮询:       简单场景，节点性能一致")
	fmt.Println("  加权轮询:   节点性能不一致")
	fmt.Println("  随机:       简单且有效，大量请求下趋于均匀")
	fmt.Println("  一致性哈希: 缓存场景，需要相同key路由到相同节点")
	fmt.Println("  最少连接:   长连接场景（如WebSocket）")
}

func testBalancer(b Balancer, nodes []string, requests int) map[string]int {
	dist := make(map[string]int)
	for i := 0; i < requests; i++ {
		node := b.Pick(nodes)
		dist[node]++
	}
	return dist
}

func printDistribution(name string, dist map[string]int) {
	fmt.Printf("  %s 分配结果:\n", name)
	for node, count := range dist {
		fmt.Printf("    %s: %d 次\n", node, count)
	}
}
