package order

import (
	"e-commerce/internal/app/identity"
	"e-commerce/internal/pkg/response"
	"e-commerce/internal/product"
	"e-commerce/pkg/errno"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type Handler struct {
	svc        *Service
	productSvc *product.Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// CreateOrder 用户下单商品, 提前减少stock并且增加frozen_stock
func (h *Handler) CreateOrder(c *gin.Context) {
	ctx := c.Request.Context()

	accountInfo := identity.GetAccountInfo(ctx)
	if accountInfo == nil {
		response.Write(c, errno.ErrInternalServer, nil)
		return
	}

	// 1. 要获取商品信息 TODO 这里的GetProduct是否有性能问题？
	err := h.svc.CreateOrder(ctx, accountInfo.AccountId, InputCreateOrder{
		productID:      uuid.UUID{},
		quantity:       0,
		idempotencyKey: "",
	})
	if err != nil {
		response.Write(c, err, nil)
		return
	}

	response.Write(c, nil, nil)
}
