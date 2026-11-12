package order

import (
	"context"
	"errors"
	"time"

	"e-commerce/internal/coupon"
	"e-commerce/internal/model"
	"e-commerce/internal/pkg/database"
	"e-commerce/internal/wallet"
	"e-commerce/pkg/clog"
	"e-commerce/pkg/errno"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

const (
	refundWorkerScanInterval = 5 * time.Second
	refundWorkerBatchSize    = 20
	refundWorkerRetryBase    = 5 * time.Second
	refundWorkerRetryMax     = 10 * time.Minute
)

type RefundWorker struct {
	repo   *Repository
	coupon *coupon.Repository
	wallet *wallet.Repository
}

func NewRefundWorker(repo *Repository, couponRepo *coupon.Repository) *RefundWorker {
	return &RefundWorker{
		repo:   repo,
		coupon: couponRepo,
		wallet: wallet.NewRepository(repo.GetDB(context.Background()), nil),
	}
}

func (w *RefundWorker) Run(ctx context.Context) {
	ticker := time.NewTicker(refundWorkerScanInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := w.ProcessRefundJobs(ctx); err != nil {
				clog.L(ctx).Warn("退款任务扫描失败", zap.Error(err))
			}
		}
	}
}

func (w *RefundWorker) ProcessRefundJobs(ctx context.Context) (int, error) {
	jobs, err := w.repo.FindDueRefundJobs(ctx, refundWorkerBatchSize)
	if err != nil {
		return 0, err
	}
	processed := 0
	for _, job := range jobs {
		if ctx.Err() != nil {
			return processed, ctx.Err()
		}
		if err := w.processWalletRefund(ctx, job); err != nil {
			clog.L(ctx).Warn("退款任务处理失败",
				zap.String("refund_id", job.RefundID.String()),
				zap.String("job_type", string(job.JobType)),
				zap.Error(err))
			continue
		}
		processed++
	}
	return processed, nil
}

func (w *RefundWorker) processWalletRefund(ctx context.Context, job *model.RefundJob) error {
	return database.ExecuteTransaction(ctx, w.repo.GetDB(ctx), func(txCtx context.Context) error {
		refund, err := w.repo.GetRefundForUpdate(txCtx, job.RefundID)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return w.repo.DeleteRefundJob(txCtx, job.ID)
		}
		if err != nil {
			return err
		}
		if refund.Status == model.RefundStatusSucceeded || refund.Status == model.RefundStatusFailed || refund.Status == model.RefundStatusManual {
			return w.repo.DeleteRefundJob(txCtx, job.ID)
		}
		due, err := w.repo.isRefundJobDue(txCtx, job)
		if err != nil || !due {
			return err
		}
		order, err := w.repo.GetOrderForUpdate(txCtx, refund.OrderID)
		if err != nil {
			return err
		}
		if order.Status != model.OrderStatusPaid {
			return errno.ErrOrderRefundStatusInvalid
		}
		if refund.Amount.IsPositive() {
			err := w.wallet.TransferOrderRefund(txCtx, order.UserID, order.SellerID, refund.ID, refund.Amount)
			if errors.Is(err, wallet.ErrInsufficientBalance) {
				return w.failRefund(txCtx, job, "seller wallet balance is insufficient")
			}
			if err != nil {
				return err
			}
		}
		return w.completeRefund(txCtx, job.ID, refund, order)
	})
}

func (w *RefundWorker) completeRefund(ctx context.Context, jobID uuid.UUID, refund *model.Refund, order *model.Order) error {
	if order.UserCouponID != nil && w.coupon != nil {
		if err := w.coupon.ReturnCouponForOrder(ctx, *order.UserCouponID, order.ID); err != nil && !errors.Is(err, coupon.ErrCouponAlreadyUsed) {
			return err
		}
	}
	now := time.Now()
	if err := w.repo.UpdateRefundStatus(ctx, refund.ID, model.RefundStatusSucceeded, "", &now); err != nil {
		return err
	}
	if err := w.repo.MarkOrderRefunded(ctx, order.ID); err != nil {
		return err
	}
	return w.repo.DeleteRefundJob(ctx, jobID)
}

func (w *RefundWorker) failRefund(ctx context.Context, job *model.RefundJob, reason string) error {
	if err := w.repo.UpdateRefundStatus(ctx, job.RefundID, model.RefundStatusFailed, reason, nil); err != nil {
		return err
	}
	return w.repo.DeleteRefundJob(ctx, job.ID)
}

func retryBackoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	d := refundWorkerRetryBase
	for i := 1; i < attempt && d < refundWorkerRetryMax; i++ {
		d *= 2
	}
	if d > refundWorkerRetryMax {
		return refundWorkerRetryMax
	}
	return d
}
