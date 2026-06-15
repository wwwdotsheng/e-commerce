package coupon

import (
	"e-commerce/internal/app/identity"
	"e-commerce/internal/pkg/response"
	"e-commerce/pkg/errno"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// CreateTemplate 运营创建优惠券模板
func (h *Handler) CreateTemplate(c *gin.Context) {
	ctx := c.Request.Context()

	accountInfo := identity.GetAccountInfo(ctx)
	if accountInfo == nil {
		response.Write(c, errno.ErrGetAccountInfo, nil)
		return
	}

	var body CreateTemplateParam
	if err := c.ShouldBindJSON(&body); err != nil {
		response.WriteInvalidParam(c, err)
		return
	}

	body.Publisher = accountInfo.AccountId

	t, err := h.svc.CreateTemplate(ctx, body)
	if err != nil {
		response.Write(c, err, nil)
		return
	}

	response.Write(c, nil, formatTemplate(t))
}

// GrantCoupon 运营给指定用户发券
func (h *Handler) GrantCoupon(c *gin.Context) {
	ctx := c.Request.Context()

	var body struct {
		TemplateID string `json:"template_id" binding:"required"`
		UserID     string `json:"user_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		response.WriteInvalidParam(c, err)
		return
	}

	templateID, err := uuid.Parse(body.TemplateID)
	if err != nil {
		response.WriteInvalidParam(c, err)
		return
	}
	userID, err := uuid.Parse(body.UserID)
	if err != nil {
		response.WriteInvalidParam(c, err)
		return
	}

	uc, err := h.svc.GrantCoupon(ctx, GrantCouponParam{
		TemplateID: templateID,
		UserID:     userID,
	})
	if err != nil {
		response.Write(c, err, nil)
		return
	}

	response.Write(c, nil, formatUserCoupon(uc))
}

// ListUserCoupons 用户查看自己的券
func (h *Handler) ListUserCoupons(c *gin.Context) {
	ctx := c.Request.Context()

	accountInfo := identity.GetAccountInfo(ctx)
	if accountInfo == nil {
		response.Write(c, errno.ErrGetAccountInfo, nil)
		return
	}

	var query ListUserCouponsParam
	if err := c.ShouldBindQuery(&query); err != nil {
		response.WriteInvalidParam(c, err)
		return
	}

	query.UserID = accountInfo.AccountId

	coupons, total, err := h.svc.ListUserCoupons(ctx, query)
	if err != nil {
		response.Write(c, err, nil)
		return
	}

	items := make([]UserCouponItem, 0, len(coupons))
	for _, uc := range coupons {
		items = append(items, *formatUserCoupon(uc))
	}

	response.Write(c, nil, ListUserCouponsResponse{
		Coupons: items,
		Total:   total,
	})
}
