package order

import (
	"context"
	"e-commerce/internal/app/identity"
	"e-commerce/internal/model"
	"e-commerce/internal/pkg/response"
	"e-commerce/pkg/errno"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type Handler struct {
	svc orderService
}

type orderService interface {
	CreateOrder(context.Context, uuid.UUID, CreateOrderParam) error
	ListOrders(context.Context, ListOrdersParam) ([]*model.Order, int64, error)
	GetOrderByID(context.Context, uuid.UUID, uuid.UUID) (*model.Order, error)
	PayOrder(context.Context, uuid.UUID, string, uuid.UUID) (*model.Order, error)
	ApplyRefund(context.Context, uuid.UUID, uuid.UUID) (*model.Refund, error)
	GetOrderWithRefund(context.Context, uuid.UUID, uuid.UUID) (*model.Order, *model.Refund, error)
}

func NewHandler(svc orderService) *Handler {
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
		response.WriteInvalidParam(c, err)
		return
	}

	productID, err := uuid.Parse(body.ProductID)
	if err != nil {
		response.WriteInvalidParam(c, err)
		return
	}

	var couponID uuid.UUID
	if body.CouponID != "" {
		couponID, err = uuid.Parse(body.CouponID)
		if err != nil {
			response.WriteInvalidParam(c, err)
			return
		}
	}

	if err := h.svc.CreateOrder(ctx, accountInfo.AccountId, CreateOrderParam{
		ProductID:      productID,
		Quantity:       body.Quantity,
		UserCouponID:   couponID,
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
		response.WriteInvalidParam(c, err)
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

	items := make([]Item, 0, len(orders))
	for _, o := range orders {
		items = append(items, *FormatItem(o))
	}

	response.Write(c, nil, ListOrdersResponse{
		Orders: items,
		Total:  total,
	})
}

// GetOrderByID 获取当前用户的订单详情
func (h *Handler) GetOrderByID(c *gin.Context) {
	ctx := c.Request.Context()
	accountInfo := identity.GetAccountInfo(ctx)
	if accountInfo == nil {
		response.Write(c, errno.ErrGetAccountInfo, nil)
		return
	}

	var uri UriWithOrderID
	if err := c.ShouldBindUri(&uri); err != nil {
		response.WriteInvalidParam(c, err)
		return
	}
	orderID, err := uuid.Parse(uri.ID)
	if err != nil {
		response.WriteInvalidParam(c, err)
		return
	}

	o, refund, err := h.svc.GetOrderWithRefund(ctx, accountInfo.AccountId, orderID)
	if err != nil {
		response.Write(c, err, nil)
		return
	}
	detail := FormatOrderDetail(o)
	if refund != nil {
		detail.Refund = FormatRefund(refund)
	}
	detail.Timeline = BuildTimeline(o, refund)
	response.Write(c, nil, detail)
}

// PayOrder 使用账户余额支付订单
func (h *Handler) PayOrder(c *gin.Context) {
	ctx := c.Request.Context()
	accountInfo := identity.GetAccountInfo(ctx)
	if accountInfo == nil {
		response.Write(c, errno.ErrGetAccountInfo, nil)
		return
	}

	var uri UriWithOrderID
	if err := c.ShouldBindUri(&uri); err != nil {
		response.WriteInvalidParam(c, err)
		return
	}
	orderID, err := uuid.Parse(uri.ID)
	if err != nil {
		response.WriteInvalidParam(c, err)
		return
	}

	o, err := h.svc.PayOrder(ctx, accountInfo.AccountId, accountInfo.SessionID, orderID)
	if err != nil {
		response.Write(c, err, nil)
		return
	}
	response.Write(c, nil, FormatOrderDetail(o))
}

// ApplyRefund 受理全额退款：只落 refunds + refund_jobs，立即返回 pending。
func (h *Handler) ApplyRefund(c *gin.Context) {
	ctx := c.Request.Context()
	accountInfo := identity.GetAccountInfo(ctx)
	if accountInfo == nil {
		response.Write(c, errno.ErrGetAccountInfo, nil)
		return
	}
	var uri UriWithOrderID
	if err := c.ShouldBindUri(&uri); err != nil {
		response.WriteInvalidParam(c, err)
		return
	}
	orderID, err := uuid.Parse(uri.ID)
	if err != nil {
		response.WriteInvalidParam(c, err)
		return
	}
	refund, err := h.svc.ApplyRefund(ctx, accountInfo.AccountId, orderID)
	if err != nil {
		response.Write(c, err, nil)
		return
	}
	response.Write(c, nil, FormatRefund(refund))
}
