package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// ============================================================
// 高并发秒杀系统 — 综合实战
// ============================================================
//
// 【整体架构 — 5层漏斗过滤】
//
//   用户请求 (10万QPS)
//       │
//   ┌───▼────────────────┐
//   │ 第1层: 令牌桶限流     │  只放行 1000QPS, 其余直接返回429
//   │ (middleware.go)     │  → 保护后端不被打垮
//   └───┬────────────────┘
//       │
//   ┌───▼────────────────┐
//   │ 第2层: 本地缓存售罄标记│  sync.Map存"商品已售罄"
//   │ (soldOut)           │  → 纳秒级判断, 不走后续逻辑
//   └───┬────────────────┘
//       │
//   ┌───▼────────────────┐
//   │ 第3层: 幂等去重       │  同一用户只能抢一次
//   │ (userBought)        │  → 生产用Redis SETNX
//   └───┬────────────────┘
//       │
//   ┌───▼────────────────┐
//   │ 第4层: 原子减库存     │  atomic.AddInt64(&stock, -1)
//   │ (模拟Redis DECR)    │  → CAS无锁操作, 绝不超卖
//   └───┬────────────────┘
//       │ stock >= 0
//   ┌───▼────────────────┐
//   │ 第5层: 消息队列异步    │  chan *BuyRequest (容量10000)
//   │ (orderQueue)        │  → 削峰填谷, 快速返回用户
//   └───┬────────────────┘
//       │
//   ┌───▼────────────────┐
//   │ Worker Pool消费      │  5个goroutine并发消费
//   │ 创建订单写DB          │  → 生产中写MySQL + 发通知
//   └────────────────────┘
//
// 【运行方式】
//   go run . -mode server    ← 启动秒杀HTTP服务 (默认)
//   go run . -mode stress    ← 运行压测客户端 (需要先启动server)
//
// 【关键设计】
//   - 不依赖外部组件(Redis/MySQL), 全部内存模拟, 聚焦架构设计
//   - 每一层都对应生产环境中的一个真实组件
//   - 压测工具内置, 可以直接观测限流/超卖防护/异步效果

func main() {
	mode := flag.String("mode", "server", "运行模式: server(启动秒杀服务) / stress(压测客户端)")
	flag.Parse()

	if *mode == "stress" {
		RunStressTest()
		return
	}

	// 初始化秒杀系统
	system := NewSeckillSystem()

	// 初始化商品库存 (模拟"库存预热")
	system.InitProduct("PROD-001", "iPhone 限量版", 100, 5999.0)
	system.InitProduct("PROD-002", "MacBook 限量版", 50, 12999.0)

	// 启动订单处理Worker
	system.StartWorkers(5) // 5个Worker并发消费

	// 创建HTTP路由
	mux := http.NewServeMux()
	mux.HandleFunc("/seckill/stock", system.StockHandler)  // 查询库存
	mux.HandleFunc("/seckill/buy", system.BuyHandler)       // 秒杀抢购
	mux.HandleFunc("/seckill/orders", system.OrdersHandler) // 查看订单
	mux.HandleFunc("/seckill/stats", system.StatsHandler)   // 系统统计

	// 包裹中间件
	handler := chainMiddleware(mux,
		logMiddleware,
		recoveryMiddleware,
		rateLimitMiddleware(500, 1000), // 每秒500请求，桶容量1000
	)

	server := &http.Server{
		Addr:         ":8080",
		Handler:      handler,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go func() {
		fmt.Println("🔥 秒杀系统启动: http://localhost:8080")
		fmt.Println("   查询库存: GET  /seckill/stock?product_id=PROD-001")
		fmt.Println("   秒杀抢购: POST /seckill/buy")
		fmt.Println("   查看订单: GET  /seckill/orders")
		fmt.Println("   系统统计: GET  /seckill/stats")
		fmt.Println("   按 Ctrl+C 优雅关闭")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("启动失败: %v", err)
		}
	}()

	// 优雅关闭
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	fmt.Println("\n📛 关闭中... 等待订单处理完成")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	server.Shutdown(ctx)
	system.Shutdown()

	fmt.Println("✅ 秒杀系统已安全关闭")
	system.PrintStats()
}

