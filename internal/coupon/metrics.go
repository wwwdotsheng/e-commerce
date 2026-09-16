package coupon

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

type CouponOpStatus string

const (
	couponOpStatusSuccess CouponOpStatus = "success"
	couponOpStatusFail    CouponOpStatus = "fail"
)

type CouponOpErrCode string

const (
	couponOpErrNone          CouponOpErrCode = "none"
	couponOpErrNotActive     CouponOpErrCode = "template_not_active"
	couponOpErrExpired       CouponOpErrCode = "template_expired"
	couponOpErrStockEmpty    CouponOpErrCode = "stock_empty"
	couponOpErrPerUserLimit  CouponOpErrCode = "per_user_limit"
	couponOpErrInternal      CouponOpErrCode = "internal"
)

type Metrics struct {
	CouponGrantCounter metric.Int64Counter
	CouponUseCounter   metric.Int64Counter
}

func NewMetrics(meter metric.Meter) (*Metrics, error) {
	grantCounter, err := meter.Int64Counter(
		"coupon_grant_total",
		metric.WithDescription("Total number of coupon grants"),
	)
	if err != nil {
		return nil, err
	}

	useCounter, err := meter.Int64Counter(
		"coupon_use_total",
		metric.WithDescription("Total number of coupon redemptions"),
	)
	if err != nil {
		return nil, err
	}

	return &Metrics{
		CouponGrantCounter: grantCounter,
		CouponUseCounter:   useCounter,
	}, nil
}

func (m *Metrics) AddCouponGrantTotal(ctx context.Context, status CouponOpStatus, code CouponOpErrCode) {
	m.CouponGrantCounter.Add(ctx, 1, metric.WithAttributes(
		attribute.String("status", string(status)),
		attribute.String("code", string(code)),
	))
}

func (m *Metrics) AddCouponUseTotal(ctx context.Context, status CouponOpStatus, code CouponOpErrCode) {
	m.CouponUseCounter.Add(ctx, 1, metric.WithAttributes(
		attribute.String("status", string(status)),
		attribute.String("code", string(code)),
	))
}
