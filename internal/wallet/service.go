package wallet

import (
	"context"
	"e-commerce/internal/pkg/database"
	"e-commerce/pkg/clog"
	"e-commerce/pkg/errno"
	"errors"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// walletLogIdempotencyKeyMaxLen 与 wallet_logs.idempotency_key 的 varchar(64) 保持一致。
const walletLogIdempotencyKeyMaxLen = 64

// depositIdempotencyKeyPattern 限制用户提供的幂等键字符集：字母、数字与 . _ : -，
// 避免控制字符、空白等进入日志和唯一索引。
var depositIdempotencyKeyPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]+$`)

type Service struct {
	walletRepo *Repository
	metrics    *Metrics
}

type DepositInput struct {
	Amount         decimal.Decimal
	IdempotencyKey string
}

func NewService(walletRepo *Repository, metrics *Metrics) *Service {
	return &Service{walletRepo: walletRepo, metrics: metrics}
}

func (svc *Service) Deposit(ctx context.Context, UserID uuid.UUID, SessionID string, input *DepositInput) (err error) {
	var errCode = depositErrInternal
	defer func() {
		if err != nil {
			svc.metrics.AddDepositTotal(ctx, depositStatusFail, errCode)
		} else {
			svc.metrics.AddDepositTotal(ctx, depositStatusSuccess, depositErrNone)
		}
	}()

	logger := clog.L(ctx)
	// 金额在进入数据库前显式保留两位小数，避免 0.001 这类输入被库列宽静默截断。
	input.Amount = input.Amount.Round(2)
	if !input.Amount.IsPositive() {
		logger.Warn(errno.ErrWalletInvalidDepositAmount.Message,
			zap.String("user_id", UserID.String()),
			zap.String("session_id", SessionID),
		)
		errCode = depositErrInvalid
		return errno.ErrWalletInvalidDepositAmount
	}
	if err := validateDepositIdempotencyKey(input.IdempotencyKey); err != nil {
		logger.Warn(errno.ErrWalletInvalidIdempotencyKey.Message,
			zap.String("user_id", UserID.String()),
			zap.Int("key_length", len(input.IdempotencyKey)),
		)
		errCode = depositErrInvalid
		return err
	}

	err = database.ExecuteTransaction(ctx, svc.walletRepo.GetDB(ctx), func(txCtx context.Context) error {
		return svc.walletRepo.Deposit(txCtx, UserID, SessionID, input)
	})
	if errors.Is(err, ErrWalletLogAlreadyExists) {
		return nil
	}
	return err
}

func (svc *Service) GetBalance(ctx context.Context, userID uuid.UUID) (decimal.Decimal, error) {
	balance, err := svc.walletRepo.GetBalance(ctx, userID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return decimal.Zero, errno.ErrNotFoundRecord
	}
	return balance, err
}

// validateDepositIdempotencyKey 保证用户提供的充值幂等键不会占用系统订单流水的保留命名空间、
// 长度不超过 wallet_logs.idempotency_key 列宽、字符集安全，避免写入时退化为 500。
func validateDepositIdempotencyKey(key string) error {
	if key == "" || len(key) > walletLogIdempotencyKeyMaxLen || strings.HasPrefix(key, orderLogKeyPrefix) || !depositIdempotencyKeyPattern.MatchString(key) {
		return errno.ErrWalletInvalidIdempotencyKey
	}
	return nil
}
