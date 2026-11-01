package product

import (
	"testing"
	"time"

	"e-commerce/internal/model"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// 商品列表/详情的时间字段与订单、搜索保持一致，统一输出 RFC3339。
func TestFormatItemUsesRFC3339(t *testing.T) {
	zone := time.FixedZone("CST", 8*3600)
	createdAt := time.Date(2026, 11, 1, 9, 30, 0, 0, zone)
	p := &model.Product{
		ID:        uuid.New(),
		SellerID:  uuid.New(),
		Name:      "时间格式测试商品",
		Price:     decimal.NewFromInt(10),
		Status:    model.ProductStatusActive,
		CreatedAt: createdAt,
	}

	item := FormatItem(p, "测试店铺")
	if item.CreatedAt != createdAt.Format(time.RFC3339) {
		t.Fatalf("created_at=%q，期望=%q", item.CreatedAt, createdAt.Format(time.RFC3339))
	}
	if _, err := time.Parse(time.RFC3339, item.CreatedAt); err != nil {
		t.Fatalf("created_at=%q 不是 RFC3339: %v", item.CreatedAt, err)
	}

	detail := FormatDetail(p, "测试店铺")
	if detail.CreatedAt != item.CreatedAt {
		t.Fatalf("详情与列表 created_at 不一致：%q vs %q", detail.CreatedAt, item.CreatedAt)
	}
}
