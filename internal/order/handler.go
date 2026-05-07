package order

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

// CreateOrder 用户下单
func (h *Handler) CreateOrder(c *gin.Context) {
	ctx := c.Request.Context()

	accountInfo := identity.GetAccountInfo(ctx)
	if accountInfo == nil {
		response.Write(c, errno.ErrInternalServer, nil)
		return
	}

	var body CreateOrderBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Write(c, errno.ErrInvalidParam, nil)
		return
	}

	productID, err := uuid.Parse(body.ProductID)
	if err != nil {
		response.Write(c, errno.ErrInvalidParam, nil)
		return
	}

	if err := h.svc.CreateOrder(ctx, accountInfo.AccountId, CreateOrderParam{
		ProductID:      productID,
		Quantity:       body.Quantity,
		IdempotencyKey: body.IdempotencyKey,
	}); err != nil {
		response.Write(c, err, nil)
		return
	}

	response.Write(c, nil, nil)
}

// ListOrders 用户查看自己的订单列表
func (h *Handler) ListOrders(c *gin.Context) {
	ctx := c.Request.Context()

	accountInfo := identity.GetAccountInfo(ctx)
	if accountInfo == nil {
		response.Write(c, errno.ErrInternalServer, nil)
		return
	}

	var query ListOrdersQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		response.Write(c, errno.ErrInvalidParam, nil)
		return
	}

	orders, total, err := h.svc.ListOrders(ctx, ListOrdersParam{
		UserID:   accountInfo.AccountId,
		PageNum:  query.PageNum,
		PageSize: query.PageSize,
	})
	if err != nil {
		response.Write(c, err, nil)
		return
	}

	items := make([]OrderItem, 0, len(orders))
	for _, o := range orders {
		items = append(items, *FormatOrderItem(o))
	}

	response.Write(c, nil, ListOrdersResponse{
		Orders: items,
		Total:  total,
	})
}
