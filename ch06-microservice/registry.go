package main

import (
	"fmt"
	"sync"
	"time"
)

// ============================================================
// 服务注册与发现 — 微服务的基础设施
// ============================================================

// ServiceInstance 服务实例
type ServiceInstance struct {
	ID       string
	Name     string // 服务名
	Address  string // IP:Port
	Metadata map[string]string
	LastBeat time.Time // 最后心跳时间
}

// Registry 内存版服务注册中心
// 关键点:
//   - 服务启动时注册自己
//   - 服务定期发送心跳
//   - 注册中心定期清理无心跳的实例
//   - 调用方通过服务名获取可用实例列表
type Registry struct {
	mu       sync.RWMutex
	services map[string][]*ServiceInstance // serviceName → instances
	ttl      time.Duration                 // 心跳超时
	stopCh   chan struct{}
}

func NewRegistry(heartbeatTTL time.Duration) *Registry {
	r := &Registry{
		services: make(map[string][]*ServiceInstance),
		ttl:      heartbeatTTL,
		stopCh:   make(chan struct{}),
	}
	go r.healthCheck()
	return r
}

// Register 注册服务实例
func (r *Registry) Register(instance *ServiceInstance) {
	r.mu.Lock()
	defer r.mu.Unlock()
	instance.LastBeat = time.Now()
	r.services[instance.Name] = append(r.services[instance.Name], instance)
	fmt.Printf("  📝 注册服务: %s (%s) at %s\n", instance.Name, instance.ID, instance.Address)
}

// Deregister 注销服务实例
func (r *Registry) Deregister(name, id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	instances := r.services[name]
	for i, inst := range instances {
		if inst.ID == id {
			r.services[name] = append(instances[:i], instances[i+1:]...)
			fmt.Printf("  🗑️ 注销服务: %s (%s)\n", name, id)
			return
		}
	}
}

// Heartbeat 心跳续约
func (r *Registry) Heartbeat(name, id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, inst := range r.services[name] {
		if inst.ID == id {
			inst.LastBeat = time.Now()
			return
		}
	}
}

// Discover 服务发现 — 获取某服务的所有健康实例
func (r *Registry) Discover(name string) []*ServiceInstance {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.services[name]
}

// healthCheck 后台健康检查，清理无心跳的实例
func (r *Registry) healthCheck() {
	ticker := time.NewTicker(r.ttl / 2)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			r.mu.Lock()
			now := time.Now()
			for name, instances := range r.services {
				alive := make([]*ServiceInstance, 0)
				for _, inst := range instances {
					if now.Sub(inst.LastBeat) < r.ttl {
						alive = append(alive, inst)
					} else {
						fmt.Printf("  💀 健康检查: %s (%s) 心跳超时，移除\n", name, inst.ID)
					}
				}
				r.services[name] = alive
			}
			r.mu.Unlock()
		case <-r.stopCh:
			return
		}
	}
}

func (r *Registry) Stop() {
	close(r.stopCh)
}

// ============================================================
// 演示
// ============================================================

func RunRegistryDemo() {
	fmt.Println("--- 示例: 服务注册、发现、心跳、健康检查 ---")

	registry := NewRegistry(500 * time.Millisecond) // 500ms无心跳则移除
	defer registry.Stop()

	// 注册3个 order-service 实例
	instances := []*ServiceInstance{
		{ID: "order-1", Name: "order-service", Address: "10.0.0.1:8080"},
		{ID: "order-2", Name: "order-service", Address: "10.0.0.2:8080"},
		{ID: "order-3", Name: "order-service", Address: "10.0.0.3:8080"},
	}
	for _, inst := range instances {
		registry.Register(inst)
	}

	// 服务发现
	discovered := registry.Discover("order-service")
	fmt.Printf("\n  发现 order-service 实例: %d 个\n", len(discovered))
	for _, inst := range discovered {
		fmt.Printf("    - %s @ %s\n", inst.ID, inst.Address)
	}

	// 模拟: order-3 停止发送心跳 (宕机)
	fmt.Println("\n  模拟: order-1 和 order-2 持续心跳, order-3 宕机...")
	go func() {
		for i := 0; i < 5; i++ {
			registry.Heartbeat("order-service", "order-1")
			registry.Heartbeat("order-service", "order-2")
			// order-3 不发心跳!
			time.Sleep(200 * time.Millisecond)
		}
	}()

	time.Sleep(800 * time.Millisecond)

	// 再次发现
	discovered = registry.Discover("order-service")
	fmt.Printf("\n  健康检查后, order-service 可用实例: %d 个\n", len(discovered))
	for _, inst := range discovered {
		fmt.Printf("    - %s @ %s ✓\n", inst.ID, inst.Address)
	}
}