// ============================================================
// 秒杀系统核心
// ============================================================

type Product struct {
	ID    string
	Name  string
	Stock int64   // 原子操作
	Price float64
}

type Order struct {
	ID        string    `json:"id"`
	ProductID string    `json:"product_id"`
	UserID    string    `json:"user_id"`
	Price     float64   `json:"price"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

type BuyRequest struct {
	ProductID string `json:"product_id"`
	UserID    string `json:"user_id"`
}

type SeckillSystem struct {
	products   sync.Map               // productID → *Product
	orders     sync.Map               // orderID → *Order
	orderQueue chan *BuyRequest        // 消息队列
	soldOut    sync.Map               // productID → bool 售罄标记(本地缓存)
	userBought sync.Map               // "productID:userID" → bool 幂等去重
	workerWg   sync.WaitGroup
	ctx        context.Context
	cancel     context.CancelFunc

	// 统计指标
	totalRequests   int64
	rejectedByLimit int64
	rejectedBySold  int64
	rejectedByStock int64
	rejectedByDup   int64
	successOrders   int64
	orderCounter    int64
}

func NewSeckillSystem() *SeckillSystem {
	ctx, cancel := context.WithCancel(context.Background())
	return &SeckillSystem{
		orderQueue: make(chan *BuyRequest, 10000), // 队列容量1万
		ctx:        ctx,
		cancel:     cancel,
	}
}

// InitProduct 库存预热: 将商品库存加载到内存
func (s *SeckillSystem) InitProduct(id, name string, stock int64, price float64) {
	s.products.Store(id, &Product{
		ID:    id,
		Name:  name,
		Stock: stock,
		Price: price,
	})
	fmt.Printf("  📦 商品预热: %s (%s) 库存=%d 价格=%.2f\n", id, name, stock, price)
}

// StartWorkers 启动订单处理Worker池
func (s *SeckillSystem) StartWorkers(num int) {
	for i := 0; i < num; i++ {
		s.workerWg.Add(1)
		go s.orderWorker(i)
	}
	fmt.Printf("  👷 启动 %d 个订单处理Worker\n", num)
}

// orderWorker 订单处理Worker (消费消息队列)
func (s *SeckillSystem) orderWorker(id int) {
	defer s.workerWg.Done()
	for {
		select {
		case req, ok := <-s.orderQueue:
			if !ok {
				return // 队列关闭
			}
			s.processOrder(id, req)
		case <-s.ctx.Done():
			// 关闭前处理完队列中剩余消息
			for {
				select {
				case req, ok := <-s.orderQueue:
					if !ok {
						return
					}
					s.processOrder(id, req)
				default:
					return
				}
			}
		}
	}
}

// processOrder 处理单个订单
func (s *SeckillSystem) processOrder(workerID int, req *BuyRequest) {
	val, ok := s.products.Load(req.ProductID)
	if !ok {
		return
	}
	product := val.(*Product)

	orderNum := atomic.AddInt64(&s.orderCounter, 1)
	order := &Order{
		ID:        fmt.Sprintf("ORD-%06d", orderNum),
		ProductID: req.ProductID,
		UserID:    req.UserID,
		Price:     product.Price,
		Status:    "success",
		CreatedAt: time.Now(),
	}

	s.orders.Store(order.ID, order)
	atomic.AddInt64(&s.successOrders, 1)
	log.Printf("[Worker%d] 订单创建成功: %s 用户:%s 商品:%s",
		workerID, order.ID, order.UserID, order.ProductID)
}

// Shutdown 优雅关闭
func (s *SeckillSystem) Shutdown() {
	s.cancel()
	close(s.orderQueue)
	s.workerWg.Wait() // 等待所有Worker处理完
}

// ============================================================
// HTTP Handlers
// ============================================================

// StockHandler 查询库存
func (s *SeckillSystem) StockHandler(w http.ResponseWriter, r *http.Request) {
	productID := r.URL.Query().Get("product_id")
	val, ok := s.products.Load(productID)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "商品不存在"})
		return
	}
	product := val.(*Product)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"product_id": product.ID,
		"name":       product.Name,
		"stock":      atomic.LoadInt64(&product.Stock),
		"price":      product.Price,
	})
}

// BuyHandler 秒杀抢购 — 核心接口
// 关键点: 多级过滤，逐层减少请求
func (s *SeckillSystem) BuyHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	atomic.AddInt64(&s.totalRequests, 1)

	var req BuyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "参数错误"})
		return
	}

	// === 第1层: 本地缓存判断是否售罄 (最快, 纳秒级) ===
	if _, soldOut := s.soldOut.Load(req.ProductID); soldOut {
		atomic.AddInt64(&s.rejectedBySold, 1)
		writeJSON(w, http.StatusOK, map[string]string{
			"status": "failed", "reason": "商品已售罄",
		})
		return
	}

	// === 第2层: 幂等检查 — 防止同一用户重复抢购 ===
	dupKey := req.ProductID + ":" + req.UserID
	if _, loaded := s.userBought.LoadOrStore(dupKey, true); loaded {
		atomic.AddInt64(&s.rejectedByDup, 1)
		writeJSON(w, http.StatusOK, map[string]string{
			"status": "failed", "reason": "您已参与过抢购",
		})
		return
	}

	// === 第3层: 原子减库存 (模拟Redis DECR) ===
	val, ok := s.products.Load(req.ProductID)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "商品不存在"})
		return
	}
	product := val.(*Product)

	newStock := atomic.AddInt64(&product.Stock, -1)
	if newStock < 0 {
		// 库存不足，回滚并标记售罄
		atomic.AddInt64(&product.Stock, 1)
		s.soldOut.Store(req.ProductID, true) // 标记售罄
		s.userBought.Delete(dupKey)          // 回滚幂等标记
		atomic.AddInt64(&s.rejectedByStock, 1)
		writeJSON(w, http.StatusOK, map[string]string{
			"status": "failed", "reason": "库存不足",
		})
		return
	}

	// === 第4层: 发送到消息队列，异步处理 ===
	select {
	case s.orderQueue <- &req:
		writeJSON(w, http.StatusOK, map[string]string{
			"status":  "success",
			"message": "抢购成功，订单处理中",
		})
	default:
		// 队列满了，回滚库存
		atomic.AddInt64(&product.Stock, 1)
		s.userBought.Delete(dupKey)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status": "failed", "reason": "系统繁忙，请重试",
		})
	}
}

// OrdersHandler 查看所有订单
func (s *SeckillSystem) OrdersHandler(w http.ResponseWriter, r *http.Request) {
	var orders []*Order
	s.orders.Range(func(key, value interface{}) bool {
		orders = append(orders, value.(*Order))
		return true
	})
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"total":  len(orders),
		"orders": orders,
	})
}

// StatsHandler 系统统计
func (s *SeckillSystem) StatsHandler(w http.ResponseWriter, r *http.Request) {
	stats := s.getStats()
	writeJSON(w, http.StatusOK, stats)
}

func (s *SeckillSystem) getStats() map[string]interface{} {
	stats := map[string]interface{}{
		"total_requests":    atomic.LoadInt64(&s.totalRequests),
		"rejected_by_sold":  atomic.LoadInt64(&s.rejectedBySold),
		"rejected_by_stock": atomic.LoadInt64(&s.rejectedByStock),
		"rejected_by_dup":   atomic.LoadInt64(&s.rejectedByDup),
		"success_orders":    atomic.LoadInt64(&s.successOrders),
		"queue_pending":     len(s.orderQueue),
	}

	// 各商品库存
	products := make(map[string]int64)
	s.products.Range(func(key, value interface{}) bool {
		p := value.(*Product)
		products[p.ID] = atomic.LoadInt64(&p.Stock)
		return true
	})
	stats["product_stock"] = products
	return stats
}

func (s *SeckillSystem) PrintStats() {
	fmt.Println("\n📊 秒杀系统统计:")
	stats := s.getStats()
	data, _ := json.MarshalIndent(stats, "  ", "  ")
	fmt.Printf("  %s\n", string(data))
}

// ============================================================
// 工具函数
// ============================================================

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
