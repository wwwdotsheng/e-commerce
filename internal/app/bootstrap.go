package app

import (
	"context"
	"e-commerce/internal/auth"
	"e-commerce/internal/config"
	"e-commerce/internal/middleware"
	"e-commerce/internal/model"
	"e-commerce/internal/user"
	"e-commerce/pkg/clog"
	"e-commerce/pkg/database"
	"e-commerce/pkg/redis"
	"fmt"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
	"go.uber.org/zap"
)

func Bootstrap(configPath string) (context.Context, func(), *config.AppConfig, error) {
	ctx, cancel := context.WithCancel(context.Background())

	conf, err := config.Init()
	if err != nil {
		cancel()
		return ctx, nil, nil, fmt.Errorf("加载配置失败：%w", err)
	}

	otelShutdown, err := setupOTelSDK(ctx)
	if err != nil {
		cancel()
		return ctx, nil, nil, fmt.Errorf("otel sdk初始化失败：%w", err)
	}

	// --- 日志初始化
	logger := clog.Init(conf.Log)
	ctx = clog.WithLogger(ctx, logger)

	stop := func() {
		if err = otelShutdown(context.Background()); err != nil {
			fmt.Printf("otel shutdown error: %v\n", err)
		}
		clog.Close(logger)
		cancel()
	}

	return ctx, stop, conf, nil
}

func SetupRouter(authSvc *auth.Service, userSvc *user.Service, logger *zap.Logger, mp *metric.MeterProvider) (*gin.Engine, error) {

	r := gin.Default()
	err := r.SetTrustedProxies([]string{})
	if err != nil {
		return nil, err
	}

	r.Use(middleware.InjectLoggerMiddleware(logger))
	r.Use(middleware.TraceMiddleware("e-commerce"))
	r.Use(middleware.RequestLogMiddleware())

	v1 := r.Group("/api/v1")
	{
		h := auth.NewHandler(authSvc)
		accessTokenAuthMiddleware := middleware.AccessTokenAuth(authSvc)
		refreshTokenAuthMiddleware := middleware.RefreshTokenAuth(authSvc)

		v1.POST("/auth/login", h.Login)

		authGroup := r.Group("/auth").Use(accessTokenAuthMiddleware)
		authGroup.POST("/fetch-refresh-token", h.FetchRefreshToken)
		authGroup.POST("/logout", h.Logout)

		r.Group("/auth").Use(refreshTokenAuthMiddleware).POST("/fetch-access-token", h.FetchAccessToken)

		userH := user.NewHandler(userSvc, authSvc)
		v1.POST("/user/register", userH.Register)
		v1.Group("/user").Use(accessTokenAuthMiddleware).GET("/good")
	}
	return r, nil
}

func Run(ctx context.Context, config config.AppConfig) {
	var err error

	logger := clog.L(ctx)
	mp := otel.GetMeterProvider()

	db := database.Init(ctx, config.Database)
	if err := db.AutoMigrate(&model.User{}); err != nil {
		logger.Fatal("数据库AutoMigrate失败")
	}
	rdb := redis.Init(ctx, config.Redis)

	authRepo := auth.NewRepository(db, rdb, &config.Auth)
	authSvc := auth.NewService(authRepo, &config.Auth)

	userMeter := mp.Meter("user_api")
	userMetrics, err := user.NewMetrics(userMeter)
	if err != nil {
		logger.Fatal("metrics初始化失败", zap.Error(err))
	}
	userRepo := user.NewRepository(db)
	userSvc := user.NewService(userRepo, userMetrics)

	r, err := SetupRouter(authSvc, userSvc, logger, &mp)
	if err != nil {
		logger.Fatal("初始化路由失败", zap.Error(err))
	}

	// --- 服务启动
	addr := fmt.Sprintf("0.0.0.0:%d", config.App.Port)
	if config.App.SSL {
		if err := r.RunTLS(addr, config.App.SSLCrtPath, config.App.SSLKeyPath); err != nil {
			logger.Fatal("TLS服务器启动失败", zap.Error(err))
		}
	} else {
		if err := r.Run(addr); err != nil {
			logger.Fatal("服务器启动失败", zap.Error(err))
		}

	}
}
