package coupon

import (
	"context"
	"e-commerce/internal/model"
	"e-commerce/pkg/clog"
	"e-commerce/pkg/errno"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type Service struct {
	db       *gorm.DB
	repo     *Repository
	metrics  *Metrics
	location *time.Location
}

// NewService 的 location 用于解释不带时区的历史时间格式；为空时回退到进程本地时区。
func NewService(db *gorm.DB, repo *Repository, metrics *Metrics, location *time.Location) *Service {
	return &Service{db: db, repo: repo, metrics: metrics, location: location}
}

// CreateTemplate 运营创建优惠券模板。
// 时间字段优先按带时区的 RFC3339 解析，兼容历史无时区格式；非法输入返回 A00001。
func (s *Service) CreateTemplate(ctx context.Context, param CreateTemplateParam) (*model.CouponTemplate, error) {
	startTime, err := parseCouponTime(param.StartTime, s.location)
	if err != nil {
		return nil, err
	}
	endTime, err := parseCouponTime(param.EndTime, s.location)
	if err != nil {
		return nil, err
	}
	if !endTime.After(startTime) {
		return nil, errno.ErrInvalidParam.WithRaw(errors.New("end_time must be after start_time"))
	}
	// 金额和折扣率统一先按列宽取两位小数，再校验语义。
	param.DiscountValue = param.DiscountValue.Round(2)
	param.DiscountRate = param.DiscountRate.Round(2)
	param.MaxDeduction = param.MaxDeduction.Round(2)
	param.MinAmount = param.MinAmount.Round(2)
	if param.DiscountValue.IsNegative() || param.MaxDeduction.IsNegative() || param.MinAmount.IsNegative() {
		return nil, errno.ErrInvalidParam.WithRaw(errors.New("coupon amounts must be non-negative"))
	}
	switch model.CouponType(param.Type) {
	case model.CouponTypeFixed:
		if !param.DiscountValue.IsPositive() {
			return nil, errno.ErrInvalidParam.WithRaw(errors.New("discount_value must be positive"))
		}
	case model.CouponTypePercentage:
		if !param.DiscountRate.IsPositive() || param.DiscountRate.GreaterThan(decimal.NewFromInt(1)) {
			return nil, errno.ErrInvalidParam.WithRaw(errors.New("discount_rate must be in (0,1]"))
		}
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
func (s *Service) UseCoupon(ctx context.Context, userID uuid.UUID, param UseCouponParam) (decimal.Decimal, error) {
	uc, err := s.repo.GetUserCouponForUpdate(ctx, param.UserCouponID, userID)
	if err != nil {
		return decimal.Zero, err
	}

	if uc.Status != model.UserCouponStatusUnused {
		return decimal.Zero, ErrCouponAlreadyUsed
	}
	if time.Now().After(uc.ExpireTime) {
		return decimal.Zero, ErrCouponAlreadyUsed
	}

	template := uc.Template
	if template == nil {
		return decimal.Zero, ErrTemplateNotFound
	}
	if template.MinAmount.IsPositive() && param.OrderAmount.LessThan(template.MinAmount) {
		return decimal.Zero, ErrCouponMinAmountNotMet
	}

	deduction := calcDeduction(template, param.OrderAmount)

	if err := s.repo.UseCouponWithVersion(ctx, uc.ID, userID, param.OrderID, uc.Version); err != nil {
		return decimal.Zero, err
	}

	return deduction.Round(2), nil
}

// calcDeduction 计算优惠金额（金额使用 decimal，避免浮点误差）
func calcDeduction(t *model.CouponTemplate, orderAmount decimal.Decimal) decimal.Decimal {
	switch t.Type {
	case model.CouponTypeFixed:
		return t.DiscountValue
	case model.CouponTypePercentage:
		d := orderAmount.Mul(t.DiscountRate)
		if t.MaxDeduction.IsPositive() && d.GreaterThan(t.MaxDeduction) {
			return t.MaxDeduction
		}
		return d
	default:
		return decimal.Zero
	}
}

// ReturnCoupon 超时退券（由 order mq_handler 超时处理时调用）
func (s *Service) ReturnCoupon(ctx context.Context, userCouponID uuid.UUID) error {
	return s.repo.ReturnCoupon(ctx, userCouponID)
}
