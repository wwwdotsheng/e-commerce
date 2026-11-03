package order

import (
	"context"
	"sync"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type refundOutcome string

const (
	refundOutcomeSucceeded refundOutcome = "succeeded"
	refundOutcomeFailed    refundOutcome = "failed"
	refundOutcomeUnknown   refundOutcome = "unknown"
	refundOutcomeNotFound  refundOutcome = "not_found"
)

type refundChannelResult struct {
	Outcome       refundOutcome
	FailureReason string
}

type refundChannel interface {
	Create(ctx context.Context, refundID uuid.UUID, amount decimal.Decimal) (refundChannelResult, error)
	Query(ctx context.Context, refundID uuid.UUID) (refundChannelResult, error)
}

type mockRefundChannel struct {
	mu       sync.Mutex
	outcomes map[uuid.UUID]refundOutcome
}

func newMockRefundChannel() refundChannel {
	return &mockRefundChannel{outcomes: make(map[uuid.UUID]refundOutcome)}
}

func (m *mockRefundChannel) Create(_ context.Context, refundID uuid.UUID, amount decimal.Decimal) (refundChannelResult, error) {
	cents := amount.Mul(decimal.NewFromInt(100)).IntPart()
	outcome := refundOutcomeSucceeded
	switch cents % 3 {
	case 1:
		outcome = refundOutcomeFailed
	case 2:
		outcome = refundOutcomeUnknown
	}
	m.mu.Lock()
	m.outcomes[refundID] = outcome
	m.mu.Unlock()
	return refundChannelResult{Outcome: outcome, FailureReason: mockFailureReason(outcome)}, nil
}

func (m *mockRefundChannel) Query(_ context.Context, refundID uuid.UUID) (refundChannelResult, error) {
	m.mu.Lock()
	outcome, ok := m.outcomes[refundID]
	m.mu.Unlock()
	if !ok {
		return refundChannelResult{Outcome: refundOutcomeNotFound}, nil
	}
	if outcome == refundOutcomeUnknown {
		return refundChannelResult{Outcome: refundOutcomeSucceeded}, nil
	}
	return refundChannelResult{Outcome: outcome, FailureReason: mockFailureReason(outcome)}, nil
}

func mockFailureReason(outcome refundOutcome) string {
	if outcome == refundOutcomeFailed {
		return "mock channel declined"
	}
	return ""
}
