package main

import (
	"crypto/rand"
	"fmt"
	"encoding/hex"
	"sync"
	"time"
)

// ============================================================
// 幂等设计 (Idempotency)
// ============================================================
// 关键点:
//   - 同一个请求执行一次和执行多次，结果相同
//   - 网络超时时客户端不知道服务端是否已处理 → 重试可能导致重复
//   - 幂等设计保证重复请求不会产生副作用
//
// 常见方案:
//   1. 幂等Token: 客户端生成唯一token，服务端去重
//   2. 数据库唯一索引: 利用DB的唯一约束防重
//   3. 状态机: 订单状态只能单向流转，重复请求被状态检查拦截

// IdempotentStore 幂等存储（生产中用 Redis SETNX 实现）
type IdempotentStore struct {
	mu     sync.Mutex
	tokens map[string]*IdempotentRecord
}

type IdempotentRecord struct {
	Token     string
	Result    interface{}
	CreatedAt time.Time
	Processed bool
}

func NewIdempotentStore() *IdempotentStore {
	return &IdempotentStore{
		tokens: make(map[string]*IdempotentRecord),
	}
}

// CheckAndMark 检查token是否已处理，未处理则标记
// 返回: (是否已处理, 之前的结果)
// 关键点: 这个操作必须是原子的（锁/Redis SETNX）
func (s *IdempotentStore) CheckAndMark(token string) (bool, interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if record, exists := s.tokens[token]; exists {
		if record.Processed {
			return true, record.Result // 已处理过，返回之前的结果
		}
	}

	// 标记为处理中
	s.tokens[token] = &IdempotentRecord{
		Token:     token,
		CreatedAt: time.Now(),
		Processed: false,
	}
	return false, nil
}

// MarkDone 标记token处理完成，存储结果
func (s *IdempotentStore) MarkDone(token string, result interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if record, exists := s.tokens[token]; exists {
		record.Processed = true
		record.Result = result
	}
}

// ============================================================
// 幂等支付服务示例
// ============================================================

type PaymentService struct {
	store   *IdempotentStore
	balance map[string]float64 // 用户余额
	mu      sync.Mutex
}

func NewPaymentService() *PaymentService {
	return &PaymentService{
		store: NewIdempotentStore(),
		balance: map[string]float64{
			"user-001": 1000.0,
			"user-002": 500.0,
		},
	}
}

// Pay 幂等扣款
// 关键点: 客户端传入 idempotencyKey (通常是订单号或UUID)
//   - 第一次请求: 正常扣款
//   - 重复请求: 直接返回第一次的结果，不重复扣款
func (ps *PaymentService) Pay(idempotencyKey, userID string, amount float64) (string, error) {
	// 1. 幂等检查
	processed, prevResult := ps.store.CheckAndMark(idempotencyKey)
	if processed {
		fmt.Printf("    [幂等] 请求已处理过，返回缓存结果\n")
		return prevResult.(string), nil
	}

	// 2. 执行实际扣款逻辑
	ps.mu.Lock()
	balance, exists := ps.balance[userID]
	if !exists {
		ps.mu.Unlock()
		return "", fmt.Errorf("用户不存在: %s", userID)
	}
	if balance < amount {
		ps.mu.Unlock()
		return "", fmt.Errorf("余额不足: %.2f < %.2f", balance, amount)
	}
	ps.balance[userID] = balance - amount
	newBalance := ps.balance[userID]
	ps.mu.Unlock()

	result := fmt.Sprintf("扣款成功: %.2f, 剩余余额: %.2f", amount, newBalance)

	// 3. 标记完成并存储结果
	ps.store.MarkDone(idempotencyKey, result)

	return result, nil
}

func (ps *PaymentService) GetBalance(userID string) float64 {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	return ps.balance[userID]
}

// generateToken 生成幂等Token
func generateToken() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// ============================================================
// 演示
// ============================================================

func RunIdempotentDemo() {
	fmt.Println("--- 示例: 幂等扣款 — 防止重复支付 ---")

	ps := NewPaymentService()
	userID := "user-001"
	fmt.Printf("  初始余额: %.2f\n\n", ps.GetBalance(userID))

	// 场景1: 正常支付
	fmt.Println("  === 场景1: 正常支付 ===")
	token1 := generateToken()
	result, err := ps.Pay(token1, userID, 100.0)
	if err != nil {
		fmt.Printf("  支付失败: %v\n", err)
	} else {
		fmt.Printf("  支付结果: %s\n", result)
	}

	// 场景2: 网络超时，客户端重试（相同token）
	fmt.Println("\n  === 场景2: 模拟网络超时，客户端用相同token重试3次 ===")
	for i := 0; i < 3; i++ {
		result, err := ps.Pay(token1, userID, 100.0) // 相同的token!
		if err != nil {
			fmt.Printf("  重试%d: 失败 %v\n", i+1, err)
		} else {
			fmt.Printf("  重试%d: %s\n", i+1, result)
		}
	}
	fmt.Printf("  最终余额: %.2f (只扣了一次!) ✓\n", ps.GetBalance(userID))

	// 场景3: 不同token = 不同支付
	fmt.Println("\n  === 场景3: 新订单（不同token）===")
	token2 := generateToken()
	result, _ = ps.Pay(token2, userID, 200.0)
	fmt.Printf("  支付结果: %s\n", result)
	fmt.Printf("  最终余额: %.2f\n", ps.GetBalance(userID))

	fmt.Println("\n  === 幂等设计方案总结 ===")
	fmt.Println("  Token去重:    客户端生成UUID，服务端Redis SETNX检查")
	fmt.Println("  数据库唯一索引: 利用订单号唯一索引，重复插入报错")
	fmt.Println("  状态机:       订单状态 待支付→已支付, 重复请求被状态拦截")
	fmt.Println("  乐观锁:       UPDATE ... WHERE version=X, 版本不匹配则失败")
}
