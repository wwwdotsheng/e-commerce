package order

import (
	"github.com/google/uuid"
)

type CreateOrderParam struct {
	ProductID      uuid.UUID
	Quantity       int
	UserCouponID   uuid.UUID
	IdempotencyKey string
}

type ListOrdersParam struct {
	UserID   uuid.UUID
	PageNum  int
	PageSize int
}