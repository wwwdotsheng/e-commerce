package order

import (
	"context"
	"e-commerce/internal/coupon"
	"e-commerce/internal/model"
	"e-commerce/internal/pkg/database"
	"e-commerce/internal/product"
	"e-commerce/pkg/errno"
	"e-commerce/pkg/clog"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type Service struct {
	db          *gorm.DB
	repo        *Repository
	productRepo *product.Repository
	couponRepo  *coupon.Repository
	metrics     *Metrics
}

func NewService(db *gorm.DB, repo *Repository, productRepo *product.Repository, couponRepo *coupon.Repository, metrics *Metrics) *Service {
	return &Service{db: db, repo: repo, productRepo: productRepo, couponRepo: couponRepo, metrics: metrics}
}

// CreateOrder 创建订单（支持可选优惠券）
func (svc *Service) CreateOrder(ctx context.Context, userID uuid.UUID, param CreateOrderParam) (err error) {
	var errCode = orderCreateErrInternal
	defer func() {
		if err != nil {
			svc.metrics.AddOrderCreateTotal(ctx, orderCreateStatusFail, errCode)
		} else {
			svc.metrics.AddOrderCreateTotal(ctx, orderCreateStatusSuccess, orderCreateErrNone)
		}
	}()

	var order *model.Order

	err = database.ExecuteTransaction(ctx, svc.db, func(ctx context.Context) error {
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

		var discountAmount float64
		var userCouponID *uuid.UUID

		if param.UserCouponID != uuid.Nil {
			uc, err := svc.couponRepo.GetUserCouponForUpdate(ctx, param.UserCouponID, userID)
			if err != nil {
				return err
			}

			if uc.Status != model.UserCouponStatusUnused {
				return coupon.ErrCouponAlreadyUsed
			}
			if time.Now().After(uc.ExpireTime) {
				return coupon.ErrCouponAlreadyUsed
			}

			template := uc.Template
			if template == nil {
				return coupon.ErrTemplateNotFound
			}

			orderAmount := p.Price * float64(param.Quantity)
			if template.MinAmount > 0 && orderAmount < template.MinAmount {
				return coupon.ErrCouponMinAmountNotMet
			}

			discountAmount = calcCouponDeduction(template, orderAmount)

			if err := svc.couponRepo.UseCouponWithVersion(ctx, uc.ID, userID, param.UserCouponID, uc.Version); err != nil {
				return err
			}

			couponID := param.UserCouponID
			userCouponID = &couponID
		}

		order = &model.Order{
			UserID:         userID,
			ProductId:      param.ProductID,
			Quantity:       param.Quantity,
			SnapshotTitle:  p.Name,
			SnapshotPrice:  p.Price,
			Status:         model.OrderStatusProcessing,
			UserCouponID:   userCouponID,
			DiscountAmount: discountAmount,
			IdempotencyKey: param.IdempotencyKey,
		}
		return svc.repo.CreateOrder(ctx, order)
	})
	if err != nil {
		if errors.Is(err, repoErrOrderIdempotencyConflict) {
			return nil
		}
		switch {
		case errors.Is(err, errno.ErrOrderProductIdNotFound):
			errCode = orderCreateErrProductNotFound
		case errors.Is(err, errno.ErrProductStockInsufficient):
			errCode = orderCreateErrStockInsufficient
		case errors.Is(err, coupon.ErrCouponAlreadyUsed) ||
			errors.Is(err, coupon.ErrTemplateNotFound) ||
			errors.Is(err, coupon.ErrCouponMinAmountNotMet) ||
			errors.Is(err, coupon.ErrCouponNotOwned):
			errCode = orderCreateErrCoupon
		}
		return err
	}

	if err := svc.repo.PublishTimeoutMessage(ctx, order.ID); err != nil {
		clog.L(ctx).Error("发送订单超时消息失败",
			zap.String("order_id", order.ID.String()),
			zap.Error(err),
		)
		errCode = orderCreateErrTimeoutSchedule
		return fmt.Errorf("订单已创建但超时调度失败: %w", err)
	}
	return nil
}

// calcCouponDeduction 计算优惠金额
func calcCouponDeduction(t *model.CouponTemplate, orderAmount float64) float64 {
	switch t.Type {
	case model.CouponTypeFixed:
		return t.DiscountValue
	case model.CouponTypePercentage:
		d := orderAmount * t.DiscountRate
		if t.MaxDeduction > 0 && d > t.MaxDeduction {
			return t.MaxDeduction
		}
		return d
	default:
		return 0
	}
}

func (svc *Service) HandleOrderTimeout(ctx context.Context, orderID uuid.UUID) (err error) {
	defer func() {
		if err != nil {
			svc.metrics.AddOrderTimeoutTotal(ctx, orderTimeoutStatusFail)
		} else {
			svc.metrics.AddOrderTimeoutTotal(ctx, orderTimeoutStatusClosed)
		}
	}()

	order, err := svc.repo.HandleOrderTimeout(ctx, orderID)
	if err != nil {
		return err
	}

	if order.UserCouponID != nil {
		if err := svc.couponRepo.ReturnCoupon(ctx, *order.UserCouponID); err != nil {
			clog.L(ctx).Error("退还优惠券失败",
				zap.String("order_id", orderID.String()),
				zap.String("coupon_id", order.UserCouponID.String()),
				zap.Error(err),
			)
		}
	}

	return nil
}

func (svc *Service) ListOrders(ctx context.Context, param ListOrdersParam) ([]*model.Order, int64, error) {
	return svc.repo.ListOrdersByUserID(ctx, param.UserID, param.PageNum, param.PageSize)
}
