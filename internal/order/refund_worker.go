package order

import (
	"context"
	"errors"
	"time"

	"e-commerce/internal/coupon"
	"e-commerce/internal/model"
	"e-commerce/internal/pkg/database"
	"e-commerce/pkg/clog"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

const (
	refundWorkerScanInterval    = 5 * time.Second
	refundWorkerBatchSize       = 20
	refundWorkerRecoveryDelay   = 30 * time.Second
	refundWorkerChannelTimeout  = 10 * time.Second
	refundWorkerMaxCreateTries  = 5
	refundWorkerMaxQueryTries   = 10
	refundWorkerRetryBase       = 5 * time.Second
	refundWorkerRetryMax        = time.Minute
	refundWorkerQueryParkPeriod = 24 * time.Hour
)

type RefundWorker struct {
	repo    *Repository
	channel refundChannel
	coupon  *coupon.Repository
}

func NewRefundWorker(repo *Repository, couponRepo *coupon.Repository) *RefundWorker {
	return &RefundWorker{repo: repo, channel: newMockRefundChannel(), coupon: couponRepo}
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
		if err := w.processJob(ctx, job); err != nil {
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

func (w *RefundWorker) processJob(ctx context.Context, job *model.RefundJob) error {
	refund, err := w.repo.GetRefundByID(ctx, job.RefundID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return w.repo.DeleteRefundJob(ctx, job.ID)
		}
		return err
	}
	if refund.Status == model.RefundStatusSucceeded || refund.Status == model.RefundStatusFailed {
		return w.repo.DeleteRefundJob(ctx, job.ID)
	}

	jobType := job.JobType
	if jobType == model.RefundJobTypeCreate && refund.Status == model.RefundStatusProcessing {
		jobType = model.RefundJobTypeQuery
	}
	if jobType == model.RefundJobTypeCreate {
		return w.processCreate(ctx, job, refund)
	}
	return w.processQuery(ctx, job, refund)
}

func (w *RefundWorker) processCreate(ctx context.Context, job *model.RefundJob, refund *model.Refund) error {
	createAttempts := job.CreateAttempts
	if createAttempts >= refundWorkerMaxCreateTries {
		return w.terminalFailed(ctx, job, "retry limit exceeded")
	}
	if err := w.beginCreateAttempt(ctx, job.ID, refund.ID); err != nil {
		return err
	}
	createAttempts++

	callCtx, cancel := context.WithTimeout(ctx, refundWorkerChannelTimeout)
	result, err := w.channel.Create(callCtx, refund.ID, refund.Amount)
	cancel()
	if err != nil {
		result = refundChannelResult{Outcome: refundOutcomeUnknown}
	}
	return w.applyResult(ctx, job, result, createAttempts, job.QueryAttempts, true)
}

func (w *RefundWorker) processQuery(ctx context.Context, job *model.RefundJob, refund *model.Refund) error {
	queryAttempts := job.QueryAttempts
	if queryAttempts >= refundWorkerMaxQueryTries {
		return w.repo.RescheduleRefundJob(ctx, job.ID, model.RefundJobTypeQuery,
			time.Now().Add(refundWorkerQueryParkPeriod), "query attempts exhausted")
	}
	if err := w.beginQueryAttempt(ctx, job.ID); err != nil {
		return err
	}
	queryAttempts++

	callCtx, cancel := context.WithTimeout(ctx, refundWorkerChannelTimeout)
	result, err := w.channel.Query(callCtx, refund.ID)
	cancel()
	if err != nil {
		result = refundChannelResult{Outcome: refundOutcomeUnknown}
	}
	return w.applyResult(ctx, job, result, job.CreateAttempts, queryAttempts, false)
}

func (w *RefundWorker) beginCreateAttempt(ctx context.Context, jobID, refundID uuid.UUID) error {
	return database.ExecuteTransaction(ctx, w.repo.GetDB(ctx), func(txCtx context.Context) error {
		if err := w.repo.MarkRefundCreateAttempt(txCtx, jobID); err != nil {
			return err
		}
		return w.repo.SetRefundProcessing(txCtx, refundID)
	})
}

func (w *RefundWorker) beginQueryAttempt(ctx context.Context, jobID uuid.UUID) error {
	return database.ExecuteTransaction(ctx, w.repo.GetDB(ctx), func(txCtx context.Context) error {
		return w.repo.MarkRefundQueryAttempt(txCtx, jobID)
	})
}

func (w *RefundWorker) applyResult(ctx context.Context, job *model.RefundJob, result refundChannelResult, createAttempts, queryAttempts int, isCreate bool) error {
	return database.ExecuteTransaction(ctx, w.repo.GetDB(ctx), func(txCtx context.Context) error {
		refund, err := w.repo.GetRefundForUpdate(txCtx, job.RefundID)
		if err != nil {
			return err
		}
		if refund.Status == model.RefundStatusSucceeded || refund.Status == model.RefundStatusFailed {
			return w.repo.DeleteRefundJob(txCtx, job.ID)
		}

		switch result.Outcome {
		case refundOutcomeSucceeded:
			now := time.Now()
			order, err := w.repo.GetOrderForUpdate(txCtx, refund.OrderID)
			if err != nil {
				return err
			}
			if order.UserCouponID != nil && w.coupon != nil {
				if err := w.coupon.ReturnCouponForOrder(txCtx, *order.UserCouponID, order.ID); err != nil && !errors.Is(err, coupon.ErrCouponAlreadyUsed) {
					return err
				}
			}
			if err := w.repo.UpdateRefundStatus(txCtx, refund.ID, model.RefundStatusSucceeded, "", &now); err != nil {
				return err
			}
			if err := w.repo.MarkOrderRefunded(txCtx, refund.OrderID); err != nil {
				return err
			}
			return w.repo.DeleteRefundJob(txCtx, job.ID)

		case refundOutcomeFailed:
			if createAttempts >= refundWorkerMaxCreateTries {
				if err := w.repo.UpdateRefundStatus(txCtx, refund.ID, model.RefundStatusFailed, result.FailureReason, nil); err != nil {
					return err
				}
				return w.repo.DeleteRefundJob(txCtx, job.ID)
			}
			if err := w.repo.UpdateRefundStatus(txCtx, refund.ID, model.RefundStatusWaitRetry, result.FailureReason, nil); err != nil {
				return err
			}
			return w.repo.RescheduleRefundJob(txCtx, job.ID, model.RefundJobTypeCreate,
				time.Now().Add(retryBackoff(createAttempts)), result.FailureReason)

		case refundOutcomeNotFound:
			if createAttempts >= refundWorkerMaxCreateTries {
				if err := w.repo.UpdateRefundStatus(txCtx, refund.ID, model.RefundStatusFailed, "retry limit exceeded", nil); err != nil {
					return err
				}
				return w.repo.DeleteRefundJob(txCtx, job.ID)
			}
			if err := w.repo.UpdateRefundStatus(txCtx, refund.ID, model.RefundStatusWaitRetry, "", nil); err != nil {
				return err
			}
			return w.repo.RescheduleRefundJob(txCtx, job.ID, model.RefundJobTypeCreate, time.Now(), "not found")

		default:
			if err := w.repo.UpdateRefundStatus(txCtx, refund.ID, model.RefundStatusWaitRetry, "", nil); err != nil {
				return err
			}
			nextType := model.RefundJobTypeQuery
			if !isCreate && queryAttempts >= refundWorkerMaxQueryTries {
				return w.repo.RescheduleRefundJob(txCtx, job.ID, model.RefundJobTypeQuery,
					time.Now().Add(refundWorkerQueryParkPeriod), "query attempts exhausted")
			}
			return w.repo.RescheduleRefundJob(txCtx, job.ID, nextType,
				time.Now().Add(retryBackoff(createAttempts)), "channel result unknown")
		}
	})
}

func (w *RefundWorker) terminalFailed(ctx context.Context, job *model.RefundJob, reason string) error {
	return database.ExecuteTransaction(ctx, w.repo.GetDB(ctx), func(txCtx context.Context) error {
		if err := w.repo.UpdateRefundStatus(txCtx, job.RefundID, model.RefundStatusFailed, reason, nil); err != nil {
			return err
		}
		return w.repo.DeleteRefundJob(txCtx, job.ID)
	})
}

func retryBackoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	d := time.Duration(attempt) * refundWorkerRetryBase
	if d > refundWorkerRetryMax {
		return refundWorkerRetryMax
	}
	return d
}
