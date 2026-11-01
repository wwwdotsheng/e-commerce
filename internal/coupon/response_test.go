package coupon

import (
	"testing"
	"time"

	"e-commerce/internal/model"

	"github.com/google/uuid"
)

// 优惠券模板与用户券的时间字段统一输出 RFC3339，和订单/商品保持一致。
func TestFormatTemplateUsesRFC3339(t *testing.T) {
	zone := time.FixedZone("CST", 8*3600)
	start := time.Date(2026, 11, 1, 0, 0, 0, 0, zone)
	end := time.Date(2026, 12, 1, 23, 59, 59, 0, zone)
	created := time.Date(2026, 10, 31, 18, 0, 0, 0, zone)

	tpl := &model.CouponTemplate{
		ID:        uuid.New(),
		Name:      "时间格式测试券",
		Type:      model.CouponTypeFixed,
		StartTime: start,
		EndTime:   end,
		CreatedAt: created,
	}
	item := formatTemplate(tpl)

	for name, got := range map[string]string{
		"start_time": item.StartTime,
		"end_time":   item.EndTime,
		"created_at": item.CreatedAt,
	} {
		if _, err := time.Parse(time.RFC3339, got); err != nil {
			t.Fatalf("%s=%q 不是 RFC3339: %v", name, got, err)
		}
	}
	if item.StartTime != start.Format(time.RFC3339) {
		t.Fatalf("start_time=%q，期望=%q", item.StartTime, start.Format(time.RFC3339))
	}
}

func TestFormatUserCouponUsesRFC3339(t *testing.T) {
	zone := time.FixedZone("CST", 8*3600)
	expire := time.Date(2026, 12, 1, 23, 59, 59, 0, zone)
	created := time.Date(2026, 11, 1, 10, 0, 0, 0, zone)

	uc := &model.UserCoupon{
		ID:         uuid.New(),
		TemplateID: uuid.New(),
		Status:     model.UserCouponStatusUnused,
		ExpireTime: expire,
		CreatedAt:  created,
	}
	item := formatUserCoupon(uc)

	if item.ExpireTime != expire.Format(time.RFC3339) {
		t.Fatalf("expire_time=%q，期望=%q", item.ExpireTime, expire.Format(time.RFC3339))
	}
	if item.CreatedAt != created.Format(time.RFC3339) {
		t.Fatalf("created_at=%q，期望=%q", item.CreatedAt, created.Format(time.RFC3339))
	}
	for name, got := range map[string]string{"expire_time": item.ExpireTime, "created_at": item.CreatedAt} {
		if _, err := time.Parse(time.RFC3339, got); err != nil {
			t.Fatalf("%s=%q 不是 RFC3339: %v", name, got, err)
		}
	}
}
