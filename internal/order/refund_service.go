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

// ApplyRefund 受理全额退款：只在一个短事务内写 refunds + refund_jobs，立即返回 pending。
// 本阶段不调用第三方渠道、不启动 worker；退款由未来的后台任务执行。
func (svc *Service) ApplyRefund(ctx context.Context, userID, orderID uuid.UUID) (*model.Refund, error) {
	order, err := svc.repo.GetOrderByID(ctx, userID, orderID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errno.ErrNotFoundRecord
		}
		return nil, err
	}

	// 先查已有退款：重复申请返回同一条记录，不再创建任务。
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
		inserted, existing, err := svc.repo.CreateRefundIfAbsent(txCtx, refund)
		if err != nil {
			return err
		}
		if !inserted {
			// 并发重复申请：另一个事务已提交同一条退款记录，直接复用，不再创建任务。
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

// GetOrderWithRefund 读取订单及其退款记录（无退款时 refund 为 nil），供订单详情一次性组装。
func (svc *Service) GetOrderWithRefund(ctx context.Context, userID, orderID uuid.UUID) (*model.Order, *model.Refund, error) {
	order, err := svc.repo.GetOrderByID(ctx, userID, orderID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, errno.ErrNotFoundRecord
		}
		return nil, nil, err
	}
	refund, err := svc.repo.GetRefundByOrderID(ctx, orderID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return order, nil, nil
		}
		return nil, nil, err
	}
	return order, refund, nil
}
