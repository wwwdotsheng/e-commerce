package app

import (
	"context"
	"e-commerce/internal/config"
	"errors"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.39.0"
)

// setupOTelSDK 初始化 OpenTelemetry 的管道。
// 如果没有返回错误, 用户需要确保在之后调用返回的 shutdown 方法进行清理。
func setupOTelSDK(ctx context.Context, conf config.OtelSection) (func(context.Context) error, error) {
	var shutdownFuncs []func(context.Context) error
	var err error

	// shutdown 会调用所有注册的清理函数。
	// 所有返回的错误都会被合并到一起。
	// 每个注册的清理函数仅会被调用一次。
	shutdown := func(ctx context.Context) error {
		var err error
		for _, fn := range shutdownFuncs {
			err = errors.Join(err, fn(ctx))
		}
		shutdownFuncs = nil
		return err
	}

	// handleErr 用户调用 shutdown 并合并返回的错误。
	handleErr := func(inErr error) {
		err = errors.Join(inErr, shutdown(ctx))
	}

	// 设置上下文传播器（用于跨服务传递追踪信息）。
	prop := newPropagator()
	otel.SetTextMapPropagator(prop)

	// 初始化 trace 提供者。
	tracerProvider, err := newTracerProvider(ctx, conf)
	if err != nil {
		handleErr(err)
		return shutdown, err
	}
	shutdownFuncs = append(shutdownFuncs, tracerProvider.Shutdown)
	otel.SetTracerProvider(tracerProvider)

	// 初始化 Meter 提供者。
	meterProvider, err := newMeterProvider(ctx, conf)
	if err != nil {
		handleErr(err)
		return shutdown, err
	}
	shutdownFuncs = append(shutdownFuncs, meterProvider.Shutdown)
	otel.SetMeterProvider(meterProvider)

	return shutdown, err
}

func newPropagator() propagation.TextMapPropagator {
	return propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)
}

func newTracerProvider(ctx context.Context, conf config.OtelSection) (*trace.TracerProvider, error) {
	if !conf.Enabled {
		return trace.NewTracerProvider(), nil
	}

	traceExporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(conf.Endpoint.Trace),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		return nil, err
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String(conf.ServiceName),
		),
	)
	if err != nil {
		return nil, err
	}

	tracerProvider := trace.NewTracerProvider(
		trace.WithBatcher(traceExporter),
		trace.WithResource(res),
	)

	return tracerProvider, nil
}

func newMeterProvider(ctx context.Context, conf config.OtelSection) (*metric.MeterProvider, error) {
	if !conf.Enabled {
		return metric.NewMeterProvider(), nil
	}

	metricExporter, err := otlpmetrichttp.New(
		ctx,
		otlpmetrichttp.WithEndpoint(conf.Endpoint.Metric),
		otlpmetrichttp.WithInsecure(),
		otlpmetrichttp.WithURLPath("/api/v1/otlp/v1/metrics"),
	)
	if err != nil {
		return nil, err
	}

	meterProvider := metric.NewMeterProvider(
		metric.WithReader(metric.NewPeriodicReader(metricExporter,
			metric.WithInterval(3*time.Second))),
	)

	return meterProvider, nil
}
