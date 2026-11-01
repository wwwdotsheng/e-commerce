package order

import (
	"testing"
	"time"

	"e-commerce/internal/model"

	"github.com/google/uuid"
)

// 订单响应里的时间字段统一走 RFC3339，与 payment_deadline / paid_at / closed_at
// 由 encoding/json 序列化的格式一致，避免同一响应里出现两种时间格式。
func TestFormatOrderTimesUseRFC3339(t *testing.T) {
	zone := time.FixedZone("CST", 8*3600)
	createdAt := time.Date(2026, 11, 1, 10, 30, 0, 0, zone)
	updatedAt := time.Date(2026, 11, 1, 10, 31, 0, 0, zone)

	o := &model.Order{
		ID:            uuid.New(),
		UserID:        uuid.New(),
		SellerID:      uuid.New(),
		ProductId:     uuid.New(),
		Quantity:      1,
		SnapshotTitle: "format test",
		SnapshotPrice: 10,
		PayableAmount: 10,
		Status:        model.OrderStatusProcessing,
		CreatedAt:     createdAt,
		UpdatedAt:     updatedAt,
	}

	item := FormatOrderItem(o)
	if _, err := time.Parse(time.RFC3339, item.CreatedAt); err != nil {
		t.Fatalf("created_at=%q 不是 RFC3339: %v", item.CreatedAt, err)
	}
	if want := createdAt.Format(time.RFC3339); item.CreatedAt != want {
		t.Fatalf("created_at=%q，期望=%q", item.CreatedAt, want)
	}

	detail := FormatOrderDetail(o)
	if _, err := time.Parse(time.RFC3339, detail.UpdatedAt); err != nil {
		t.Fatalf("updated_at=%q 不是 RFC3339: %v", detail.UpdatedAt, err)
	}
	if want := updatedAt.Format(time.RFC3339); detail.UpdatedAt != want {
		t.Fatalf("updated_at=%q，期望=%q", detail.UpdatedAt, want)
	}
}
