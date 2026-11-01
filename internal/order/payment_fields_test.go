package order

import (
	"e-commerce/internal/model"
	"encoding/json"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func TestPayableAmount(t *testing.T) {
	cases := []struct {
		name        string
		price, want decimal.Decimal
		discount    decimal.Decimal
		qty         int
	}{
		{"无优惠", decimal.RequireFromString("59.99"), decimal.RequireFromString("119.98"), decimal.Zero, 2},
		{"正常优惠", decimal.RequireFromString("59.99"), decimal.RequireFromString("109.98"), decimal.RequireFromString("10"), 2},
		{"优惠超过商品总额时应付为零", decimal.RequireFromString("10"), decimal.Zero, decimal.RequireFromString("20"), 1},
		{"优惠等于商品总额时应付为零", decimal.RequireFromString("10"), decimal.Zero, decimal.RequireFromString("10"), 1},
		{"金额保留两位小数", decimal.RequireFromString("10"), decimal.RequireFromString("8.76"), decimal.RequireFromString("1.236"), 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := calculatePayableAmount(c.price, c.qty, c.discount); !got.Equal(c.want) {
				t.Fatalf("应付金额=%s，预期=%s", got, c.want)
			}
		})
	}
}
func TestOrderPaymentResponse(t *testing.T) {
	t.Run("响应使用已保存金额而非重新计算", func(t *testing.T) {
		deadline := time.Date(2026, 10, 31, 12, 30, 0, 0, time.UTC)
		o := &model.Order{
			SnapshotPrice:   decimal.RequireFromString("999"),
			Quantity:        2,
			DiscountAmount:  decimal.Zero,
			PayableAmount:   decimal.RequireFromString("109.98"),
			PaymentDeadline: deadline,
		}
		got := FormatOrderItem(o)
		want := decimal.RequireFromString("109.98")
		if !got.PayableAmount.Equal(want) || !got.TotalAmount.Equal(want) || !got.PaymentDeadline.Equal(deadline) {
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
		if fields["paid_at"] != nil || fields["closed_at"] != nil || fields["close_reason"] != nil {
			t.Fatal("未支付、未关闭时必须返回 null")
		}
	})
	t.Run("已支付和已关闭时间原样返回", func(t *testing.T) {
		now := time.Now()
		reason := model.OrderCloseReasonPaymentTimeout
		got := FormatOrderItem(&model.Order{PaidAt: &now, ClosedAt: &now, CloseReason: &reason})
		if got.CloseReason == nil || *got.CloseReason != model.OrderCloseReasonPaymentTimeout {
			t.Fatal("关闭原因缺失")
		}
		if got.PaidAt == nil || got.ClosedAt == nil || !got.PaidAt.Equal(now) || !got.ClosedAt.Equal(now) {
			t.Fatal("状态时间缺失")
		}
	})
}
