package order

import (
	"e-commerce/internal/model"
	"time"
)

type OrderItem struct {
	CloseReason     *model.OrderCloseReason `json:"close_reason"`
	PayableAmount   float64                 `json:"payable_amount"`
	PaymentDeadline time.Time               `json:"payment_deadline"`
	PaidAt          *time.Time              `json:"paid_at"`
	ClosedAt        *time.Time              `json:"closed_at"`

	SellerID       string  `json:"seller_id"`
	BuyerID        string  `json:"buyer_id"`
	ID             string  `json:"id"`
	ProductID      string  `json:"product_id"`
	Quantity       int     `json:"quantity"`
	SnapshotTitle  string  `json:"snapshot_title"`
	SnapshotPrice  float64 `json:"snapshot_price"`
	DiscountAmount float64 `json:"discount_amount"`
	TotalAmount    float64 `json:"total_amount"`
	Status         int     `json:"status"`
	CreatedAt      string  `json:"created_at"`
}

type ListOrdersResponse struct {
	Orders []OrderItem `json:"orders"`
	Total  int64       `json:"total"`
}

type RefundDetailResponse struct {
	ID            string  `json:"id"`
	Status        string  `json:"status"`
	Amount        float64 `json:"amount"`
	FailureReason string  `json:"failure_reason,omitempty"`
	UpdatedAt     string  `json:"updated_at"`
}

type OrderDetailResponse struct {
	OrderItem
	CouponID  *string               `json:"coupon_id,omitempty"`
	UpdatedAt string                `json:"updated_at"`
	Refund    *RefundDetailResponse `json:"refund"`
}

func FormatOrderItem(o *model.Order) *OrderItem {
	return &OrderItem{
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
		CreatedAt:      o.CreatedAt.Format("2006-01-02 15:04:05"),
	}
}

func FormatOrderDetail(o *model.Order) *OrderDetailResponse {
	var couponID *string
	if o.UserCouponID != nil {
		id := o.UserCouponID.String()
		couponID = &id
	}
	return &OrderDetailResponse{
		OrderItem: *FormatOrderItem(o),
		CouponID:  couponID,
		UpdatedAt: o.UpdatedAt.Format("2006-01-02 15:04:05"),
		Refund:    nil,
	}
}
