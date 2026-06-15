package coupon

import (
	"e-commerce/internal/model"
	"time"
)

type TemplateItem struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	Type          string  `json:"type"`
	DiscountValue float64 `json:"discount_value"`
	DiscountRate  float64 `json:"discount_rate"`
	MaxDeduction  float64 `json:"max_deduction"`
	MinAmount     float64 `json:"min_amount"`
	TotalQty      int     `json:"total_qty"`
	RemainingQty  int     `json:"remaining_qty"`
	PerUserLimit  int     `json:"per_user_limit"`
	StartTime     string  `json:"start_time"`
	EndTime       string  `json:"end_time"`
	Status        string  `json:"status"`
	CreatedAt     string  `json:"created_at"`
}

type UserCouponItem struct {
	ID             string        `json:"id"`
	TemplateID     string        `json:"template_id"`
	TemplateName   string        `json:"template_name"`
	Type           string        `json:"type"`
	DiscountValue  float64       `json:"discount_value"`
	DiscountRate   float64       `json:"discount_rate"`
	MaxDeduction   float64       `json:"max_deduction"`
	MinAmount      float64       `json:"min_amount"`
	Status         string        `json:"status"`
	ExpireTime     string        `json:"expire_time"`
	CreatedAt      string        `json:"created_at"`
}

type ListUserCouponsResponse struct {
	Coupons []UserCouponItem `json:"coupons"`
	Total   int64            `json:"total"`
}

func formatTemplate(t *model.CouponTemplate) *TemplateItem {
	return &TemplateItem{
		ID:            t.ID.String(),
		Name:          t.Name,
		Type:          string(t.Type),
		DiscountValue: t.DiscountValue,
		DiscountRate:  t.DiscountRate,
		MaxDeduction:  t.MaxDeduction,
		MinAmount:     t.MinAmount,
		TotalQty:      t.TotalQty,
		RemainingQty:  t.RemainingQty,
		PerUserLimit:  t.PerUserLimit,
		StartTime:     t.StartTime.Format(time.DateTime),
		EndTime:       t.EndTime.Format(time.DateTime),
		Status:        string(t.Status),
		CreatedAt:     t.CreatedAt.Format(time.DateTime),
	}
}

func formatUserCoupon(uc *model.UserCoupon) *UserCouponItem {
	item := &UserCouponItem{
		ID:         uc.ID.String(),
		TemplateID: uc.TemplateID.String(),
		Status:     string(uc.Status),
		ExpireTime: uc.ExpireTime.Format(time.DateTime),
		CreatedAt:  uc.CreatedAt.Format(time.DateTime),
	}
	if uc.Template != nil {
		item.TemplateName = uc.Template.Name
		item.Type = string(uc.Template.Type)
		item.DiscountValue = uc.Template.DiscountValue
		item.DiscountRate = uc.Template.DiscountRate
		item.MaxDeduction = uc.Template.MaxDeduction
		item.MinAmount = uc.Template.MinAmount
	}
	return item
}
