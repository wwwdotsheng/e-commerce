package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type CouponType string

const (
	CouponTypeFixed      CouponType = "fixed_amount"
	CouponTypePercentage CouponType = "percentage"
)

func (ct CouponType) IsValid() bool {
	return ct == CouponTypeFixed || ct == CouponTypePercentage
}

type CouponStatus string

const (
	CouponStatusActive   CouponStatus = "active"
	CouponStatusInactive CouponStatus = "inactive"
)

// CouponTemplate 优惠券模板
type CouponTemplate struct {
	ID             uuid.UUID    `gorm:"column:id;primaryKey;type:uuid"`
	Name           string       `gorm:"column:name;type:varchar(128);not null"`
	Type           CouponType   `gorm:"column:type;type:varchar(16);not null"`
	DiscountValue  float64      `gorm:"column:discount_value;decimal(16,2);not null;default:0"`
	DiscountRate   float64      `gorm:"column:discount_rate;decimal(5,2);not null;default:0"`
	MaxDeduction   float64      `gorm:"column:max_deduction;decimal(16,2);not null;default:0"`
	MinAmount      float64      `gorm:"column:min_amount;decimal(16,2);not null;default:0"`
	TotalQty       int          `gorm:"column:total_qty;not null"`
	RemainingQty   int          `gorm:"column:remaining_qty;not null"`
	PerUserLimit   int          `gorm:"column:per_user_limit;not null;default:1"`
	StartTime      time.Time    `gorm:"column:start_time;not null"`
	EndTime        time.Time    `gorm:"column:end_time;not null"`
	Status         CouponStatus `gorm:"column:status;type:varchar(16);not null;default:'active'"`
	Version        int          `gorm:"column:version;not null;default:0"`
	CreatedAt      time.Time    `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt      time.Time    `gorm:"column:updated_at;autoUpdateTime"`
	DeletedAt      gorm.DeletedAt `gorm:"index"`
}

func (t *CouponTemplate) BeforeCreate(tx *gorm.DB) error {
	if t.ID == uuid.Nil {
		id, err := uuid.NewV7()
		if err != nil {
			return err
		}
		t.ID = id
	}
	if t.RemainingQty == 0 && t.TotalQty > 0 {
		t.RemainingQty = t.TotalQty
	}
	return nil
}

type UserCouponStatus string

const (
	UserCouponStatusUnused  UserCouponStatus = "unused"
	UserCouponStatusUsed    UserCouponStatus = "used"
	UserCouponStatusExpired UserCouponStatus = "expired"
)

// UserCoupon 用户持有的优惠券
type UserCoupon struct {
	ID          uuid.UUID        `gorm:"column:id;primaryKey;type:uuid"`
	UserID      uuid.UUID        `gorm:"column:user_id;type:uuid;index;not null"`
	TemplateID  uuid.UUID        `gorm:"column:template_id;type:uuid;not null"`
	Status      UserCouponStatus `gorm:"column:status;type:varchar(16);not null;default:'unused'"`
	UsedOrderID *uuid.UUID       `gorm:"column:used_order_id;type:uuid"`
	UsedAt      *time.Time       `gorm:"column:used_at"`
	ExpireTime  time.Time        `gorm:"column:expire_time;not null"`
	Version     int              `gorm:"column:version;not null;default:0"`
	CreatedAt   time.Time        `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt   time.Time        `gorm:"column:updated_at;autoUpdateTime"`

	// Preload 用
	Template *CouponTemplate `gorm:"foreignKey:TemplateID;references:ID"`
}

func (uc *UserCoupon) BeforeCreate(tx *gorm.DB) error {
	if uc.ID == uuid.Nil {
		id, err := uuid.NewV7()
		if err != nil {
			return err
		}
		uc.ID = id
	}
	return nil
}
