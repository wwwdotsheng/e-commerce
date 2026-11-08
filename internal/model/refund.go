package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type RefundStatus string

const (
	RefundStatusPending    RefundStatus = "pending"
	RefundStatusProcessing RefundStatus = "processing"
	RefundStatusWaitRetry  RefundStatus = "wait_retry"
	RefundStatusSucceeded  RefundStatus = "succeeded"
	RefundStatusFailed     RefundStatus = "failed"
	RefundStatusManual     RefundStatus = "manual"
)

type Refund struct {
	ID            uuid.UUID       `gorm:"column:id;primaryKey;type:uuid"`
	OrderID       uuid.UUID       `gorm:"column:order_id;type:uuid;not null;uniqueIndex:uni_refunds_order_id"`
	BuyerID       uuid.UUID       `gorm:"column:buyer_id;type:uuid;not null"`
	Amount        decimal.Decimal `gorm:"column:amount;type:numeric(16,2);not null"`
	Status        RefundStatus    `gorm:"column:status;type:varchar(20);not null"`
	FailureReason string          `gorm:"column:failure_reason;type:varchar(255);not null;default:''"`
	RefundedAt    *time.Time      `gorm:"column:refunded_at;type:timestamptz"`
	CreatedAt     time.Time       `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt     time.Time       `gorm:"column:updated_at;autoUpdateTime"`
}

func (r *Refund) TableName() string {
	return "refunds"
}

func (r *Refund) BeforeCreate(tx *gorm.DB) error {
	if r.ID == uuid.Nil {
		id, err := uuid.NewV7()
		if err != nil {
			return err
		}
		r.ID = id
	}
	return nil
}
