package order

import (
	"e-commerce/internal/model"
	"time"

	"github.com/shopspring/decimal"
)

type Item struct {
	CloseReason     *model.OrderCloseReason `json:"close_reason"`
	PayableAmount   decimal.Decimal         `json:"payable_amount"`
	PaymentDeadline time.Time               `json:"payment_deadline"`
	PaidAt          *time.Time              `json:"paid_at"`
	ClosedAt        *time.Time              `json:"closed_at"`

	SellerID       string          `json:"seller_id"`
	BuyerID        string          `json:"buyer_id"`
	ID             string          `json:"id"`
	ProductID      string          `json:"product_id"`
	Quantity       int             `json:"quantity"`
	SnapshotTitle  string          `json:"snapshot_title"`
	SnapshotPrice  decimal.Decimal `json:"snapshot_price"`
	DiscountAmount decimal.Decimal `json:"discount_amount"`
	TotalAmount    decimal.Decimal `json:"total_amount"`
	Status         int             `json:"status"`
	CreatedAt      string          `json:"created_at"`
}

type ListOrdersResponse struct {
	Orders []Item `json:"orders"`
	Total  int64  `json:"total"`
}

type RefundDetailResponse struct {
	ID            string          `json:"id"`
	Status        string          `json:"status"`
	Amount        decimal.Decimal `json:"amount"`
	FailureReason string          `json:"failure_reason,omitempty"`
	RefundedAt    *string         `json:"refunded_at,omitempty"`
	UpdatedAt     string          `json:"updated_at"`
}

type DetailResponse struct {
	Item
	CouponID  *string               `json:"coupon_id,omitempty"`
	UpdatedAt string                `json:"updated_at"`
	Refund    *RefundDetailResponse `json:"refund"`
	Timeline  []TimelineEvent       `json:"timeline"`
}

type TimelineEvent struct {
	Type     string `json:"type"`
	At       string `json:"at"`
	RefundID string `json:"refund_id,omitempty"`
}

func FormatItem(o *model.Order) *Item {
	return &Item{
		CloseReason:   o.CloseReason,
		PayableAmount: o.PayableAmount, PaymentDeadline: o.PaymentDeadline, PaidAt: o.PaidAt, ClosedAt: o.ClosedAt,
		SellerID: o.SellerID.String(), BuyerID: o.UserID.String(),
		ID:             o.ID.String(),
		ProductID:      o.ProductId.String(),
		Quantity:       o.Quantity,
		SnapshotTitle:  o.SnapshotTitle,
		SnapshotPrice:  o.SnapshotPrice,
		DiscountAmount: o.DiscountAmount,
		TotalAmount:    o.PayableAmount,
		Status:         int(o.Status),
		CreatedAt:      o.CreatedAt.Format(time.RFC3339),
	}
}

func FormatRefund(r *model.Refund) *RefundDetailResponse {
	var refundedAt *string
	if r.RefundedAt != nil {
		s := r.RefundedAt.Format(time.RFC3339)
		refundedAt = &s
	}
	return &RefundDetailResponse{
		ID:            r.ID.String(),
		Status:        string(r.Status),
		Amount:        r.Amount,
		FailureReason: r.FailureReason,
		RefundedAt:    refundedAt,
		UpdatedAt:     r.UpdatedAt.Format(time.RFC3339),
	}
}

func FormatOrderDetail(o *model.Order) *DetailResponse {
	var couponID *string
	if o.UserCouponID != nil {
		id := o.UserCouponID.String()
		couponID = &id
	}
	return &DetailResponse{
		Item:      *FormatItem(o),
		CouponID:  couponID,
		UpdatedAt: o.UpdatedAt.Format(time.RFC3339),
	}
}

func BuildTimeline(o *model.Order, refund *model.Refund) []TimelineEvent {
	events := []TimelineEvent{{Type: "order_created", At: o.CreatedAt.Format(time.RFC3339)}}
	if o.PaidAt != nil {
		events = append(events, TimelineEvent{Type: "payment_succeeded", At: o.PaidAt.Format(time.RFC3339)})
	}
	if o.ClosedAt != nil {
		typ := "order_closed"
		if o.CloseReason != nil && *o.CloseReason == model.OrderCloseReasonPaymentTimeout {
			typ = "payment_timeout"
		}
		events = append(events, TimelineEvent{Type: typ, At: o.ClosedAt.Format(time.RFC3339)})
	}
	if refund != nil {
		events = append(events, TimelineEvent{
			Type: "refund_requested", At: refund.CreatedAt.Format(time.RFC3339), RefundID: refund.ID.String(),
		})
		switch {
		case refund.RefundedAt != nil:
			events = append(events, TimelineEvent{
				Type: "refund_succeeded", At: refund.RefundedAt.Format(time.RFC3339), RefundID: refund.ID.String(),
			})
		case refund.Status == model.RefundStatusFailed:
			events = append(events, TimelineEvent{
				Type: "refund_failed", At: refund.UpdatedAt.Format(time.RFC3339), RefundID: refund.ID.String(),
			})
		default:
			events = append(events, TimelineEvent{
				Type: "refund_processing", At: refund.UpdatedAt.Format(time.RFC3339), RefundID: refund.ID.String(),
			})
		}
	}
	return events
}
