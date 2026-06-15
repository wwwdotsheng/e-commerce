package coupon

import "github.com/google/uuid"

type CreateTemplateParam struct {
	Name          string  `json:"name" binding:"required,min=1,max=128"`
	Type          string  `json:"type" binding:"required,oneof=fixed_amount percentage"`
	DiscountValue float64 `json:"discount_value" binding:"omitempty,gte=0"`
	DiscountRate  float64 `json:"discount_rate" binding:"omitempty,gte=0,lte=1"`
	MaxDeduction  float64 `json:"max_deduction" binding:"omitempty,gte=0"`
	MinAmount     float64 `json:"min_amount" binding:"omitempty,gte=0"`
	TotalQty      int     `json:"total_qty" binding:"required,gte=1"`
	PerUserLimit  int     `json:"per_user_limit" binding:"omitempty,gte=1"`
	StartTime     string  `json:"start_time" binding:"required"`
	EndTime       string  `json:"end_time" binding:"required"`
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
	OrderAmount  float64
}
