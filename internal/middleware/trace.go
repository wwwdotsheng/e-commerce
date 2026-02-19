package middleware

import (
	"e-commerce/pkg/clog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"time"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

var (
	meter                = otel.GetMeterProvider().Meter("e-commerce-api")
	httpRequestsTotal, _ = meter.Int64Counter("http_requests_total")
	httpDuration, _      = meter.Float64Histogram("http_request_duration_seconds")
)

func TraceMiddleware(serviceName string) gin.HandlerFunc {
	return otelgin.Middleware(serviceName)
}

func RequestLogMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		route := c.FullPath()
		// 当出现404的情况
		if route == "" {
			route = "unknow"
		}
		query := c.Request.URL.RawQuery

		span := trace.SpanFromContext(c.Request.Context())
		var traceID string
		if span.SpanContext().IsValid() {
			traceID = span.SpanContext().TraceID().String()
		}
		if traceID != "" {
			c.Header("x-trace-id", traceID)
		}

		c.Next()

		ctx := c.Request.Context()
		latency := time.Since(start)
		attrs := metric.WithAttributes(
			attribute.String("method", c.Request.Method),
			attribute.String("route", route),
			attribute.Int("status", c.Writer.Status()),
		)
		httpRequestsTotal.Add(ctx, 1, attrs)
		httpDuration.Record(ctx, latency.Seconds(), attrs)

		clog.L(ctx).Info("HTTP Request",
			zap.Int("status", c.Writer.Status()),
			zap.String("method", c.Request.Method),
			zap.String("path", path),
			zap.String("full-path", route),
			zap.String("query", query),
			zap.String("ip", c.ClientIP()),
			zap.Duration("latency", latency),
			zap.String("user-agent", c.Request.UserAgent()),
			zap.String("errors", c.Errors.ByType(gin.ErrorTypePrivate).String()),
		)
	}
}
