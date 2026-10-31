package order

import (
	"context"
	"e-commerce/internal/coupon"
	"e-commerce/internal/model"
	"e-commerce/internal/pkg/database"
	"e-commerce/internal/product"
	"e-commerce/internal/wallet"
	"e-commerce/pkg/clog"
	"e-commerce/pkg/errno"
	"errors"
	"fmt"
	"math"
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
	walletRepo  *wallet.Repository
	metrics     *Metrics
}

func NewService(db *gorm.DB, repo *Repository, productRepo *product.Repository, couponRepo *coupon.Repository, walletRepo *wallet.Repository, metrics *Metrics) *Service {
	return &Service{db: db, repo: repo, productRepo: productRepo, couponRepo: couponRepo, walletRepo: walletRepo, metrics: metrics}
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
	replay := false
	orderID, idErr := uuid.NewV7()
	if idErr != nil {
		return idErr
	}
	matches := func(existing *model.Order) error {
		couponID := uuid.Nil
		if existing.UserCouponID != nil {
			couponID = *existing.UserCouponID
		}
		if existing.UserID != userID || existing.ProductId != param.ProductID ||
			existing.Quantity != param.Quantity || couponID != param.UserCouponID {
			return errno.ErrInvalidParam.WithRaw(errors.New("idempotency key reused with a different account or payload"))
		}
		return nil
	}

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

		// Product locking serializes replays before inventory/coupon validation.
		existing, lookupErr := svc.repo.FindByIdempotencyKey(ctx, param.IdempotencyKey)
		if lookupErr == nil {
			replay = true
			return matches(existing)
		}
		if !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
			return lookupErr
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

			if err := svc.couponRepo.UseCouponWithVersion(ctx, uc.ID, userID, orderID, uc.Version); err != nil {
				return err
			}

			couponID := param.UserCouponID
			userCouponID = &couponID
		}

		discountAmount = math.Round(discountAmount*100) / 100
		now := time.Now()
		order = &model.Order{
			PayableAmount:   calculatePayableAmount(p.Price, param.Quantity, discountAmount),
			PaymentDeadline: now.Add(time.Duration(svc.repo.mqCfg.TTLMs) * time.Millisecond),
			CreatedAt:       now,

			SellerID:       p.SellerID,
			ID:             orderID,
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
			existing, lookupErr := svc.repo.FindByIdempotencyKey(ctx, param.IdempotencyKey)
			if lookupErr != nil {
				return lookupErr
			}
			return matches(existing)
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

	if replay {
		return nil
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

	// Closing, inventory restoration and coupon return must commit together.
	return database.ExecuteTransaction(ctx, svc.db, func(txCtx context.Context) error {
		o, err := svc.repo.HandleOrderTimeout(txCtx, orderID)
		if err != nil {
			return err
		}
		if o == nil {
			return nil
		}
		if err := svc.productRepo.RestoreOrderStock(txCtx, o.ProductId, o.Quantity); err != nil {
			return err
		}
		if o.UserCouponID != nil {
			if err := svc.couponRepo.ReturnCouponForOrder(txCtx, *o.UserCouponID, o.ID); err != nil {
				return err
			}
		}
		return nil
	})
}

func (svc *Service) ListOrders(ctx context.Context, param ListOrdersParam) ([]*model.Order, int64, error) {
	return svc.repo.ListOrdersByUserID(ctx, param.UserID, param.PageNum, param.PageSize)
}

func (svc *Service) GetOrderByID(ctx context.Context, userID, orderID uuid.UUID) (*model.Order, error) {
	return svc.repo.GetOrderByID(ctx, userID, orderID)
}

// PayOrder 使用买家余额支付订单：扣买家余额、记入商家余额、订单转为已支付在同一事务内完成。
// 重复支付返回当前订单且不再扣款；已关闭或超过截止时间的订单拒绝支付。
func (svc *Service) PayOrder(ctx context.Context, userID uuid.UUID, sessionID string, orderID uuid.UUID) (paid *model.Order, err error) {
	errCode := orderPaymentErrInternal
	defer func() {
		if err != nil {
			svc.metrics.AddOrderPaymentTotal(ctx, orderPaymentStatusFail, errCode)
			return
		}
		svc.metrics.AddOrderPaymentTotal(ctx, orderPaymentStatusSuccess, orderPaymentErrNone)
	}()

	err = database.ExecuteTransaction(ctx, svc.db, func(txCtx context.Context) error {
		current, err := svc.repo.GetOrderForUpdateByUser(txCtx, userID, orderID)
		if err != nil {
			return err
		}
		// 已支付订单的重复请求直接返回已有结果，不重复扣款。
		if current.Status == model.OrderStatusPaid {
			paid = current
			return nil
		}
		if current.Status != model.OrderStatusProcessing {
			return errno.ErrOrderPaymentStatusInvalid
		}
		now := time.Now()
		if !now.Before(current.PaymentDeadline) {
			return errno.ErrOrderPaymentExpired
		}

		if current.PayableAmount > 0 {
			if err := svc.walletRepo.DebitForOrderPayment(txCtx, userID, current.PayableAmount, sessionID, current.ID); err != nil {
				if errors.Is(err, wallet.ErrInsufficientBalance) {
					return errno.ErrWalletBalanceInsufficient
				}
				return err
			}
			if err := svc.walletRepo.CreditOrderIncome(txCtx, current.SellerID, current.PayableAmount, sessionID, current.ID); err != nil {
				return err
			}
		}
		if err := svc.repo.MarkOrderPaid(txCtx, current.ID, now); err != nil {
			return err
		}
		persisted, err := svc.repo.GetOrderByID(txCtx, userID, orderID)
		if err != nil {
			return err
		}
		paid = persisted
		return nil
	})
	if err != nil {
		switch {
		case errors.Is(err, errno.ErrWalletBalanceInsufficient):
			errCode = orderPaymentErrInsufficientBalance
		case errors.Is(err, errno.ErrOrderPaymentExpired):
			errCode = orderPaymentErrExpired
		case errors.Is(err, errno.ErrOrderPaymentStatusInvalid):
			errCode = orderPaymentErrStatusInvalid
		case errors.Is(err, errno.ErrNotFoundRecord):
			errCode = orderPaymentErrNotFound
		}
		return nil, err
	}
	return paid, nil
}

// calculatePayableAmount fixes the amount at order creation, retaining the zero floor.
func calculatePayableAmount(unitPrice float64, quantity int, discount float64) float64 {
	gross := math.Round(unitPrice*float64(quantity)*100) / 100
	return math.Max(0, math.Round((gross-math.Round(discount*100)/100)*100)/100)
}
