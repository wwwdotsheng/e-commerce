package model

import (
	"time"

	"github.com/google/uuid"
)

// StockChangeReason 库存变动原因
type StockChangeReason int8

const (
	StockChangeOrder   StockChangeReason = 1 // 下单扣减
	StockChangeRefund  StockChangeReason = 2 // 退单归还
	StockChangeTimeout StockChangeReason = 3 // 超时关单归还
	StockChangeManual  StockChangeReason = 4 // 手动调整
)

func (r StockChangeReason) String() string {
	switch r {
	case StockChangeOrder:
		return "order"
	case StockChangeRefund:
		return "refund"
	case StockChangeTimeout:
		return "timeout"
	case StockChangeManual:
		return "manual"
	default:
		return "unknown"
	}
}

type StockChangeLog struct {
	ID        uuid.UUID         `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	ProductID uuid.UUID         `gorm:"type:uuid;not null;index"`
	Quantity  int               `gorm:"not null;comment:变动数量，正数增加负数减少"`
	Before    int               `gorm:"not null;comment:变动前库存"`
	After     int               `gorm:"not null;comment:变动后库存"`
	Reason    StockChangeReason `gorm:"type:smallint;not null;index"`
	CreatedAt time.Time         `gorm:"not null;index"`
}

func (StockChangeLog) TableName() string {
	return "stock_change_logs"
}