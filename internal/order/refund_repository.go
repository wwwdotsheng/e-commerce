package order

import (
	"context"
	"fmt"

	"e-commerce/internal/model"
	"e-commerce/pkg/errno"

	"github.com/google/uuid"
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

// CreateRefundIfAbsent 用 ON CONFLICT DO NOTHING 处理并发重复申请：
// 插入成功返回 inserted=true；撞唯一约束则返回已存在的记录（inserted=false）。
func (repo *Repository) CreateRefundIfAbsent(ctx context.Context, refund *model.Refund) (bool, *model.Refund, error) {
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
	// refund_id 唯一：极端并发下任务已存在时忽略即可，退款记录是唯一事实来源。
	return repo.GetDB(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(job).Error
}

// MarkOrderRefunded 只允许已支付订单转为已退款；后台成功落库时与退款记录同事务调用。
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

// FindDueRefundJobs 供未来 worker 使用：按到期时间取待执行任务。
func (repo *Repository) FindDueRefundJobs(ctx context.Context, nowLimit int) ([]*model.RefundJob, error) {
	var jobs []*model.RefundJob
	err := repo.GetDB(ctx).
		Where("next_run_at <= now()").
		Order("next_run_at").
		Limit(nowLimit).
		Find(&jobs).Error
	return jobs, err
}
