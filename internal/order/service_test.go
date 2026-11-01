package order

import (
	"errors"
	"testing"

	"e-commerce/internal/wallet"
	"e-commerce/pkg/errno"
)

func TestClassifyPaymentError(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		wantCode OrderPaymentErrCode
		wantErr  error
	}{
		{"余额不足", errno.ErrWalletBalanceInsufficient, orderPaymentErrInsufficientBalance, errno.ErrWalletBalanceInsufficient},
		{"订单超时", errno.ErrOrderPaymentExpired, orderPaymentErrExpired, errno.ErrOrderPaymentExpired},
		{"状态不可支付", errno.ErrOrderPaymentStatusInvalid, orderPaymentErrStatusInvalid, errno.ErrOrderPaymentStatusInvalid},
		{"订单不存在", errno.ErrNotFoundRecord, orderPaymentErrNotFound, errno.ErrNotFoundRecord},
		{"资金流水冲突", wallet.ErrWalletLogAlreadyExists, orderPaymentErrLedgerConflict, errno.ErrOrderPaymentConflict},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			code, got := classifyPaymentError(c.err)
			if code != c.wantCode {
				t.Fatalf("指标分类=%q，期望=%q", code, c.wantCode)
			}
			if !errors.Is(got, c.wantErr) {
				t.Fatalf("返回错误=%v，期望=%v", got, c.wantErr)
			}
		})
	}

	t.Run("未识别错误保持原样", func(t *testing.T) {
		raw := errors.New("boom")
		code, got := classifyPaymentError(raw)
		if code != orderPaymentErrInternal {
			t.Fatalf("指标分类=%q，期望=%q", code, orderPaymentErrInternal)
		}
		if got != raw {
			t.Fatalf("返回错误=%v，期望原样返回 %v", got, raw)
		}
	})
}
