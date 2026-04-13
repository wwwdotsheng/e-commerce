package database

import (
	"context"
	"e-commerce/internal/config"
	"e-commerce/pkg/clog"
	"fmt"
	"strings"

	"go.opentelemetry.io/otel"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	dbLogger "gorm.io/gorm/logger"
	"gorm.io/plugin/opentelemetry/tracing"
)

func parseLevel(l string) dbLogger.LogLevel {
	switch strings.ToLower(l) {
	case "silent":
		return dbLogger.Silent
	case "error":
		return dbLogger.Error
	case "warn":
		return dbLogger.Warn
	case "info":
		return dbLogger.Info
	default:
		return dbLogger.Info
	}
}

func Init(ctx context.Context, config config.DatabaseSection) *gorm.DB {
	ctx, span := otel.Tracer("database").Start(ctx, "Connecting")
	logger := clog.L(ctx)

	dsn := fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%d sslmode=%s TimeZone=%s",
		config.Host, config.User, config.Password, config.DBName, config.Port, config.SSLMode, config.TimeZone,
	)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: dbLogger.Default.LogMode(parseLevel(config.LogLevel)),
	})
	if err != nil {
		logger.Error("连接数据库失败", zap.Error(err))
		panic("连接数据库失败")
	}
	if err := db.Use(tracing.NewPlugin()); err != nil {
		panic(err)
	}

	logger.Info("连接数据库成功")
	span.End()

	return db
}
