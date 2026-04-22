package wallet

import (
	"context"
	"e-commerce/internal/pkg/database"
	"e-commerce/pkg/clog"
	"e-commerce/pkg/errno"
	"errors"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type Service struct {
	walletRepo *Repository
}

type DepositInput struct {
	Amount         float64
	IdempotencyKey string
}

func NewService(walletRepo *Repository) *Service {
	return &Service{walletRepo: walletRepo}
}

func (svc *Service) Deposit(ctx context.Context, UserID uuid.UUID, SessionID string, input *DepositInput) error {
	logger := clog.L(ctx)
	if input.Amount <= 0 {
		logger.Warn(errno.ErrWalletInvalidDepositAmount.Message,
			zap.String("user_id", UserID.String()),
			zap.String("session_id", SessionID),
		)
		return errno.ErrWalletInvalidDepositAmount
	}

	err := database.ExecuteTransaction(ctx, svc.walletRepo.GetDB(ctx), func(txCtx context.Context) error {
		return svc.walletRepo.Deposit(txCtx, UserID, SessionID, input)
	})
	if errors.Is(err, repoErrDepositRecordAlreadyExists) {
		return nil
	}
	return err
}
