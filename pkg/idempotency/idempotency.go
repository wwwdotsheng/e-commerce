package idempotency

import (
	"context"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
)

type Manager struct {
	client *redis.Client
	prefix string
}

func NewManager(client *redis.Client, prefix string) *Manager {
	return &Manager{
		client: client,
		prefix: prefix,
	}
}

func buildFullKey(prefix, key string) string {
	return fmt.Sprintf("idemp:%s:%s", prefix, key)
}

func (m *Manager) CheckAndSet(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	fullKey := buildFullKey(m.prefix, key)
	success, err := m.client.SetNX(ctx, fullKey, "PROCESSING", ttl).Result()
	if err != nil {
		return false, fmt.Errorf("redis idempotency check failed: %w", err)
	}
	return success, nil
}

func (m *Manager) Delete(ctx context.Context, key string) error {
	fullKey := buildFullKey(m.prefix, key)
	return m.client.Del(ctx, fullKey).Err()
}
