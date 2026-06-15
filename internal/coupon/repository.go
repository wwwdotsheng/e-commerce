package coupon

import (
	"context"
	"e-commerce/internal/model"
	"e-commerce/internal/pkg/database"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrTemplateNotFound      = errors.New("coupon template not found")
	ErrCouponOutOfStock      = errors.New("coupon template out of stock")
	ErrCouponAlreadyUsed     = errors.New("coupon already used or expired")
	ErrCouponNotOwned        = errors.New("coupon does not belong to this user")
	ErrTemplateNotActive     = errors.New("coupon template is not active")
	ErrTemplateExpired       = errors.New("coupon template has expired")
	ErrCouponMinAmountNotMet = errors.New("order amount does not meet coupon minimum")
)

type Repository struct {
	*database.BaseRepo
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{BaseRepo: database.NewBaseRepo(db)}
}

// ========== Template ==========

func (r *Repository) CreateTemplate(ctx context.Context, t *model.CouponTemplate) error {
	return r.GetDB(ctx).Create(t).Error
}

func (r *Repository) GetTemplate(ctx context.Context, id uuid.UUID) (*model.CouponTemplate, error) {
	var t model.CouponTemplate
	err := r.GetDB(ctx).Where("id = ?", id).First(&t).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrTemplateNotFound
	}
	return &t, err
}

// DeductRemainingQty 乐观锁扣减模板库存
func (r *Repository) DeductRemainingQty(ctx context.Context, templateID uuid.UUID) error {
	result := r.GetDB(ctx).Model(&model.CouponTemplate{}).
		Where("id = ? AND remaining_qty > 0 AND status = ? AND NOW() BETWEEN start_time AND end_time",
			templateID, model.CouponStatusActive).
		Where("remaining_qty > 0").
		Updates(map[string]interface{}{
			"remaining_qty": gorm.Expr("remaining_qty - 1"),
			"version":       gorm.Expr("version + 1"),
			"updated_at":    time.Now(),
		})

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrCouponOutOfStock
	}
	return nil
}

// ========== UserCoupon ==========

func (r *Repository) CreateUserCoupon(ctx context.Context, uc *model.UserCoupon) error {
	return r.GetDB(ctx).Create(uc).Error
}

// CountUserCouponsByTemplate 统计用户已领取的某模板券数量
func (r *Repository) CountUserCouponsByTemplate(ctx context.Context, userID, templateID uuid.UUID) (int64, error) {
	var count int64
	err := r.GetDB(ctx).Model(&model.UserCoupon{}).
		Where("user_id = ? AND template_id = ?", userID, templateID).
		Count(&count).Error
	return count, err
}

// ListUserCoupons 用户查看自己的优惠券
func (r *Repository) ListUserCoupons(ctx context.Context, userID uuid.UUID, pageNum, pageSize int) ([]*model.UserCoupon, int64, error) {
	var coupons []*model.UserCoupon
	var total int64

	baseQuery := r.GetDB(ctx).Model(&model.UserCoupon{}).
		Where("user_id = ?", userID).
		Preload("Template")

	if err := baseQuery.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := baseQuery.Session(&gorm.Session{}).
		Offset((pageNum - 1) * pageSize).
		Limit(pageSize).
		Order("created_at DESC").
		Find(&coupons).Error

	return coupons, total, err
}

// GetUserCouponForUpdate 获取用户券（带行锁+预载模板，事务内使用）
func (r *Repository) GetUserCouponForUpdate(ctx context.Context, id, userID uuid.UUID) (*model.UserCoupon, error) {
	var uc model.UserCoupon
	err := r.GetDB(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Preload("Template").
		Where("id = ? AND user_id = ?", id, userID).
		First(&uc).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrCouponNotOwned
	}
	return &uc, err
}

// UseCouponWithVersion 乐观锁核销优惠券（传入当前 version）
func (r *Repository) UseCouponWithVersion(ctx context.Context, id, userID uuid.UUID, orderID uuid.UUID, currentVersion int) error {
	result := r.GetDB(ctx).Model(&model.UserCoupon{}).
		Where("id = ? AND user_id = ? AND status = ? AND version = ? AND expire_time > ?",
			id, userID, model.UserCouponStatusUnused, currentVersion, time.Now()).
		Updates(map[string]interface{}{
			"status":        model.UserCouponStatusUsed,
			"used_order_id": orderID,
			"used_at":       time.Now(),
			"version":       gorm.Expr("version + 1"),
			"updated_at":    time.Now(),
		})

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrCouponAlreadyUsed
	}
	return nil
}

// ReturnCoupon 超时退券
func (r *Repository) ReturnCoupon(ctx context.Context, userCouponID uuid.UUID) error {
	result := r.GetDB(ctx).Model(&model.UserCoupon{}).
		Where("id = ? AND status = ?", userCouponID, model.UserCouponStatusUsed).
		Updates(map[string]interface{}{
			"status":        model.UserCouponStatusUnused,
			"used_order_id": nil,
			"used_at":       nil,
			"version":       gorm.Expr("version + 1"),
			"updated_at":    time.Now(),
		})

	if result.Error != nil {
		return result.Error
	}
	// 退券可能 RowsAffected == 0（券已被其他操作改变），不是硬错误
	return nil
}
