package product

import (
	"e-commerce/internal/model"

	"github.com/google/uuid"
)

type CreateProductParam struct {
	Name        string
	Description string
	Price       float64
	Status      *model.ProductStatus
	Stock       int
	SellerID    uuid.UUID
}
type UpdateProductStatusParam struct {
	ProductID uuid.UUID
	SellerID  uuid.UUID
	Status    model.ProductStatus
}

type ListProductsParam struct {
	PageNum  int
	PageSize int
}

type DeleteProductParam struct {
	ProductID uuid.UUID
	SellerID  uuid.UUID
}

type UpdateProductPropertyParam struct {
	ProductID   uuid.UUID
	SellerID    uuid.UUID
	Name        *string
	Description *string
	Price       *float64
}

type UpdateProductStockParam struct {
	ProductID uuid.UUID
	SellerID  uuid.UUID
	Quantity  int
	Reason    model.StockChangeReason
}

type SearchProductsParam struct {
	SellerID *uuid.UUID
	Query    string
	MinPrice *float64
	MaxPrice *float64
	PageNum  int
	PageSize int
}

type SearchProductsResult struct {
	Products []Item
	Total    int64
}
