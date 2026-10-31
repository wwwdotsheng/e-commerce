package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	ConstraintOrderIdempotencyKey = "uni_order_idempotency_key"
)

type OrderStatus int

const (
	// OrderStatusProcessing 已创建、等待买家支付；超过 payment_deadline 后由超时消息关闭。
	OrderStatusProcessing OrderStatus = 0
	// OrderStatusCompleted 已履约完成，一期尚未接入履约流程。
	OrderStatusCompleted OrderStatus = 1
	OrderStatusCancelled OrderStatus = 2
	// OrderStatusTimeout 支付超时关闭，close_reason 记录 payment_timeout。
	OrderStatusTimeout OrderStatus = 3
	// OrderStatusPaid 余额支付成功，paid_at 同时写入；等待履约或退款。
	OrderStatusPaid OrderStatus = 4
)

// OrderCloseReason is a stable reason code persisted as varchar.
type OrderCloseReason string

const OrderCloseReasonPaymentTimeout OrderCloseReason = "payment_timeout"

// Order 订单表
// TODO(10)[2026-05-04] 使得订单表字段不用与Product表绑定
type Order struct {
	CloseReason     *OrderCloseReason `gorm:"column:close_reason;type:varchar(32)"`
	PayableAmount   float64           `gorm:"column:payable_amount;type:numeric(16,2);not null;check:payable_amount >= 0"`
	PaymentDeadline time.Time         `gorm:"column:payment_deadline;type:timestamptz;not null;index"`
	PaidAt          *time.Time        `gorm:"column:paid_at;type:timestamptz"`
	ClosedAt        *time.Time        `gorm:"column:closed_at;type:timestamptz"`

	SellerID       uuid.UUID   `gorm:"column:seller_id;type:uuid;not null;index"`
	ID             uuid.UUID   `gorm:"column:id;primaryKey;type:uuid"`
	UserID         uuid.UUID   `gorm:"column:user_id;type:uuid"`
	ProductId      uuid.UUID   `gorm:"column:product_id;type:uuid"`
	Quantity       int         `gorm:"column:quantity;not null;check:quantity >= 0"`
	SnapshotTitle  string      `gorm:"column:snapshot_title;varchar(255);not null"`
	SnapshotPrice  float64     `gorm:"column:snapshot_price;decimal(16,2);not null"`
	Status         OrderStatus `gorm:"column:status;type:smallint;not null"`
	UserCouponID   *uuid.UUID  `gorm:"column:user_coupon_id;type:uuid"`
	DiscountAmount float64     `gorm:"column:discount_amount;decimal(16,2);not null;default:0"`
	IdempotencyKey string      `gorm:"column:idempotency_key;uniqueIndex:uni_order_idempotency_key;type:varchar(64);not null;"`
	CreatedAt      time.Time   `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt      time.Time   `gorm:"column:updated_at;autoUpdateTime"`
}

func (Order) TableName() string {
	return "orders"
}

func (o *Order) BeforeCreate(tx *gorm.DB) (err error) {
	if o.ID == uuid.Nil {
		id, err := uuid.NewV7()
		if err != nil {
			return err
		}
		o.ID = id
	}
	return nil
}
