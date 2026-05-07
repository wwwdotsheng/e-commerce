package order

import (
	"e-commerce/internal/model"
)

type OrderItem struct {
	ID            string  `json:"id"`
	ProductID     string  `json:"product_id"`
	Quantity      int     `json:"quantity"`
	SnapshotTitle string  `json:"snapshot_title"`
	SnapshotPrice float64 `json:"snapshot_price"`
	Status        int     `json:"status"`
	CreatedAt     string  `json:"created_at"`
}

type ListOrdersResponse struct {
	Orders []OrderItem `json:"orders"`
	Total  int64       `json:"total"`
}

func FormatOrderItem(o *model.Order) *OrderItem {
	return &OrderItem{
		ID:            o.ID.String(),
		ProductID:     o.ProductId.String(),
		Quantity:      o.Quantity,
		SnapshotTitle: o.SnapshotTitle,
		SnapshotPrice: o.SnapshotPrice,
		Status:        int(o.Status),
		CreatedAt:     o.CreatedAt.Format("2006-01-02 15:04:05"),
	}
}