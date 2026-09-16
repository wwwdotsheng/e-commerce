package product

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"e-commerce/internal/model"
	"e-commerce/pkg/esconn"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type searchRepo struct {
	es     *elasticsearch.Client
	logger *zap.Logger
}

// doc 是索引到 ES 的产品文档
type doc struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Price       float64 `json:"price"`
	Status      string  `json:"status"`
	Publisher   string  `json:"publisher"`
	ShopID      string  `json:"shop_id"`
	CreatedAt   string  `json:"created_at"`
}

func newSearchRepo(es *elasticsearch.Client, logger *zap.Logger) *searchRepo {
	return &searchRepo{es: es, logger: logger}
}

// index 索引一个产品（创建或全量更新）
func (r *searchRepo) index(ctx context.Context, p *model.Product) {
	shopID := ""
	if p.ShopID != nil {
		shopID = p.ShopID.String()
	}
	d := doc{
		ID:          p.ID.String(),
		Name:        p.Name,
		Description: p.Description,
		Price:       p.Price,
		Status:      string(p.Status),
		Publisher:   p.Publisher.String(),
		ShopID:      shopID,
		CreatedAt:   p.CreatedAt.Format(time.RFC3339),
	}

	body, _ := json.Marshal(d)
	res, err := r.es.Index(esconn.ProductIndex,
		bytes.NewReader(body),
		r.es.Index.WithDocumentID(d.ID),
		r.es.Index.WithContext(ctx),
	)
	if err != nil {
		r.logger.Warn("ES index failed", zap.String("id", d.ID), zap.Error(err))
		return
	}
	defer res.Body.Close()
	if res.IsError() {
		r.logger.Warn("ES index error", zap.String("id", d.ID), zap.String("status", res.Status()))
	}
}

// remove 从 ES 中删除产品
func (r *searchRepo) remove(ctx context.Context, id string) {
	res, err := r.es.Delete(esconn.ProductIndex, id,
		r.es.Delete.WithContext(ctx),
	)
	if err != nil {
		r.logger.Warn("ES delete failed", zap.String("id", id), zap.Error(err))
		return
	}
	defer res.Body.Close()
	if res.IsError() {
		r.logger.Warn("ES delete error", zap.String("id", id), zap.String("status", res.Status()))
	}
}

// search 搜索产品，返回匹配的 ID 列表
func (r *searchRepo) search(ctx context.Context, query string, min, max *float64, page, size int, shopID *uuid.UUID) ([]string, int64, error) {
	var must []map[string]any

	if query != "" {
		must = append(must, map[string]any{
			"multi_match": map[string]any{
				"query":  query,
				"fields": []string{"name^3", "description"},
			},
		})
	}

	var filters []map[string]any
	if min != nil || max != nil {
		priceFilter := map[string]any{}
		if min != nil {
			priceFilter["gte"] = *min
		}
		if max != nil {
			priceFilter["lte"] = *max
		}
		filters = append(filters, map[string]any{"range": map[string]any{"price": priceFilter}})
	}

	if shopID != nil {
		filters = append(filters, map[string]any{"term": map[string]any{"shop_id.keyword": shopID.String()}})
	}

	body := map[string]any{
		"query": map[string]any{
			"bool": map[string]any{
				"must":   must,
				"filter": filters,
			},
		},
		"from":             (page - 1) * size,
		"size":             size,
		"track_total_hits": true,
		"sort": []map[string]any{
			{"_score": map[string]any{"order": "desc"}},
			{"created_at": map[string]any{"order": "desc"}},
		},
	}

	buf, _ := json.Marshal(body)
	res, err := r.es.Search(
		r.es.Search.WithContext(ctx),
		r.es.Search.WithIndex(esconn.ProductIndex),
		r.es.Search.WithBody(bytes.NewReader(buf)),
	)
	if err != nil {
		return nil, 0, fmt.Errorf("ES search failed: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return nil, 0, fmt.Errorf("ES search error: %s", res.Status())
	}

	var result struct {
		Hits struct {
			Total struct {
				Value int64 `json:"value"`
			} `json:"total"`
			Hits []struct {
				ID     string `json:"_id"`
				Source doc    `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}

	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		return nil, 0, fmt.Errorf("ES decode failed: %w", err)
	}

	ids := make([]string, len(result.Hits.Hits))
	for i, hit := range result.Hits.Hits {
		ids[i] = hit.ID
	}

	return ids, result.Hits.Total.Value, nil
}

// rebuildIndex 全量重建索引，使用 bulk API 批量写入
func (r *searchRepo) rebuildIndex(ctx context.Context, products []*model.Product) error {
	if len(products) == 0 {
		return nil
	}

	const batchSize = 10000
	for i := 0; i < len(products); i += batchSize {
		end := i + batchSize
		if end > len(products) {
			end = len(products)
		}
		batch := products[i:end]

		var buf bytes.Buffer
		for _, p := range batch {
			meta := map[string]any{
				"index": map[string]any{
					"_index": esconn.ProductIndex,
					"_id":    p.ID.String(),
				},
			}
			shopID := ""
			if p.ShopID != nil {
				shopID = p.ShopID.String()
			}
			body := doc{
				ID:          p.ID.String(),
				Name:        p.Name,
				Description: p.Description,
				Price:       p.Price,
				Status:      string(p.Status),
				Publisher:   p.Publisher.String(),
				ShopID:      shopID,
				CreatedAt:   p.CreatedAt.Format(time.RFC3339),
			}

			metaBytes, _ := json.Marshal(meta)
			bodyBytes, _ := json.Marshal(body)
			buf.Write(metaBytes)
			buf.WriteByte('\n')
			buf.Write(bodyBytes)
			buf.WriteByte('\n')
		}

		res, err := r.es.Bulk(bytes.NewReader(buf.Bytes()),
			r.es.Bulk.WithContext(ctx),
		)
		if err != nil {
			r.logger.Warn("ES bulk index failed", zap.Int("batch", i/batchSize), zap.Error(err))
			continue
		}
		res.Body.Close()
		if res.IsError() {
			r.logger.Warn("ES bulk index error", zap.Int("batch", i/batchSize), zap.String("status", res.Status()))
		}

		r.logger.Info("ES bulk index progress", zap.Int("indexed", end), zap.Int("total", len(products)))
	}

	r.logger.Info("ES index rebuilt", zap.Int("count", len(products)))
	return nil
}

