package main

import (
	"fmt"
	"sync/atomic"
	"time"
)

// ============================================================
// 优雅降级 (Graceful Degradation)
// ============================================================
// 关键点:
//   - 核心功能保证可用，非核心功能可降级
//   - 降级不是"关闭"，而是提供"有损但可用"的服务
//   - 降级策略: 返回默认值 / 返回缓存数据 / 简化处理逻辑
//
// 实际场景:
//   - 推荐服务挂了 → 返回热门商品（而不是空白页面）
//   - 评论服务慢了 → 只显示前10条评论
//   - 库存查询超时 → 显示"库存充足"（而不是错误页面）

// DegradeLevel 降级级别
type DegradeLevel int

const (
	LevelNormal   DegradeLevel = iota // 正常
	LevelWarn                          // 警告级别: 部分功能降级
	LevelCritical                      // 严重级别: 只保核心功能
)

func (l DegradeLevel) String() string {
	switch l {
	case LevelNormal:
		return "NORMAL(正常)"
	case LevelWarn:
		return "WARN(部分降级)"
	case LevelCritical:
		return "CRITICAL(核心保护)"
	}
	return "UNKNOWN"
}

// DegradeManager 降级管理器
// 关键点:
//   - 根据系统负载/错误率自动调整降级级别
//   - 每个功能注册自己的降级策略
//   - 提供手动降级开关（运维紧急操作）
type DegradeManager struct {
	level    int32 // 使用atomic操作
	handlers map[string]DegradeHandler
}

type DegradeHandler struct {
	Normal   func() (interface{}, error) // 正常处理
	Degraded func() (interface{}, error) // 降级处理
}

func NewDegradeManager() *DegradeManager {
	return &DegradeManager{
		handlers: make(map[string]DegradeHandler),
	}
}

func (dm *DegradeManager) SetLevel(level DegradeLevel) {
	atomic.StoreInt32(&dm.level, int32(level))
}

func (dm *DegradeManager) GetLevel() DegradeLevel {
	return DegradeLevel(atomic.LoadInt32(&dm.level))
}

// Register 注册功能的正常和降级处理函数
func (dm *DegradeManager) Register(name string, handler DegradeHandler) {
	dm.handlers[name] = handler
}

// Execute 执行功能，根据降级级别自动选择处理方式
func (dm *DegradeManager) Execute(name string) (interface{}, error) {
	handler, ok := dm.handlers[name]
	if !ok {
		return nil, fmt.Errorf("未注册的功能: %s", name)
	}

	level := dm.GetLevel()

	switch level {
	case LevelNormal:
		// 正常执行，如果失败则自动降级
		result, err := handler.Normal()
		if err != nil {
			fmt.Printf("    [降级] %s 正常调用失败，自动降级: %v\n", name, err)
			return handler.Degraded()
		}
		return result, nil

	case LevelWarn, LevelCritical:
		// 直接走降级逻辑
		fmt.Printf("    [降级] 当前级别=%s, %s 直接降级处理\n", level, name)
		return handler.Degraded()
	}

	return handler.Normal()
}

// ============================================================
// 演示
// ============================================================

func RunDegradeDemo() {
	fmt.Println("--- 示例: 优雅降级 — 商品详情页 ---")
	fmt.Println("  场景: 商品详情页需要调用 推荐服务 + 评论服务 + 库存服务")
	fmt.Println("  当服务不可用时，降级提供有损但可用的响应\n")

	dm := NewDegradeManager()

	// 注册: 推荐服务
	dm.Register("recommendation", DegradeHandler{
		Normal: func() (interface{}, error) {
			time.Sleep(30 * time.Millisecond) // 模拟正常调用
			return []string{"推荐商品A", "推荐商品B", "推荐商品C"}, nil
		},
		Degraded: func() (interface{}, error) {
			// 降级: 返回热门商品（预缓存的静态数据）
			return []string{"热门商品1", "热门商品2"}, nil
		},
	})

	// 注册: 评论服务
	dm.Register("comments", DegradeHandler{
		Normal: func() (interface{}, error) {
			return nil, fmt.Errorf("评论服务超时") // 模拟故障
		},
		Degraded: func() (interface{}, error) {
			// 降级: 返回缓存的评论
			return "暂时无法加载评论，请稍后刷新", nil
		},
	})

	// 注册: 库存服务
	dm.Register("stock", DegradeHandler{
		Normal: func() (interface{}, error) {
			time.Sleep(20 * time.Millisecond)
			return map[string]interface{}{"count": 42, "status": "充足"}, nil
		},
		Degraded: func() (interface{}, error) {
			// 降级: 显示"库存充足"而不是错误
			return map[string]interface{}{"count": -1, "status": "库存充足(降级)"}, nil
		},
	})

	// 场景1: 正常状态
	fmt.Println("  === 场景1: 正常状态 ===")
	dm.SetLevel(LevelNormal)
	executeAll(dm)

	// 场景2: 系统压力大，手动降级
	fmt.Println("\n  === 场景2: 系统压力大，降级到WARN ===")
	dm.SetLevel(LevelWarn)
	executeAll(dm)

	// 场景3: 恢复正常
	fmt.Println("\n  === 场景3: 恢复正常 ===")
	dm.SetLevel(LevelNormal)
	executeAll(dm)

	fmt.Println("\n  === 降级设计原则 ===")
	fmt.Println("  1. 核心功能不降级 (下单、支付)")
	fmt.Println("  2. 非核心功能优先降级 (推荐、评论、积分)")
	fmt.Println("  3. 降级方案提前设计好，不能临时想")
	fmt.Println("  4. 降级开关支持动态配置 (配置中心/运维平台)")
	fmt.Println("  5. 降级后要有监控告警，及时恢复")
}

func executeAll(dm *DegradeManager) {
	services := []string{"recommendation", "comments", "stock"}
	for _, svc := range services {
		result, err := dm.Execute(svc)
		if err != nil {
			fmt.Printf("  ✗ %s: 错误=%v\n", svc, err)
		} else {
			fmt.Printf("  ✓ %s: %v\n", svc, result)
		}
	}
}
