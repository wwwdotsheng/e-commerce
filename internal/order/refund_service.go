package order

import (
	"context"
	"errors"
	"regexp"
	"time"

	"e-commerce/internal/model"
	"e-commerce/internal/pkg/database"
	"e-commerce/pkg/errno"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// refundIdempotencyKeyPattern 限制退款幂等键字符集，避免控制字符进入日志和唯一索引。
var refundIdempotencyKeyPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,64}$`)

// ApplyRefund 申请订单全额退款。
// 规则：
//   - 只有已支付订单可以申请；一个订单最多一条退款记录；
//   - 重复申请返回已有记录，不重复创建；failed 状态允许重新提交并复用同一记录重试；
//   - confirming（渠道结果不明）通过查询同一退款标识推进，不重新创建。
func (svc *Service) ApplyRefund(ctx context.Context, userID, orderID uuid.UUID, idempotencyKey string) (*model.Refund, error) {
	if !refundIdempotencyKeyPattern.MatchString(idempotencyKey) {
		return nil, errno.ErrInvalidParam.WithRaw(errors.New("invalid refund idempotency key"))
	}

	order, err := svc.repo.GetOrderByID(ctx, userID, orderID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errno.ErrNotFoundRecord
		}
		return nil, err
	}

	refund, err := svc.repo.GetRefundByOrderID(ctx, orderID)
	if err == nil {
		return svc.continueRefund(ctx, refund, idempotencyKey)
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if order.Status != model.OrderStatusPaid {
		return nil, errno.ErrOrderRefundStatusInvalid
	}

	refund = &model.Refund{
		OrderID:        order.ID,
		BuyerID:        userID,
		Amount:         order.PayableAmount.Round(2),
		Status:         model.RefundStatusProcessing,
		IdempotencyKey: idempotencyKey,
	}
	if err := svc.repo.CreateRefund(ctx, refund); err != nil {
		// 并发重复申请会撞 uni_refunds_order_id，退回已有记录继续处理。
		if existing, getErr := svc.repo.GetRefundByOrderID(ctx, orderID); getErr == nil {
			return svc.continueRefund(ctx, existing, idempotencyKey)
		}
		return nil, err
	}

	result, err := svc.channel.Create(ctx, refund.ID, refund.Amount)
	if err != nil {
		// 渠道调用异常按“结果不明”处理，等查询时确认，避免误判成功/失败。
		result = refundChannelResult{Status: model.RefundStatusConfirming}
	}
	return svc.applyRefundResult(ctx, refund, result)
}

// GetRefund 查询订单退款记录；confirming 状态会顺带向渠道确认一次结果。
func (svc *Service) GetRefund(ctx context.Context, userID, orderID uuid.UUID) (*model.Refund, error) {
	if _, err := svc.repo.GetOrderByID(ctx, userID, orderID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errno.ErrNotFoundRecord
		}
		return nil, err
	}
	refund, err := svc.repo.GetRefundByOrderID(ctx, orderID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errno.ErrNotFoundRecord
		}
		return nil, err
	}
	if refund.Status != model.RefundStatusConfirming {
		return refund, nil
	}
	result, err := svc.channel.Query(ctx, refund.ID)
	if err != nil {
		return refund, nil
	}
	return svc.applyRefundResult(ctx, refund, result)
}

// continueRefund 处理重复申请：成功直接返回；失败时同幂等键只返回、新键才重试；
// 结果不明查询确认；处理中返回现状。
func (svc *Service) continueRefund(ctx context.Context, refund *model.Refund, idempotencyKey string) (*model.Refund, error) {
	switch refund.Status {
	case model.RefundStatusSucceeded:
		return refund, nil
	case model.RefundStatusFailed:
		if idempotencyKey == refund.IdempotencyKey {
			return refund, nil
		}
		result, err := svc.channel.Create(ctx, refund.ID, refund.Amount)
		if err != nil {
			return refund, nil
		}
		return svc.applyRefundResult(ctx, refund, result)
	case model.RefundStatusConfirming:
		result, err := svc.channel.Query(ctx, refund.ID)
		if err != nil {
			return refund, nil
		}
		return svc.applyRefundResult(ctx, refund, result)
	default:
		return refund, nil
	}
}

// applyRefundResult 把渠道结果落库；成功时同步把订单状态改为已退款（同一事务）。
func (svc *Service) applyRefundResult(ctx context.Context, refund *model.Refund, result refundChannelResult) (*model.Refund, error) {
	var updated model.Refund
	now := time.Now()
	var refundedAt *time.Time
	if result.Status == model.RefundStatusSucceeded {
		refundedAt = &now
	}
	err := database.ExecuteTransaction(ctx, svc.db, func(txCtx context.Context) error {
		locked, err := svc.repo.GetRefundForUpdate(txCtx, refund.ID)
		if err != nil {
			return err
		}
		if locked.Status == model.RefundStatusSucceeded || locked.Status == model.RefundStatusFailed {
			updated = *locked
			return nil
		}
		if err := svc.repo.UpdateRefundResult(txCtx, locked.ID, result.Status, result.FailureReason, refundedAt); err != nil {
			return err
		}
		if result.Status == model.RefundStatusSucceeded {
			if err := svc.repo.MarkOrderRefunded(txCtx, locked.OrderID); err != nil {
				return err
			}
			locked.RefundedAt = refundedAt
		}
		locked.Status = result.Status
		locked.FailureReason = result.FailureReason
		locked.Attempts++
		updated = *locked
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &updated, nil
}
