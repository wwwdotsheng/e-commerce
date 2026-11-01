package wallet

import (
	"context"
	"errors"
	"strings"
	"testing"

	"e-commerce/pkg/errno"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
)

func TestValidateDepositIdempotencyKey(t *testing.T) {
	cases := []struct {
		name    string
		key     string
		wantErr bool
	}{
		{"合法充值键", "deposit-2026-11-01-0001", false},
		{"空键", "", true},
		{"恰好等于列宽", strings.Repeat("x", walletLogIdempotencyKeyMaxLen), false},
		{"超过列宽", strings.Repeat("x", walletLogIdempotencyKeyMaxLen+1), true},
		{"占用订单保留前缀", orderLogKeyPrefix + uuid.NewString() + ":payment", true},
		{"允许的字符集", "deposit_2026-11-01.1:abc", false},
		{"含空格", "deposit key", true},
		{"含中文字符", "充值-2026", true},
		{"含斜杠", "deposit/2026", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateDepositIdempotencyKey(c.key)
			if c.wantErr {
				if !errors.Is(err, errno.ErrWalletInvalidIdempotencyKey) {
					t.Fatalf("期望返回幂等键非法，实际=%v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("期望通过校验，实际=%v", err)
			}
		})
	}
}

// 充值键占用订单流水键时必须在校验阶段失败，不能进入数据库写入。
// 这里用 nil 仓储证明失败发生在触达数据库之前。
func TestDepositRejectsReservedKeyBeforeWriting(t *testing.T) {
	metrics, err := NewMetrics(otel.Meter("wallet-unit-test"))
	if err != nil {
		t.Fatalf("初始化指标失败: %v", err)
	}
	svc := NewService(nil, metrics)

	err = svc.Deposit(context.Background(), uuid.New(), "test-session", &DepositInput{
		Amount:         10,
		IdempotencyKey: orderPaymentLogKey(uuid.New()),
	})
	if !errors.Is(err, errno.ErrWalletInvalidIdempotencyKey) {
		t.Fatalf("期望返回幂等键非法，实际=%v", err)
	}
}
