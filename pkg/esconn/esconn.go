package esconn

import (
	"context"
	"fmt"
	"log"

	"github.com/elastic/go-elasticsearch/v8"
)

type Config struct {
	Host string
	Port int
}

func Init(ctx context.Context, cfg Config) (*elasticsearch.Client, error) {
	addr := fmt.Sprintf("http://%s:%d", cfg.Host, cfg.Port)

	es, err := elasticsearch.NewClient(elasticsearch.Config{
		Addresses: []string{addr},
		Logger:    nil, // 生产可替换为 zap adapter
	})
	if err != nil {
		return nil, fmt.Errorf("esconn: 创建 ES 客户端失败: %w", err)
	}

	res, err := es.Ping()
	if err != nil {
		return nil, fmt.Errorf("esconn: ES ping 失败: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return nil, fmt.Errorf("esconn: ES 返回错误状态 %s", res.Status())
	}

	log.Printf("[esconn] ES connected at %s", addr)
	return es, nil
}

// IndexName 产品搜索索引名称
const ProductIndex = "products"
