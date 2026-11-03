package order

import (
	"context"
	"errors"
	"time"

	"e-commerce/internal/model"
	"e-commerce/internal/pkg/database"
	"e-commerce/pkg/errno"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func (svc *Service) ApplyRefund(ctx context.Context, userID, orderID uuid.UUID) (*model.Refund, error) {
	order, err := svc.repo.GetOrderByID(ctx, userID, orderID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errno.ErrNotFoundRecord
		}
		return nil, err
	}

	existing, err := svc.repo.GetRefundByOrderID(ctx, orderID)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	if order.Status != model.OrderStatusPaid {
		return nil, errno.ErrOrderRefundStatusInvalid
	}

	refund := &model.Refund{
		OrderID: order.ID,
		BuyerID: userID,
		Amount:  order.PayableAmount.Round(2),
		Status:  model.RefundStatusPending,
	}
	var result *model.Refund
	err = database.ExecuteTransaction(ctx, svc.db, func(txCtx context.Context) error {
		inserted, existing, err := svc.repo.CreateOrderRefund(txCtx, refund)
		if err != nil {
			return err
		}
		if !inserted {
			result = existing
			return nil
		}
		job := &model.RefundJob{
			RefundID:  refund.ID,
			JobType:   model.RefundJobTypeCreate,
			NextRunAt: time.Now(),
		}
		if err := svc.repo.CreateRefundJob(txCtx, job); err != nil {
			return err
		}
		result = refund
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

type OrderDetail struct {
	Order  *model.Order
	Refund *model.Refund
}

func (svc *Service) GetOrderDetail(ctx context.Context, userID, orderID uuid.UUID) (*OrderDetail, error) {
	order, err := svc.repo.GetOrderByID(ctx, userID, orderID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errno.ErrNotFoundRecord
		}
		return nil, err
	}
	refund, err := svc.repo.GetRefundByOrderID(ctx, orderID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &OrderDetail{Order: order}, nil
		}
		return nil, err
	}
	return &OrderDetail{Order: order, Refund: refund}, nil
}
