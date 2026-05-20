package product

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

func (h *Handler) CreateProduct(c *gin.Context) {
	ctx := c.Request.Context()

	accountInfo := identity.GetAccountInfo(ctx)
	if accountInfo == nil {
		response.Write(c, errno.ErrGetAccountInfo, nil)
		return
	}

	var body CreateProductBody

	if err := c.ShouldBindJSON(&body); err != nil {
		response.WriteInvalidParam(c, err)
		return
	}

	// TODO(9)[2026-04-29] 校验逻辑好像放在service会好一点吗？
	//  假如后面要增设grpc调用svc，如果放在http handler那不就得再编编写一次校验规则？
	if err := h.svc.CreateProduct(ctx, CreateProductParam{
		Name:        body.Name,
		Description: body.Description,
		Price:       body.Price,
		Status:      body.Status,
		Stock:       body.Stock,
		Publisher:   accountInfo.AccountId,
	}); err != nil {
		response.Write(c, err, nil)
		return
	}
	response.Write(c, nil, nil)
}

func (h *Handler) ListProducts(c *gin.Context) {
	ctx := c.Request.Context()

	accountInfo := identity.GetAccountInfo(ctx)
	if accountInfo == nil {
		response.Write(c, errno.ErrGetAccountInfo, nil)
		return
	}

	var query ListProductsQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		response.WriteInvalidParam(c, err)
		return
	}

	products, total, err := h.svc.ListProducts(c, ListProductsParam{
		PageNum:  query.PageNum,
		PageSize: query.PageSize,
	})
	if err != nil {
		response.Write(c, err, nil)
		return
	}

	items := make([]Item, 0, len(products))
	for _, p := range products {
		items = append(items, *FormatItem(p))
	}

	response.Write(c, nil, ListProductsResponse{
		Products: items,
		Total:    total,
	})
}

func (h *Handler) GetProduct(c *gin.Context) {
	ctx := c.Request.Context()

	var uri UriWithProductID
	if err := c.ShouldBindUri(&uri); err != nil {
		response.WriteInvalidParam(c, err)
		return
	}

	productID, err := uuid.Parse(uri.ID)
	if err != nil {
		response.WriteInvalidParam(c, err)
		return
	}

	p, err := h.svc.GetProduct(ctx, productID)
	if err != nil {
		response.Write(c, err, nil)
		return
	}

	response.Write(c, nil, FormatDetail(p))
}

func (h *Handler) DeleteProduct(c *gin.Context) {
	ctx := c.Request.Context()

	var uri UriWithProductID
	if err := c.ShouldBindUri(&uri); err != nil {
		response.WriteInvalidParam(c, err)
		return
	}

	accountInfo := identity.GetAccountInfo(ctx)
	if accountInfo == nil {
		response.Write(c, errno.ErrGetAccountInfo, nil)
		return
	}

	productID, err := uuid.Parse(uri.ID)
	if err != nil {
		response.WriteInvalidParam(c, err)
		return
	}

	if err = h.svc.DeleteProduct(ctx, DeleteProductParam{
		ProductID: productID,
		Publisher: accountInfo.AccountId,
	}); err != nil {
		response.Write(c, err, nil)
		return
	}

	response.Write(c, nil, nil)
}

func (h *Handler) UpdateProductProperty(c *gin.Context) {
	ctx := c.Request.Context()

	var uri UriWithProductID
	var body UpdateProductPropertyBody
	if err := c.ShouldBindUri(&uri); err != nil {
		response.WriteInvalidParam(c, err)
		return
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		response.WriteInvalidParam(c, err)
		return

	}

	accountInfo := identity.GetAccountInfo(ctx)
	if accountInfo == nil {
		response.Write(c, errno.ErrGetAccountInfo, nil)
		return
	}

	productID, err := uuid.Parse(uri.ID)
	if err != nil {
		response.WriteInvalidParam(c, err)
		return
	}

	if err = h.svc.UpdateProductProperty(ctx, UpdateProductPropertyParam{
		ProductID:   productID,
		Publisher:   accountInfo.AccountId,
		Name:        body.Name,
		Description: body.Description,
		Price:       body.Price,
	}); err != nil {
		response.Write(c, err, nil)
		return
	}

	response.Write(c, nil, nil)
}

func (h *Handler) UpdateProductStatus(c *gin.Context) {
	ctx := c.Request.Context()

	var uri UriWithProductID
	var body UpdateProductStatusBody
	if err := c.ShouldBindUri(&uri); err != nil {
		response.WriteInvalidParam(c, err)
		return
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		response.WriteInvalidParam(c, err)
		return
	}

	accountInfo := identity.GetAccountInfo(ctx)
	if accountInfo == nil {
		response.Write(c, errno.ErrGetAccountInfo, nil)
		return
	}

	productID, err := uuid.Parse(uri.ID)
	if err != nil {
		response.WriteInvalidParam(c, err)
		return
	}

	if err = h.svc.UpdateProductStatus(ctx, UpdateProductStatusParam{
		ProductID: productID,
		Publisher: accountInfo.AccountId,
		Status:    body.Status,
	}); err != nil {
		response.Write(c, err, nil)
		return
	}

	response.Write(c, nil, nil)
}
