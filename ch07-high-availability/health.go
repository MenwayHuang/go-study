package main

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// ============================================================
// 健康检查 (Health Check)
// ============================================================
// 关键点:
//   K8s 两种健康检查:
//   - Liveness Probe:  进程是否活着? 失败 → 重启容器
//   - Readiness Probe: 能否接受流量? 失败 → 从负载均衡中摘除
//
//   Liveness: 检查进程本身 (死锁检测、goroutine泄漏)
//   Readiness: 检查依赖 (数据库连接、Redis连接、下游服务)
//
//   区别很关键:
//   - 数据库断了 → Readiness=false (摘流量), Liveness=true (不重启)
//   - 死锁了     → Liveness=false (重启)

// HealthStatus 健康状态
type HealthStatus string

const (
	StatusUp   HealthStatus = "UP"
	StatusDown HealthStatus = "DOWN"
)

// HealthCheck 健康检查结果
type HealthCheck struct {
	Status  HealthStatus          `json:"status"`
	Time    string                `json:"time"`
	Details map[string]CheckDetail `json:"details"`
}

type CheckDetail struct {
	Status  HealthStatus `json:"status"`
	Message string       `json:"message,omitempty"`
	Latency string       `json:"latency,omitempty"`
}

// HealthChecker 健康检查器
type HealthChecker struct {
	mu     sync.RWMutex
	checks map[string]func() CheckDetail // 注册的检查函数
}

func NewHealthChecker() *HealthChecker {
	return &HealthChecker{
		checks: make(map[string]func() CheckDetail),
	}
}

// Register 注册一个依赖的健康检查
func (hc *HealthChecker) Register(name string, check func() CheckDetail) {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	hc.checks[name] = check
}

// Check 执行所有健康检查
func (hc *HealthChecker) Check() HealthCheck {
	hc.mu.RLock()
	defer hc.mu.RUnlock()

	result := HealthCheck{
		Status:  StatusUp,
		Time:    time.Now().Format(time.RFC3339),
		Details: make(map[string]CheckDetail),
	}

	for name, check := range hc.checks {
		detail := check()
		result.Details[name] = detail
		if detail.Status == StatusDown {
			result.Status = StatusDown // 任一依赖不健康 → 整体不健康
		}
	}
	return result
}

// CheckJSON 返回JSON格式（用于HTTP endpoint）
func (hc *HealthChecker) CheckJSON() string {
	result := hc.Check()
	data, _ := json.MarshalIndent(result, "  ", "  ")
	return string(data)
}

// ============================================================
// 常见依赖检查函数
// ============================================================

// CheckMySQL 模拟MySQL连接检查
func CheckMySQL(healthy bool) func() CheckDetail {
	return func() CheckDetail {
		start := time.Now()
		if healthy {
			time.Sleep(5 * time.Millisecond) // 模拟ping
			return CheckDetail{
				Status:  StatusUp,
				Message: "mysql ping ok",
				Latency: time.Since(start).String(),
			}
		}
		return CheckDetail{
			Status:  StatusDown,
			Message: "mysql connection refused",
			Latency: time.Since(start).String(),
		}
	}
}

// CheckRedis 模拟Redis连接检查
func CheckRedis(healthy bool) func() CheckDetail {
	return func() CheckDetail {
		start := time.Now()
		if healthy {
			time.Sleep(2 * time.Millisecond)
			return CheckDetail{
				Status:  StatusUp,
				Message: "redis pong",
				Latency: time.Since(start).String(),
			}
		}
		return CheckDetail{
			Status:  StatusDown,
			Message: "redis timeout",
			Latency: time.Since(start).String(),
		}
	}
}

// ============================================================
// 演示
// ============================================================

func RunHealthCheckDemo() {
	fmt.Println("--- 示例: 健康检查 (K8s Readiness/Liveness) ---")

	checker := NewHealthChecker()

	// 场景1: 所有依赖健康
	fmt.Println("\n  === 场景1: 所有依赖健康 ===")
	checker.Register("mysql", CheckMySQL(true))
	checker.Register("redis", CheckRedis(true))
	fmt.Printf("  GET /health 响应:\n  %s\n", checker.CheckJSON())

	// 场景2: Redis不可用
	fmt.Println("\n  === 场景2: Redis故障 ===")
	checker.Register("redis", CheckRedis(false)) // 模拟Redis挂了
	fmt.Printf("  GET /health 响应:\n  %s\n", checker.CheckJSON())

	// 说明K8s配置
	fmt.Println("\n  === K8s 健康检查配置示例 ===")
	fmt.Println(`  # deployment.yaml
  livenessProbe:
    httpGet:
      path: /health/live    # 只检查进程本身
      port: 8080
    initialDelaySeconds: 10  # 启动后10秒开始检查
    periodSeconds: 15        # 每15秒检查一次
    failureThreshold: 3      # 连续3次失败则重启
  
  readinessProbe:
    httpGet:
      path: /health/ready   # 检查所有依赖
      port: 8080
    initialDelaySeconds: 5
    periodSeconds: 10
    failureThreshold: 3      # 连续3次失败则摘流量`)

	fmt.Println("\n  === 健康检查设计原则 ===")
	fmt.Println("  1. Liveness 只检查进程自身 (不检查外部依赖)")
	fmt.Println("  2. Readiness 检查所有必要依赖 (DB/Cache/下游服务)")
	fmt.Println("  3. 健康检查要快 (<1s), 不要做重操作")
	fmt.Println("  4. 检查要有超时，避免检查本身阻塞")
	fmt.Println("  5. 启动时给足初始化时间 (initialDelaySeconds)")
}
