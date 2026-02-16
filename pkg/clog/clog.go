package clog

import (
	"context"
	"e-commerce/internal/config"
	"os"

	"go.opentelemetry.io/otel/trace" // 引入 trace 包
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

type ctxKey struct{}

var loggerKey = ctxKey{}

func Init(config config.LogSection) *zap.Logger {
	encoderConfig := zap.NewProductionEncoderConfig()
	// 关键修改：将时间格式化为 ISO8601，方便 Loki 解析
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	// 建议把 trace_id 等字段固定下来
	encoderConfig.MessageKey = "msg"
	encoderConfig.TimeKey = "ts"

	jsonEncoder := zapcore.NewJSONEncoder(encoderConfig)

	consoleSyncer := zapcore.AddSync(os.Stdout)
	fileSyncer := zapcore.AddSync(&lumberjack.Logger{
		Filename:   config.Filename,
		MaxSize:    config.MaxSize,
		MaxBackups: config.MaxBackups,
		MaxAge:     config.MaxAge,
	})

	core := zapcore.NewTee(
		zapcore.NewCore(jsonEncoder, fileSyncer, zapcore.DebugLevel),
		zapcore.NewCore(jsonEncoder, consoleSyncer, zapcore.InfoLevel),
	)

	return zap.New(core, zap.AddCaller())
}

func Close(l *zap.Logger) {
	if l != nil {
		_ = l.Sync()
	}
}

func WithLogger(ctx context.Context, l *zap.Logger) context.Context {
	return context.WithValue(ctx, loggerKey, l)
}

// L 是最关键的修改点
func L(ctx context.Context) *zap.Logger {
	if ctx == nil {
		return zap.L()
	}

	// 1. 获取基础 Logger
	l, ok := ctx.Value(loggerKey).(*zap.Logger)
	if !ok {
		l = zap.L()
	}

	// 2. 从 Context 中提取 Span 信息
	span := trace.SpanFromContext(ctx)
	if !span.SpanContext().IsValid() {
		return l
	}

	// 3. 将 TraceID 和 SpanID 注入日志字段
	// 这样你每次调用 clog.L(ctx).Info(...) 都会自动带上这些 ID
	return l.With(
		zap.String("trace_id", span.SpanContext().TraceID().String()),
		zap.String("span_id", span.SpanContext().SpanID().String()),
	)
}
