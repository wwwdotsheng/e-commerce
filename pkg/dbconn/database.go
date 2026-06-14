package dbconn

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	dbLogger "gorm.io/gorm/logger"
	"gorm.io/plugin/opentelemetry/tracing"
)

type Logger interface {
	Info(msg string, field ...zap.Field)
	Error(msg string, field ...zap.Field)
}

type Config struct {
	Host            string
	Port            int
	User            string
	Password        string
	DBName          string
	SSLMode         string
	MaxIdleConns    int
	MaxOpenConns    int
	ConnMaxLifetime int
	TimeZone        string
	LogLevel        string
}

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

func Init(ctx context.Context, logger Logger, config Config) (*gorm.DB, error) {
	ctx, span := otel.Tracer("database").Start(ctx, "Connecting")
	defer span.End()

	dsn := fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%d sslmode=%s TimeZone=%s",
		config.Host, config.User, config.Password, config.DBName, config.Port, config.SSLMode, config.TimeZone,
	)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: dbLogger.Default.LogMode(parseLevel(config.LogLevel)),
	})
	if err != nil {
		return nil, fmt.Errorf("连接数据库失败: %w", err)
	}

	if err := db.Use(tracing.NewPlugin()); err != nil {
		return nil, fmt.Errorf("注册 tracing 插件失败: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("获取 sqlDB 失败: %w", err)
	}

	sqlDB.SetMaxIdleConns(config.MaxIdleConns)
	sqlDB.SetMaxOpenConns(config.MaxOpenConns)
	sqlDB.SetConnMaxLifetime(time.Duration(config.ConnMaxLifetime) * time.Minute)

	logger.Info("连接数据库成功")

	return db, nil
}
