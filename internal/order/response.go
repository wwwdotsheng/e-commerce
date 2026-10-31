package order

import (
	"e-commerce/internal/model"
	"time"
)

type OrderItem struct {
	PayableAmount   float64    `json:"payable_amount"`
	PaymentDeadline time.Time  `json:"payment_deadline"`
	PaidAt          *time.Time `json:"paid_at"`
	ClosedAt        *time.Time `json:"closed_at"`

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

func FormatOrderItem(o *model.Order) *OrderItem {
	return &OrderItem{
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
