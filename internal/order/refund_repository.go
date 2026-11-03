package order

import (
	"context"
	"fmt"
	"time"

	"e-commerce/internal/model"
	"e-commerce/pkg/errno"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
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

func (repo *Repository) GetRefundForUpdate(ctx context.Context, refundID uuid.UUID) (*model.Refund, error) {
	var r model.Refund
	if err := repo.GetDB(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		First(&r, "id = ?", refundID).Error; err != nil {
		return nil, err
	}
	return &r, nil
}

func (repo *Repository) CreateOrderRefund(ctx context.Context, refund *model.Refund) (bool, *model.Refund, error) {
	result := repo.GetDB(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(refund)
	if result.Error != nil {
		return false, nil, result.Error
	}
	if result.RowsAffected == 1 {
		return true, nil, nil
	}
	existing, err := repo.GetRefundByOrderID(ctx, refund.OrderID)
	if err != nil {
		return false, nil, err
	}
	return false, existing, nil
}

func (repo *Repository) CreateRefundJob(ctx context.Context, job *model.RefundJob) error {
	return repo.GetDB(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(job).Error
}

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

func (repo *Repository) FindDueRefundJobs(ctx context.Context, limit int) ([]*model.RefundJob, error) {
	var jobs []*model.RefundJob
	err := repo.GetDB(ctx).
		Where("next_run_at <= now()").
		Order("next_run_at").
		Limit(limit).
		Find(&jobs).Error
	return jobs, err
}

type orderRefundRow struct {
	model.Order
	RefundID            *uuid.UUID       `gorm:"column:refund_id"`
	RefundBuyerID       *uuid.UUID       `gorm:"column:refund_buyer_id"`
	RefundAmount        *decimal.Decimal `gorm:"column:refund_amount"`
	RefundStatus        *string          `gorm:"column:refund_status"`
	RefundFailureReason *string          `gorm:"column:refund_failure_reason"`
	RefundRefundedAt    *time.Time       `gorm:"column:refund_refunded_at"`
	RefundCreatedAt     *time.Time       `gorm:"column:refund_created_at"`
	RefundUpdatedAt     *time.Time       `gorm:"column:refund_updated_at"`
}

func (repo *Repository) GetOrderDetail(ctx context.Context, userID, orderID uuid.UUID) (*model.Order, *model.Refund, error) {
	const query = `
SELECT o.*,
       r.id AS refund_id,
       r.buyer_id AS refund_buyer_id,
       r.amount AS refund_amount,
       r.status AS refund_status,
       r.failure_reason AS refund_failure_reason,
       r.refunded_at AS refund_refunded_at,
       r.created_at AS refund_created_at,
       r.updated_at AS refund_updated_at
FROM orders o
LEFT JOIN refunds r ON r.order_id = o.id
WHERE o.id = ? AND o.user_id = ?
LIMIT 1`
	var row orderRefundRow
	result := repo.GetDB(ctx).Raw(query, orderID, userID).Scan(&row)
	if result.Error != nil {
		return nil, nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, nil, gorm.ErrRecordNotFound
	}
	if row.RefundID == nil {
		return &row.Order, nil, nil
	}
	refund := &model.Refund{
		ID:            *row.RefundID,
		OrderID:       row.Order.ID,
		BuyerID:       *row.RefundBuyerID,
		Amount:        *row.RefundAmount,
		Status:        model.RefundStatus(*row.RefundStatus),
		FailureReason: *row.RefundFailureReason,
		RefundedAt:    row.RefundRefundedAt,
		CreatedAt:     *row.RefundCreatedAt,
		UpdatedAt:     *row.RefundUpdatedAt,
	}
	return &row.Order, refund, nil
}
func (repo *Repository) GetRefundByID(ctx context.Context, refundID uuid.UUID) (*model.Refund, error) {
	var r model.Refund
	if err := repo.GetDB(ctx).First(&r, "id = ?", refundID).Error; err != nil {
		return nil, err
	}
	return &r, nil
}

func (repo *Repository) MarkRefundCreateAttempt(ctx context.Context, jobID uuid.UUID) error {
	return repo.markRefundJobAttempt(ctx, jobID, "create_attempts")
}

func (repo *Repository) MarkRefundQueryAttempt(ctx context.Context, jobID uuid.UUID) error {
	return repo.markRefundJobAttempt(ctx, jobID, "query_attempts")
}

func (repo *Repository) markRefundJobAttempt(ctx context.Context, jobID uuid.UUID, column string) error {
	switch column {
	case "create_attempts", "query_attempts":
	default:
		return fmt.Errorf("unsupported attempt column: %s", column)
	}
	updates := map[string]interface{}{
		"next_run_at": time.Now().Add(refundWorkerRecoveryDelay),
		"updated_at":  time.Now(),
		column:        gorm.Expr(column + " + 1"),
	}
	return repo.GetDB(ctx).Model(&model.RefundJob{}).Where("id = ?", jobID).Updates(updates).Error
}

func (repo *Repository) SetRefundProcessing(ctx context.Context, refundID uuid.UUID) error {
	return repo.GetDB(ctx).Model(&model.Refund{}).
		Where("id = ? AND status IN ?", refundID, []model.RefundStatus{model.RefundStatusPending, model.RefundStatusWaitRetry}).
		Update("status", model.RefundStatusProcessing).Error
}

func (repo *Repository) UpdateRefundStatus(ctx context.Context, refundID uuid.UUID, status model.RefundStatus, reason string, refundedAt *time.Time) error {
	updates := map[string]interface{}{
		"status":         status,
		"failure_reason": reason,
	}
	if refundedAt != nil {
		updates["refunded_at"] = *refundedAt
	}
	return repo.GetDB(ctx).Model(&model.Refund{}).Where("id = ?", refundID).Updates(updates).Error
}

func (repo *Repository) RescheduleRefundJob(ctx context.Context, jobID uuid.UUID, jobType model.RefundJobType, nextRunAt time.Time, lastError string) error {
	return repo.GetDB(ctx).Model(&model.RefundJob{}).Where("id = ?", jobID).Updates(map[string]interface{}{
		"job_type":    jobType,
		"next_run_at": nextRunAt,
		"last_error":  lastError,
		"updated_at":  time.Now(),
	}).Error
}

func (repo *Repository) DeleteRefundJob(ctx context.Context, jobID uuid.UUID) error {
	return repo.GetDB(ctx).Delete(&model.RefundJob{}, "id = ?", jobID).Error
}
