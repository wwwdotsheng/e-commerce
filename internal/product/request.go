package product

import (
	"e-commerce/internal/model"

	"github.com/google/uuid"
)

type CreateProductBody struct {
	Name        string               `json:"name" binding:"required,min=2,max=120"`
	Description string               `json:"description" binding:"required,max=3000"`
	Price       float64              `json:"price" binding:"required,gt=0"`
	Status      *model.ProductStatus `json:"status" binding:"required,oneof=active inactive"`
	Stock       int                  `json:"stock" binding:"required,gte=0"`
	Publisher   uuid.UUID            `json:"publisher" binding:"required"`
}

type UriWithProductID struct {
	ID string `uri:"id" binding:"required"`
}

type ListProductsQuery struct {
	PageNum  int `form:"page_num" binding:"required,gt=0"`
	PageSize int `form:"page_size" binding:"required,max=20"`
}

type UpdateProductPropertyBody struct {
	Name        *string  `json:"name" binding:"omitempty,min=2,max=120"`
	Description *string  `json:"description" binding:"omitempty,max=3000"`
	Price       *float64 `json:"price" binding:"omitempty,gt=0"`
}
type UpdateProductStatusBody struct {
	Status model.ProductStatus `json:"status" binding:"required,oneof=active inactive"`
}
