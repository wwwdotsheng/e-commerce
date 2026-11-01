package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// RefundStatus 退款状态。
// processing 本次请求正在调用模拟渠道；succeeded/failed 为终态；
// confirming 表示渠道结果不明（超时/无响应），只能通过查询同一退款标识确认。
type RefundStatus string

const (
	RefundStatusProcessing RefundStatus = "processing"
	RefundStatusSucceeded  RefundStatus = "succeeded"
	RefundStatusFailed     RefundStatus = "failed"
	RefundStatusConfirming RefundStatus = "confirming"
)

// Refund 订单退款记录。一期一个订单最多一条记录，重复申请返回已有记录。
type Refund struct {
	ID             uuid.UUID       `gorm:"column:id;primaryKey;type:uuid"`
	OrderID        uuid.UUID       `gorm:"column:order_id;type:uuid;not null;uniqueIndex:uni_refunds_order_id"`
	BuyerID        uuid.UUID       `gorm:"column:buyer_id;type:uuid;not null"`
	Amount         decimal.Decimal `gorm:"column:amount;type:numeric(16,2);not null"`
	Status         RefundStatus    `gorm:"column:status;type:varchar(16);not null"`
	FailureReason  string          `gorm:"column:failure_reason;type:varchar(255);not null;default:''"`
	IdempotencyKey string          `gorm:"column:idempotency_key;type:varchar(64);not null"`
	Attempts       int             `gorm:"column:attempts;not null;default:0"`
	// RefundedAt 只在退款成功时写入一次，代表退款完成时间；失败/确认中保持 NULL。
	RefundedAt *time.Time `gorm:"column:refunded_at;type:timestamptz"`
	CreatedAt  time.Time  `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt  time.Time  `gorm:"column:updated_at;autoUpdateTime"`
}

func (Refund) TableName() string {
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
