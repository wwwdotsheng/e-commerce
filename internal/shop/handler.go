package shop

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

func (h *Handler) CreateShop(c *gin.Context) {
	ctx := c.Request.Context()
	account := identity.GetAccountInfo(ctx)
	if account == nil {
		response.Write(c, errno.ErrGetAccountInfo, nil)
		return
	}

	var body CreateShopBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.WriteInvalidParam(c, err)
		return
	}

	shop, err := h.svc.Create(ctx, account.AccountId, body.Name, body.Description)
	if err != nil {
		response.Write(c, err, nil)
		return
	}
	response.Write(c, nil, FormatItem(shop))
}

func (h *Handler) GetShop(c *gin.Context) {
	ctx := c.Request.Context()
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.WriteInvalidParam(c, err)
		return
	}

	shop, err := h.svc.Get(ctx, id)
	if err != nil {
		response.Write(c, err, nil)
		return
	}
	response.Write(c, nil, FormatItem(shop))
}

func (h *Handler) ListShops(c *gin.Context) {
	ctx := c.Request.Context()
	var query ListShopsQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		response.WriteInvalidParam(c, err)
		return
	}

	shops, total, err := h.svc.List(ctx, query.PageNum, query.PageSize)
	if err != nil {
		response.Write(c, err, nil)
		return
	}

	items := make([]Item, 0, len(shops))
	for _, s := range shops {
		items = append(items, *FormatItem(s))
	}
	response.Write(c, nil, ListResponse{Shops: items, Total: total})
}
