package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	ConstraintWalletLogIdempotencyKey = "uni_wallet_log_idempotency_key"
)

type UserWallet struct {
	UserID    uuid.UUID `gorm:"column:user_id;primaryKey;type:uuid"`
	Balance   float64   `gorm:"column:balance;type:decimal(16,2);not null;default:0"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

type WalletLog struct {
	ID             uuid.UUID `gorm:"column:id;primaryKey;type:uuid"`
	UserID         uuid.UUID `gorm:"column:user_id;type:uuid;not null"`
	SessionID      string    `gorm:"column:session_id;not null"`
	Amount         float64   `gorm:"column:amount;type:decimal(16,2);not null;"`
	Type           string    `gorm:"column:type;type:varchar(20);not null;"`
	IdempotencyKey string    `gorm:"column:idempotency_key;uniqueIndex:uni_wallet_log_idempotency_key;type:varchar(64);not null;"`
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
