package order

import (
	"e-commerce/internal/model"
	"encoding/json"
	"testing"
	"time"
)

func TestPayableAmount(t *testing.T) {
	cases := []struct {
		name           string
		price          float64
		qty            int
		discount, want float64
	}{
		{"无优惠", 59.99, 2, 0, 119.98},
		{"正常优惠", 59.99, 2, 10, 109.98},
		{"优惠超过商品总额时应付为零", 10, 1, 20, 0},
		{"优惠等于商品总额时应付为零", 10, 1, 10, 0},
		{"金额保留两位小数", 10, 1, 1.236, 8.76},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := calculatePayableAmount(c.price, c.qty, c.discount); got != c.want {
				t.Fatalf("应付金额=%v，预期=%v", got, c.want)
			}
		})
	}
}
func TestOrderPaymentResponse(t *testing.T) {
	t.Run("响应使用已保存金额而非重新计算", func(t *testing.T) {
		deadline := time.Date(2026, 10, 31, 12, 30, 0, 0, time.UTC)
		o := &model.Order{SnapshotPrice: 999, Quantity: 2, DiscountAmount: 0, PayableAmount: 109.98, PaymentDeadline: deadline}
		got := FormatOrderItem(o)
		if got.PayableAmount != 109.98 || got.TotalAmount != 109.98 || !got.PaymentDeadline.Equal(deadline) {
			t.Fatalf("订单金额快照不一致: %+v", got)
		}
		raw, err := json.Marshal(got)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]interface{}
		if err = json.Unmarshal(raw, &fields); err != nil {
			t.Fatal(err)
		}
		if fields["paid_at"] != nil || fields["closed_at"] != nil {
			t.Fatal("未支付、未关闭时必须返回 null")
		}
	})
	t.Run("已支付和已关闭时间原样返回", func(t *testing.T) {
		now := time.Now()
		got := FormatOrderItem(&model.Order{PaidAt: &now, ClosedAt: &now})
		if got.PaidAt == nil || got.ClosedAt == nil || !got.PaidAt.Equal(now) || !got.ClosedAt.Equal(now) {
			t.Fatal("状态时间缺失")
		}
	})
}
