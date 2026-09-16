package wallet

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

type DepositStatus string

const (
	depositStatusSuccess DepositStatus = "success"
	depositStatusFail    DepositStatus = "fail"
)

type DepositErrCode string

const (
	depositErrNone     DepositErrCode = "none"
	depositErrInvalid  DepositErrCode = "invalid_amount"
	depositErrInternal DepositErrCode = "internal"
)

type Metrics struct {
	DepositCounter metric.Int64Counter
}

func NewMetrics(meter metric.Meter) (*Metrics, error) {
	depositCounter, err := meter.Int64Counter(
		"deposit_total",
		metric.WithDescription("Total number of wallet deposits"),
	)
	if err != nil {
		return nil, err
	}
	return &Metrics{DepositCounter: depositCounter}, nil
}

func (m *Metrics) AddDepositTotal(ctx context.Context, status DepositStatus, code DepositErrCode) {
	m.DepositCounter.Add(ctx, 1, metric.WithAttributes(
		attribute.String("status", string(status)),
		attribute.String("code", string(code)),
	))
}
