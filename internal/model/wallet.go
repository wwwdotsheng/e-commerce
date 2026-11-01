package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

const (
	ConstraintWalletLogIdempotencyKey = "uni_wallet_log_idempotency_key"
)

// WalletLogType 标识余额变动的来源，amount 带符号：支出为负，收入为正。
const (
	WalletLogTypeDeposit      = "deposit"
	WalletLogTypeOrderPayment = "order_payment"
	WalletLogTypeOrderIncome  = "order_income"
)

type UserWallet struct {
	UserID    uuid.UUID       `gorm:"column:user_id;primaryKey;type:uuid"`
	Balance   decimal.Decimal `gorm:"column:balance;type:decimal(16,2);not null;default:0"`
	CreatedAt time.Time       `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt time.Time       `gorm:"column:updated_at;autoUpdateTime"`
}

func (UserWallet) TableName() string {
	return "user_wallets"
}

type WalletLog struct {
	ID             uuid.UUID       `gorm:"column:id;primaryKey;type:uuid"`
	UserID         uuid.UUID       `gorm:"column:user_id;type:uuid;not null"`
	SessionID      string          `gorm:"column:session_id;not null"`
	Amount         decimal.Decimal `gorm:"column:amount;type:decimal(16,2);not null;"`
	Type           string          `gorm:"column:type;type:varchar(20);not null;"`
	IdempotencyKey string          `gorm:"column:idempotency_key;uniqueIndex:uni_wallet_log_idempotency_key;type:varchar(64);not null;"`
	CreatedAt      time.Time       `gorm:"column:created_at;autoCreateTime"`
}

func (WalletLog) TableName() string {
	return "wallet_logs"
}

func (wl *WalletLog) BeforeCreate(tx *gorm.DB) (err error) {
	if wl.ID == uuid.Nil {
		id, err := uuid.NewV7()
		if err != nil {
			return err
		}
		wl.ID = id
	}
	return nil
}
