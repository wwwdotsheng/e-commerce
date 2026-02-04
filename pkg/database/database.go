package database

import (
	"context"
	"e-commerce/internal/config"
	"e-commerce/pkg/clog"
	"fmt"

	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func Init(ctx context.Context, config config.DatabaseSection) *gorm.DB {
	logger := clog.L(ctx)

	dsn := fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%d sslmode=%s TimeZone=%s",
		config.Host, config.User, config.Password, config.DBName, config.Port, config.SSLMode, config.TimeZone,
	)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		logger.Error("连接数据库失败", zap.Error(err))
		panic("连接数据库失败")
	}

	logger.Info("连接数据库成功")

	return db
}
