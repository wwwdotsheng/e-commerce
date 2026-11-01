package order

import (
	"context"
	"fmt"
	"time"

	"e-commerce/internal/model"
	"e-commerce/pkg/errno"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (repo *Repository) GetRefundByOrderID(ctx context.Context, orderID uuid.UUID) (*model.Refund, error) {
	var r model.Refund
	if err := repo.GetDB(ctx).First(&r, "order_id = ?", orderID).Error; err != nil {
		return nil, err
	}
	return &r, nil
}

// GetRefundForUpdate 在事务内锁定退款记录，避免重复确认时并发覆盖终态。
func (repo *Repository) GetRefundForUpdate(ctx context.Context, refundID uuid.UUID) (*model.Refund, error) {
	var r model.Refund
	if err := repo.GetDB(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		First(&r, "id = ?", refundID).Error; err != nil {
		return nil, err
	}
	return &r, nil
}

func (repo *Repository) CreateRefund(ctx context.Context, refund *model.Refund) error {
	return repo.GetDB(ctx).Create(refund).Error
}

// MarkOrderRefunded 只允许已支付订单转为已退款，重复调用不会覆盖状态。
func (repo *Repository) MarkOrderRefunded(ctx context.Context, orderID uuid.UUID) error {
	w := repo.GetDB(ctx).Model(&model.Order{}).
		Where("id = ? AND status = ?", orderID, model.OrderStatusPaid).
		Update("status", model.OrderStatusRefunded)
	if w.Error != nil {
		return fmt.Errorf("mark order %s refunded: %w", orderID, w.Error)
	}
	if w.RowsAffected != 1 {
		return errno.ErrOrderRefundStatusInvalid
	}
	return nil
}

// UpdateRefundResult 记录本次渠道调用的结果；attempts 自增用于审计重试次数。
// 只有成功时写入 refunded_at，失败/确认中保持 NULL。
func (repo *Repository) UpdateRefundResult(ctx context.Context, refundID uuid.UUID, status model.RefundStatus, reason string, refundedAt *time.Time) error {
	updates := map[string]interface{}{
		"status":         status,
		"failure_reason": reason,
		"attempts":       gorm.Expr("attempts + 1"),
	}
	if status == model.RefundStatusSucceeded {
		updates["refunded_at"] = refundedAt
	}
	result := repo.GetDB(ctx).Model(&model.Refund{}).
		Where("id = ?", refundID).
		Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("update refund %s: %w", refundID, result.Error)
	}
	return nil
}
