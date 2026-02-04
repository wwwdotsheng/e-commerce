package clog

import (
	"context"
	"e-commerce/internal/config"
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

type ctxKey struct{}

var loggerKey = ctxKey{}

func Init(config config.LogSection) *zap.Logger {
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	jsonEncoder := zapcore.NewJSONEncoder(encoderConfig)

	//
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

func L(ctx context.Context) *zap.Logger {
	if ctx == nil {
		return zap.L()
	}
	if l, ok := ctx.Value(loggerKey).(*zap.Logger); ok {
		return l
	}
	return zap.L()
}
