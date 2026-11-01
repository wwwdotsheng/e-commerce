package coupon

import (
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type CreateTemplateParam struct {
	Name          string          `json:"name" binding:"required,min=1,max=128"`
	Type          string          `json:"type" binding:"required,oneof=fixed_amount percentage"`
	DiscountValue decimal.Decimal `json:"discount_value"`
	DiscountRate  decimal.Decimal `json:"discount_rate"`
	MaxDeduction  decimal.Decimal `json:"max_deduction"`
	MinAmount     decimal.Decimal `json:"min_amount"`
	TotalQty      int             `json:"total_qty" binding:"required,gte=1"`
	PerUserLimit  int             `json:"per_user_limit" binding:"omitempty,gte=1"`
	StartTime     string          `json:"start_time" binding:"required"`
	EndTime       string          `json:"end_time" binding:"required"`
	Publisher     uuid.UUID
}

type GrantCouponParam struct {
	TemplateID uuid.UUID `json:"template_id" binding:"required"`
	UserID     uuid.UUID
}

type ListUserCouponsParam struct {
	UserID   uuid.UUID
	PageNum  int `form:"page_num" binding:"omitempty,gte=1"`
	PageSize int `form:"page_size" binding:"omitempty,gte=1,lte=50"`
}

type UseCouponParam struct {
	UserCouponID uuid.UUID
	OrderID      uuid.UUID
	OrderAmount  decimal.Decimal
}
