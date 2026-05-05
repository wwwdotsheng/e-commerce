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

func NewService(db *gorm.DB, repo *Repository) *Service {
	return &Service{db: db, repo: repo}
}

type InputCreateOrder struct {
	productID      uuid.UUID
	quantity       int
	idempotencyKey string
}

// CreateOrder 创建订单
// TODO(1)[2026-04-29] 创建订单的时候如何标记一个特定的时期的特定商品？
func (svc *Service) CreateOrder(ctx context.Context, userID uuid.UUID, inputCreateOrder InputCreateOrder) error {
	return database.ExecuteTransaction(ctx, svc.db, func(ctx context.Context) error {
		// TODO(2)[2026-04-29]
		// 	这里如何一直请求是不是商家有可能就更新不了商品，会有这种情况吗？
		// 	还是说就这样，算他10w个用户同时抢一个，然后10w个用户之后就能更改商品了咋说
		p, err := svc.productRepo.GetProductByID(ctx, inputCreateOrder.productID, database.LockShare)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errno.ErrOrderProductIdNotFound
			}
			return errno.ErrOrderProductIdNotFound.WithRaw(
				fmt.Errorf("get product by id: %s, error: %w", inputCreateOrder.productID, err),
			)
		}

		order := &model.Order{
			UserID:         userID,
			ProductId:      inputCreateOrder.productID,
			Quantity:       inputCreateOrder.quantity,
			SnapshotTitle:  p.Name,
			SnapshotPrice:  p.Price,
			Status:         model.OrderStatusProcessing,
			IdempotencyKey: inputCreateOrder.idempotencyKey,
		}
		return svc.repo.CreateOrder(ctx, order)
	})
}

func (svc *Service) HandleOrderTimeout(ctx context.Context, orderID uuid.UUID) error {
	return svc.repo.HandleOrderTimeout(ctx, orderID)
}
