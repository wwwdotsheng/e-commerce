package user

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

type RegStatus string

const (
	MetRegStatusSuccess RegStatus = "success"
	MetRegStatusFail    RegStatus = "fail"
)

type RegErrCode string

const (
	MetErrCodeNone            RegErrCode = "none"
	MetErrCodeUserRegistered  RegErrCode = "username_registered"
	MetErrCodeEmailRegistered RegErrCode = "email_registered"
	MetErrCodeInternal        RegErrCode = "internal_error"
)

type Metrics struct {
	RegCounter metric.Int64Counter
}

func NewMetrics(meter metric.Meter) (*Metrics, error) {
	regCounter, err := meter.Int64Counter(
		"user_registration_total",
		metric.WithDescription("Total number of user registrations"),
	)
	if err != nil {
		return nil, err
	}
	return &Metrics{RegCounter: regCounter}, nil
}

func (metrics *Metrics) AddUserRegistrationTotal(ctx context.Context, status RegStatus, code RegErrCode) {
	metrics.RegCounter.Add(ctx, 1,
		metric.WithAttributes(
			attribute.String("status", string(status)),
			attribute.String("code", string(code)),
		),
	)
}
