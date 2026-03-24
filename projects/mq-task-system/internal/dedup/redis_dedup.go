package dedup

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisDedup struct {
	Client *redis.Client
	TTL    time.Duration
}

func NewRedisDedup(addr string, ttl time.Duration) *RedisDedup {
	c := redis.NewClient(&redis.Options{Addr: addr})
	return &RedisDedup{Client: c, TTL: ttl}
}

func (d *RedisDedup) Close() {
	_ = d.Client.Close()
}

// SeenBefore 使用 SETNX 实现幂等：
// - 返回 true 代表已处理过（重复消息）
// - 返回 false 代表首次处理（可以执行业务）
func (d *RedisDedup) SeenBefore(ctx context.Context, taskID string) (bool, error) {
	key := fmt.Sprintf("task:dedup:%s", taskID)
	ok, err := d.Client.SetNX(ctx, key, 1, d.TTL).Result()
	if err != nil {
		return false, err
	}
	return !ok, nil
}
