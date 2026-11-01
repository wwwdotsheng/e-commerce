package order

import (
	"context"

	"e-commerce/internal/model"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// refundChannel 是支付渠道的退款适配器。一期使用本地模拟实现，不接真实渠道。
type refundChannel interface {
	// Create 发起退款；可能返回 succeeded、failed 或 confirming（结果不明）。
	Create(ctx context.Context, refundID uuid.UUID, amount decimal.Decimal) (refundChannelResult, error)
	// Query 按同一退款标识查询结果，用于 confirming 状态恢复。
	Query(ctx context.Context, refundID uuid.UUID) (refundChannelResult, error)
}

type refundChannelResult struct {
	Status        model.RefundStatus
	FailureReason string
}

// mockRefundChannel 模拟渠道，行为由退款金额（分）确定，保证同一订单结果可复现：
//
//	分 % 3 == 0 -> 成功
//	分 % 3 == 1 -> 失败
//	分 % 3 == 2 -> 结果不明（confirming），查询时返回成功
//
// 这样集成测试可以用商品价格构造三种结果，不需要依赖真实渠道或随机数。
type mockRefundChannel struct{}

func newMockRefundChannel() refundChannel { return mockRefundChannel{} }

func (mockRefundChannel) Create(_ context.Context, _ uuid.UUID, amount decimal.Decimal) (refundChannelResult, error) {
	return mockOutcome(amount), nil
}

func (mockRefundChannel) Query(_ context.Context, _ uuid.UUID) (refundChannelResult, error) {
	// 结果不明是渠道超时；后续查询给出终态成功，模拟“确认后成功”。
	return refundChannelResult{Status: model.RefundStatusSucceeded}, nil
}

func mockOutcome(amount decimal.Decimal) refundChannelResult {
	cents := amount.Mul(decimal.NewFromInt(100)).IntPart()
	switch cents % 3 {
	case 0:
		return refundChannelResult{Status: model.RefundStatusSucceeded}
	case 1:
		return refundChannelResult{Status: model.RefundStatusFailed, FailureReason: "mock channel declined"}
	default:
		return refundChannelResult{Status: model.RefundStatusConfirming}
	}
}
