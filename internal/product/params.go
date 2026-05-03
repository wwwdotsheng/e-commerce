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
	Publisher   uuid.UUID
}
type UpdateProductStatusParam struct {
	ProductID uuid.UUID
	Publisher uuid.UUID
	Status    model.ProductStatus
}

type ListProductsParam struct {
	PageNum  int
	PageSize int
}

type DeleteProductParam struct {
	ProductID uuid.UUID
	Publisher uuid.UUID
}

type UpdateProductPropertyParam struct {
	ProductID   uuid.UUID
	Publisher   uuid.UUID
	Name        *string
	Description *string
	Price       *float64
}

type UpdateProductStockParam struct {
	ProductID uuid.UUID
	Publisher uuid.UUID
	Quantity  int
}
