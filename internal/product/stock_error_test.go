package product

import (
	"errors"
	"testing"

	"e-commerce/pkg/errno"
)

func TestClassifyStockUpdateMiss(t *testing.T) {
	cases := []struct {
		name          string
		productExists bool
		want          *errno.Errno
		wantCode      string
	}{
		{name: "商品存在但库存不足", productExists: true, want: errno.ErrProductStockInsufficient, wantCode: "A04101"},
		{name: "商品不存在或不属于当前商家", productExists: false, want: errno.ErrProductNotFound, wantCode: "A04102"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := classifyStockUpdateMiss(c.productExists)
			if got != c.want {
				t.Fatalf("classifyStockUpdateMiss(%v) = %v, want %v", c.productExists, got, c.want)
			}
			if !errors.Is(got, c.want) {
				t.Fatalf("返回错误应能被 errors.Is 匹配: %v", got)
			}
			if got.FullCode() != c.wantCode {
				t.Fatalf("错误码 = %s, want %s", got.FullCode(), c.wantCode)
			}
		})
	}
}
