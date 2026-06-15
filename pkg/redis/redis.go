package redis

import (
	"context"
	"fmt"

	"github.com/go-redis/redis/v8"
)

type Config struct {
	Host     string
	Port     int
	Password string
	DB       int
	PoolSize int
}

func Init(ctx context.Context, config Config) (*redis.Client, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", config.Host, config.Port),
		Password: config.Password,
		DB:       config.DB,
		PoolSize: config.PoolSize,
	})

	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("Redis 连接失败: %w", err)
	}

	return rdb, nil
}
