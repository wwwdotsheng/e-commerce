package order

import (
	"context"
	"e-commerce/internal/model"
	"e-commerce/internal/pkg/database"
	"e-commerce/internal/product"
	"e-commerce/pkg/errno"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Service struct {
	db          *gorm.DB
	repo        *Repository
	productRepo *product.Repository
}

func NewService(db *gorm.DB, repo *Repository, productRepo *product.Repository) *Service {
	return &Service{db: db, repo: repo, productRepo: productRepo}
}

// CreateOrder 创建订单
func (svc *Service) CreateOrder(ctx context.Context, userID uuid.UUID, param CreateOrderParam) error {
	err := database.ExecuteTransaction(ctx, svc.db, func(ctx context.Context) error {
		p, err := svc.productRepo.GetProductByID(ctx, param.ProductID, database.LockUpdate)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errno.ErrOrderProductIdNotFound
			}
			return errno.ErrOrderProductIdNotFound.WithRaw(
				fmt.Errorf("get product by id: %s, error: %w", param.ProductID, err),
			)
		}

		if err := svc.productRepo.DeductStock(ctx, param.ProductID, param.Quantity); err != nil {
			return err
		}

		order := &model.Order{
			UserID:         userID,
			ProductId:      param.ProductID,
			Quantity:       param.Quantity,
			SnapshotTitle:  p.Name,
			SnapshotPrice:  p.Price,
			Status:         model.OrderStatusProcessing,
			IdempotencyKey: param.IdempotencyKey,
		}
		return svc.repo.CreateOrder(ctx, order)
	})
	if errors.Is(err, repoErrOrderIdempotencyConflict) {
		return nil
	}
	return err
}

func (svc *Service) HandleOrderTimeout(ctx context.Context, orderID uuid.UUID) error {
	return svc.repo.HandleOrderTimeout(ctx, orderID)
}

func (svc *Service) ListOrders(ctx context.Context, param ListOrdersParam) ([]*model.Order, int64, error) {
	return svc.repo.ListOrdersByUserID(ctx, param.UserID, param.PageNum, param.PageSize)
}
