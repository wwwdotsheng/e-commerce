package app

import (
	"context"
	"e-commerce/internal/auth"
	"e-commerce/internal/config"
	"e-commerce/internal/middleware"
	"e-commerce/internal/user"
	"e-commerce/pkg/clog"
	"e-commerce/pkg/database"
	"e-commerce/pkg/redis"
	"fmt"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func Bootstrap(configPath string) (context.Context, func(), *config.AppConfig, error) {
	ctx, cancel := context.WithCancel(context.Background())

	conf, err := config.Init()
	if err != nil {
		cancel()
		return ctx, nil, nil, fmt.Errorf("加载配置失败：%w", err)
	}

	// --- 日志初始化
	logger := clog.Init(conf.Log)
	ctx = clog.WithLogger(ctx, logger)
	logger.Info("日志系统初始化完毕")

	stop := func() {
		cancel()
		clog.Close(logger)
	}

	return ctx, stop, conf, nil
}

func Run(ctx context.Context, config config.AppConfig) {
	var err error

	logger := clog.L(ctx)

	// --- 数据库初始化
	db := database.Init(ctx, config.Database)
	err = db.AutoMigrate(&user.User{})
	if err != nil {
		logger.Error("数据库AutoMigrate失败", zap.Error(err))
		panic("数据库AutoMigrate失败")
	}

	// --- Redis初始化
	rdb := redis.Init(ctx, config.Redis)

	// --- 路由初始化
	authRepo := auth.NewRepository(rdb, &config.Auth)
	authSvc := auth.NewService(authRepo, &config.Auth)
	authMiddleware := middleware.Auth(authSvc)

	r := gin.Default()
	err = r.SetTrustedProxies([]string{})
	if err != nil {
		logger.Error("执行SetTrustedProxies产生错误", zap.Error(err))
		panic("执行SetTrustedProxies产生错误")
	}

	r.Use(middleware.TraceRequest(ctx))

	api := r.Group("/api")

	v1 := api.Group("/v1")
	{
		user.RegisterRouters(v1, authSvc, authMiddleware, db)
	}

	// --- 服务启动
	addr := fmt.Sprintf("0.0.0.0:%d", config.App.Port)
	if config.App.SSL {
		err = r.RunTLS(addr, config.App.SSLCrtPath, config.App.SSLKeyPath)
	} else {
		err = r.Run(addr)
	}

	if err != nil {
		logger.Error("服务器启动错误", zap.Error(err))
		panic("服务器启动错误")
	}
}
