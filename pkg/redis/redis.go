package redis

import (
	"context"
	"e-commerce/internal/config"
	"fmt"

	"github.com/go-redis/redis/v8"
)

func Init(ctx context.Context, config config.RedisSection) *redis.Client {
	rdb := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", config.Host, config.Port),
		Password: config.Password,
		DB:       config.DB,
		PoolSize: config.PoolSize,
	})
	return rdb
}
