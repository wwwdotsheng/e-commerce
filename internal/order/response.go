package order

import (
	"e-commerce/internal/model"
	"time"

	"github.com/shopspring/decimal"
)

type OrderItem struct {
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
	Orders []OrderItem `json:"orders"`
	Total  int64       `json:"total"`
}

type OrderDetailResponse struct {
	OrderItem
	CouponID  *string `json:"coupon_id,omitempty"`
	UpdatedAt string  `json:"updated_at"`
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
		CreatedAt:      o.CreatedAt.Format(time.RFC3339),
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
		UpdatedAt: o.UpdatedAt.Format(time.RFC3339),
	}
}
