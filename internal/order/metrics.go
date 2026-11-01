package order

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

type OrderCreateStatus string

const (
	orderCreateStatusSuccess OrderCreateStatus = "success"
	orderCreateStatusFail    OrderCreateStatus = "fail"
)

type OrderCreateErrCode string

const (
	orderCreateErrNone              OrderCreateErrCode = "none"
	orderCreateErrProductNotFound   OrderCreateErrCode = "product_not_found"
	orderCreateErrStockInsufficient OrderCreateErrCode = "stock_insufficient"
	orderCreateErrCoupon            OrderCreateErrCode = "coupon_error"
	orderCreateErrTimeoutSchedule   OrderCreateErrCode = "timeout_schedule_failed"
	orderCreateErrInternal          OrderCreateErrCode = "internal"
)

type OrderTimeoutStatus string

const (
	orderTimeoutStatusClosed OrderTimeoutStatus = "closed"
	orderTimeoutStatusFail   OrderTimeoutStatus = "fail"
)

type OrderPaymentStatus string

const (
	orderPaymentStatusSuccess OrderPaymentStatus = "success"
	orderPaymentStatusFail    OrderPaymentStatus = "fail"
)

type OrderPaymentErrCode string

const (
	orderPaymentErrNone                OrderPaymentErrCode = "none"
	orderPaymentErrInsufficientBalance OrderPaymentErrCode = "insufficient_balance"
	orderPaymentErrExpired             OrderPaymentErrCode = "expired"
	orderPaymentErrStatusInvalid       OrderPaymentErrCode = "status_invalid"
	orderPaymentErrNotFound            OrderPaymentErrCode = "not_found"
	orderPaymentErrLedgerConflict      OrderPaymentErrCode = "ledger_conflict"
	orderPaymentErrSellerWallet        OrderPaymentErrCode = "seller_wallet_missing"
	orderPaymentErrInternal            OrderPaymentErrCode = "internal"
)

type Metrics struct {
	OrderCreateCounter  metric.Int64Counter
	OrderTimeoutCounter metric.Int64Counter
	OrderPaymentCounter metric.Int64Counter
}

func NewMetrics(meter metric.Meter) (*Metrics, error) {
	createCounter, err := meter.Int64Counter(
		"order_create_total",
		metric.WithDescription("Total number of order creations"),
	)
	if err != nil {
		return nil, err
	}

	timeoutCounter, err := meter.Int64Counter(
		"order_timeout_total",
		metric.WithDescription("Total number of order timeout closures"),
	)
	if err != nil {
		return nil, err
	}

	paymentCounter, err := meter.Int64Counter(
		"order_payment_total",
		metric.WithDescription("Total number of order payment attempts"),
	)
	if err != nil {
		return nil, err
	}

	return &Metrics{
		OrderCreateCounter:  createCounter,
		OrderTimeoutCounter: timeoutCounter,
		OrderPaymentCounter: paymentCounter,
	}, nil
}

func (m *Metrics) AddOrderCreateTotal(ctx context.Context, status OrderCreateStatus, code OrderCreateErrCode) {
	m.OrderCreateCounter.Add(ctx, 1, metric.WithAttributes(
		attribute.String("status", string(status)),
		attribute.String("code", string(code)),
	))
}

func (m *Metrics) AddOrderTimeoutTotal(ctx context.Context, status OrderTimeoutStatus) {
	m.OrderTimeoutCounter.Add(ctx, 1, metric.WithAttributes(
		attribute.String("status", string(status)),
	))
}

func (m *Metrics) AddOrderPaymentTotal(ctx context.Context, status OrderPaymentStatus, code OrderPaymentErrCode) {
	m.OrderPaymentCounter.Add(ctx, 1, metric.WithAttributes(
		attribute.String("status", string(status)),
		attribute.String("code", string(code)),
	))
}
