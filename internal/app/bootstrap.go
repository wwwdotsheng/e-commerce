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

func Run(ctx context.Context, config config.AppConfig) {
	var err error

	mp := otel.GetMeterProvider()
	logger := clog.L(ctx)

	// --- 数据库初始化
	db := database.Init(ctx, config.Database)
	err = db.AutoMigrate(&model.User{})
	if err != nil {
		logger.Error("数据库AutoMigrate失败", zap.Error(err))
		panic("数据库AutoMigrate失败")
	}

	// --- Redis初始化
	rdb := redis.Init(ctx, config.Redis)

	// --- 路由初始化
	authRepo := auth.NewRepository(db, rdb, &config.Auth)
	authSvc := auth.NewService(authRepo, &config.Auth)
	accessTokenAuthMiddleware := middleware.AccessTokenAuth(authSvc)
	refreshTokenAuthMiddleware := middleware.RefreshTokenAuth(authSvc)

	r := gin.Default()
	err = r.SetTrustedProxies([]string{})
	if err != nil {
		logger.Error("执行SetTrustedProxies产生错误", zap.Error(err))
		panic("执行SetTrustedProxies产生错误")
	}

	r.Use(middleware.InjectLoggerMiddleware(logger))
	r.Use(middleware.TraceMiddleware("e-commerce"))
	r.Use(middleware.RequestLogMiddleware())

	api := r.Group("/api")

	v1 := api.Group("/v1")
	{
		h := auth.NewHandler(authSvc)
		authGroup0 := v1.Group("/auth")
		{
			authGroup0.POST("/login", h.Login)
		}
		authGroup1 := v1.Group("/auth")
		authGroup1.Use(accessTokenAuthMiddleware)
		{
			authGroup1.POST("/fetch-refresh-token", h.FetchRefreshToken)
			authGroup1.POST("/logout", h.Logout)
		}
		authGroup2 := v1.Group("/auth")
		authGroup2.Use(refreshTokenAuthMiddleware)
		{
			authGroup2.POST("/fetch-access-token", h.FetchAccessToken)
		}
	}
	{
		userMeter := mp.Meter("user_api")
		metrics, err := user.NewMetrics(userMeter)
		// TODO 错误处理如何编写？直接panic？
		if err != nil {
			panic(err)
		}

		repo := user.NewRepository(db)
		svc := user.NewService(repo, metrics)
		h := user.NewHandler(svc, authSvc)
		tg := v1.Group("/user")
		{
			tg.POST("/register", h.Register)
		}
		tg.Use(accessTokenAuthMiddleware)
		{
			tg.GET("/good")
		}
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
