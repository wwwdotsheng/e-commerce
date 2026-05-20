package product

import (
	"context"
	"e-commerce/internal/model"
	"e-commerce/pkg/errno"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Service struct {
	db   *gorm.DB
	repo *Repository
}

func NewService(db *gorm.DB, repo *Repository) *Service {
	return &Service{db: db, repo: repo}
}

func (svc *Service) GetProduct(ctx context.Context, id uuid.UUID) (*model.Product, error) {
	p, err := svc.repo.GetProduct(ctx, id)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, errno.ErrProductNotFound
	}
	return p, nil
}

func (svc *Service) CreateProduct(ctx context.Context, param CreateProductParam) error {
	return svc.repo.CreateProduct(ctx, CreateProductData{
		Name:        param.Name,
		Description: param.Description,
		Price:       param.Price,
		Status:      param.Status,
		Stock:       param.Stock,
		Publisher:   param.Publisher,
	})
}

func (svc *Service) ListProducts(ctx context.Context, param ListProductsParam) ([]*model.Product, int64, error) {
	return svc.repo.ListProducts(ctx, ListProductsData{
		PageNum:  param.PageNum,
		PageSize: param.PageSize,
	})
}

func (svc *Service) DeleteProduct(ctx context.Context, param DeleteProductParam) error {
	return svc.repo.Update(ctx, UpdateProductPropertyData{
		ProductID: param.ProductID,
		Publisher: param.Publisher,
		Data:      map[string]interface{}{},
	})
}

func (svc *Service) UpdateProductProperty(ctx context.Context, param UpdateProductPropertyParam) error {
	updateData := map[string]interface{}{}
	if param.Name != nil {
		updateData["name"] = *param.Name
	}
	if param.Description != nil {
		updateData["description"] = *param.Description
	}
	if param.Price != nil {
		updateData["price"] = *param.Price
	}

	return svc.repo.Update(ctx, UpdateProductPropertyData{
		ProductID: param.ProductID,
		Publisher: param.Publisher,
		Data:      updateData,
	})
}

func (svc *Service) UpdateProductStatus(ctx context.Context, param UpdateProductStatusParam) error {
	if !param.Status.IsValid() {
		return errno.ErrProductStatusInvalid
	}
	return svc.repo.Update(ctx, UpdateProductPropertyData{
		ProductID: param.ProductID,
		Publisher: param.Publisher,
		Data: map[string]interface{}{
			"status": param.Status,
		},
	})
}

func (svc *Service) UpdateProductStock(ctx context.Context, param UpdateProductStockParam) error {
	return svc.repo.UpdateStock(ctx, UpdateStockData{
		ProductID: param.ProductID,
		Publisher: param.Publisher,
		Quantity:  param.Quantity,
		Reason:    param.Reason,
	})
}
