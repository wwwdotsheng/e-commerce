package wallet

import (
	"context"
	"e-commerce/internal/model"
	"e-commerce/internal/pkg/database"
	"errors"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	// ErrWalletLogAlreadyExists 表示 wallet_logs 的全局唯一幂等键已被占用。
	// 充值路径把它当作重复请求；订单支付路径必须由上层翻译成明确的业务错误。
	ErrWalletLogAlreadyExists = errors.New("wallet log already exists")
	// ErrInsufficientBalance 买家钱包余额不足以完成本次扣款。
	ErrInsufficientBalance = errors.New("wallet balance is insufficient")
	// ErrSellerWalletMissing 商家账号缺少钱包记录，属于需要人工处理的数据异常。
	ErrSellerWalletMissing = errors.New("seller wallet missing")
)

var constraintMap = map[string]error{
	model.ConstraintWalletLogIdempotencyKey: ErrWalletLogAlreadyExists,
}

type Repository struct {
	*database.BaseRepo
	rdb *redis.Client
}

func NewRepository(db *gorm.DB, rdb *redis.Client) *Repository {
	return &Repository{BaseRepo: database.NewBaseRepo(db), rdb: rdb}
}

func (repo *Repository) CreateDefaultAccount(ctx context.Context, userID uuid.UUID) error {
	record := &model.UserWallet{
		UserID:    userID,
		Balance:   decimal.Zero,
		UpdatedAt: time.Now(),
	}
	return repo.GetDB(ctx).Create(record).Error
}

func (repo *Repository) Deposit(ctx context.Context, userID uuid.UUID, sessionID string, input *DepositInput) error {
	log := &model.WalletLog{
		UserID:         userID,
		SessionID:      sessionID,
		Amount:         input.Amount,
		Type:           model.WalletLogTypeDeposit,
		IdempotencyKey: input.IdempotencyKey,
	}
	if err := repo.createLog(ctx, log); err != nil {
		return err
	}

	return repo.GetDB(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"balance":    gorm.Expr("user_wallets.balance + ?", input.Amount),
			"updated_at": time.Now(),
		}),
	}).Create(&model.UserWallet{
		UserID:    userID,
		Balance:   input.Amount,
		UpdatedAt: time.Now(),
	}).Error
}

func (repo *Repository) GetBalance(ctx context.Context, userID uuid.UUID) (decimal.Decimal, error) {
	var account model.UserWallet
	if err := repo.GetDB(ctx).First(&account, "user_id = ?", userID).Error; err != nil {
		return decimal.Zero, err
	}
	return account.Balance, nil
}

// DebitForOrderPayment 在调用方事务内扣减买家余额并写入一条负数账本。
// 余额不足时返回 ErrInsufficientBalance，扣款和账本一起回滚。
func (repo *Repository) DebitForOrderPayment(ctx context.Context, userID uuid.UUID, amount decimal.Decimal, sessionID string, orderID uuid.UUID) error {
	if !amount.IsPositive() {
		return nil
	}
	log := &model.WalletLog{
		UserID:         userID,
		SessionID:      sessionID,
		Amount:         amount.Neg(),
		Type:           model.WalletLogTypeOrderPayment,
		IdempotencyKey: orderPaymentLogKey(orderID),
	}
	if err := repo.createLog(ctx, log); err != nil {
		return err
	}

	result := repo.GetDB(ctx).Model(&model.UserWallet{}).
		Where("user_id = ? AND balance >= ?", userID, amount).
		Updates(map[string]interface{}{
			"balance":    gorm.Expr("user_wallets.balance - ?", amount),
			"updated_at": time.Now(),
		})
	if result.Error != nil {
		return fmt.Errorf("debit order payment for user %s: %w", userID, result.Error)
	}
	if result.RowsAffected != 1 {
		return ErrInsufficientBalance
	}
	return nil
}

// CreditOrderIncome 在调用方事务内把订单金额记入商家余额，并写入一条正数账本。
func (repo *Repository) CreditOrderIncome(ctx context.Context, sellerID uuid.UUID, amount decimal.Decimal, sessionID string, orderID uuid.UUID) error {
	if !amount.IsPositive() {
		return nil
	}
	log := &model.WalletLog{
		UserID:         sellerID,
		SessionID:      sessionID,
		Amount:         amount,
		Type:           model.WalletLogTypeOrderIncome,
		IdempotencyKey: orderIncomeLogKey(orderID),
	}
	if err := repo.createLog(ctx, log); err != nil {
		return err
	}

	result := repo.GetDB(ctx).Model(&model.UserWallet{}).
		Where("user_id = ?", sellerID).
		Updates(map[string]interface{}{
			"balance":    gorm.Expr("user_wallets.balance + ?", amount),
			"updated_at": time.Now(),
		})
	if result.Error != nil {
		return fmt.Errorf("credit order income for seller %s: %w", sellerID, result.Error)
	}
	if result.RowsAffected != 1 {
		// 商家账号应随注册创建钱包；缺失说明数据异常，整笔支付回滚。
		return fmt.Errorf("%w: seller %s", ErrSellerWalletMissing, sellerID)
	}
	return nil
}

// createLog 依赖 wallet_logs.idempotency_key 唯一索引兜底重复记账。
func (repo *Repository) createLog(ctx context.Context, log *model.WalletLog) error {
	if err := repo.GetDB(ctx).Create(log).Error; err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.SQLState() == pgerrcode.UniqueViolation {
			if mapErr, exists := constraintMap[pgErr.ConstraintName]; exists {
				return mapErr
			}
		}
		return fmt.Errorf("execute query error %w", err)
	}
	return nil
}

// orderLogKeyPrefix 是系统订单流水的保留命名空间。
// 充值幂等键由用户提供，必须拒绝该前缀，避免用户预占订单流水键导致订单永远无法支付。
const orderLogKeyPrefix = "order:"

// 订单支付与入账各只有一条账本，订单 ID 即幂等键。
func orderPaymentLogKey(orderID uuid.UUID) string {
	return orderLogKeyPrefix + orderID.String() + ":payment"
}

func orderIncomeLogKey(orderID uuid.UUID) string {
	return orderLogKeyPrefix + orderID.String() + ":income"
}
