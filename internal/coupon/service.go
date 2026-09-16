package coupon

import (
	"context"
	"e-commerce/internal/model"
	"e-commerce/pkg/clog"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type Service struct {
	db      *gorm.DB
	repo    *Repository
	metrics *Metrics
}

func NewService(db *gorm.DB, repo *Repository, metrics *Metrics) *Service {
	return &Service{db: db, repo: repo, metrics: metrics}
}

// CreateTemplate 运营创建优惠券模板
func (s *Service) CreateTemplate(ctx context.Context, param CreateTemplateParam) (*model.CouponTemplate, error) {
	startTime, err := time.Parse("2006-01-02 15:04:05", param.StartTime)
	if err != nil {
		return nil, fmt.Errorf("解析开始时间失败: %w", err)
	}
	endTime, err := time.Parse("2006-01-02 15:04:05", param.EndTime)
	if err != nil {
		return nil, fmt.Errorf("解析结束时间失败: %w", err)
	}
	if !endTime.After(startTime) {
		return nil, fmt.Errorf("结束时间必须晚于开始时间")
	}

	t := &model.CouponTemplate{
		Name:          param.Name,
		Type:          model.CouponType(param.Type),
		DiscountValue: param.DiscountValue,
		DiscountRate:  param.DiscountRate,
		MaxDeduction:  param.MaxDeduction,
		MinAmount:     param.MinAmount,
		TotalQty:      param.TotalQty,
		RemainingQty:  param.TotalQty,
		PerUserLimit:  param.PerUserLimit,
		StartTime:     startTime,
		EndTime:       endTime,
		Status:        model.CouponStatusActive,
	}

	if param.PerUserLimit == 0 {
		t.PerUserLimit = 1
	}

	if err := s.repo.CreateTemplate(ctx, t); err != nil {
		return nil, fmt.Errorf("创建优惠券模板失败: %w", err)
	}
	return t, nil
}

// GrantCoupon 给用户发券（乐观锁扣减模板库存）
func (s *Service) GrantCoupon(ctx context.Context, param GrantCouponParam) (uc *model.UserCoupon, err error) {
	var errCode = couponOpErrInternal
	defer func() {
		if err != nil {
			s.metrics.AddCouponGrantTotal(ctx, couponOpStatusFail, errCode)
		} else {
			s.metrics.AddCouponGrantTotal(ctx, couponOpStatusSuccess, couponOpErrNone)
		}
	}()

	template, err := s.repo.GetTemplate(ctx, param.TemplateID)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	if template.Status != model.CouponStatusActive {
		errCode = couponOpErrNotActive
		return nil, ErrTemplateNotActive
	}
	if now.Before(template.StartTime) || now.After(template.EndTime) {
		errCode = couponOpErrExpired
		return nil, ErrTemplateExpired
	}

	count, err := s.repo.CountUserCouponsByTemplate(ctx, param.UserID, param.TemplateID)
	if err != nil {
		return nil, fmt.Errorf("统计用户券数量失败: %w", err)
	}
	if count >= int64(template.PerUserLimit) {
		errCode = couponOpErrPerUserLimit
		return nil, fmt.Errorf("该优惠券每人限领 %d 张", template.PerUserLimit)
	}

	if err := s.repo.DeductRemainingQty(ctx, param.TemplateID); err != nil {
		if errors.Is(err, ErrCouponOutOfStock) {
			errCode = couponOpErrStockEmpty
		}
		return nil, err
	}

	uc = &model.UserCoupon{
		UserID:     param.UserID,
		TemplateID: param.TemplateID,
		Status:     model.UserCouponStatusUnused,
		ExpireTime: template.EndTime,
	}
	if err = s.repo.CreateUserCoupon(ctx, uc); err != nil {
		return nil, fmt.Errorf("创建用户券失败: %w", err)
	}

	clog.L(ctx).Info("优惠券发放成功",
		zap.String("user_id", param.UserID.String()),
		zap.String("template_id", param.TemplateID.String()),
	)
	return uc, nil
}

// ListUserCoupons 用户查看自己的券
func (s *Service) ListUserCoupons(ctx context.Context, param ListUserCouponsParam) ([]*model.UserCoupon, int64, error) {
	if param.PageNum == 0 {
		param.PageNum = 1
	}
	if param.PageSize == 0 {
		param.PageSize = 10
	}
	return s.repo.ListUserCoupons(ctx, param.UserID, param.PageNum, param.PageSize)
}

// UseCoupon 事务内核销优惠券（由 order service 调用）
func (s *Service) UseCoupon(ctx context.Context, userID uuid.UUID, param UseCouponParam) (float64, error) {
	uc, err := s.repo.GetUserCouponForUpdate(ctx, param.UserCouponID, userID)
	if err != nil {
		return 0, err
	}

	if uc.Status != model.UserCouponStatusUnused {
		return 0, ErrCouponAlreadyUsed
	}
	if time.Now().After(uc.ExpireTime) {
		return 0, ErrCouponAlreadyUsed
	}

	template := uc.Template
	if template == nil {
		return 0, ErrTemplateNotFound
	}
	if template.MinAmount > 0 && param.OrderAmount < template.MinAmount {
		return 0, ErrCouponMinAmountNotMet
	}

	deduction := calcDeduction(template, param.OrderAmount)

	if err := s.repo.UseCouponWithVersion(ctx, uc.ID, userID, param.OrderID, uc.Version); err != nil {
		return 0, err
	}

	return deduction, nil
}

// calcDeduction 计算优惠金额
func calcDeduction(t *model.CouponTemplate, orderAmount float64) float64 {
	switch t.Type {
	case model.CouponTypeFixed:
		return t.DiscountValue
	case model.CouponTypePercentage:
		d := orderAmount * t.DiscountRate
		if t.MaxDeduction > 0 && d > t.MaxDeduction {
			return t.MaxDeduction
		}
		return d
	default:
		return 0
	}
}

// ReturnCoupon 超时退券（由 order mq_handler 超时处理时调用）
func (s *Service) ReturnCoupon(ctx context.Context, userCouponID uuid.UUID) error {
	return s.repo.ReturnCoupon(ctx, userCouponID)
}
