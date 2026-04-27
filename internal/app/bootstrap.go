package app

import (
	"context"
	"e-commerce/internal/auth"
	"e-commerce/internal/config"
	"e-commerce/internal/middleware"
	"e-commerce/internal/model"
	"e-commerce/internal/user"
	"e-commerce/internal/wallet"
	"e-commerce/pkg/clog"
	"e-commerce/pkg/database"
	"e-commerce/pkg/redis"
	"encoding/json"
	"fmt"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
	"go.uber.org/zap"
)

func Bootstrap() (context.Context, func(), *config.AppConfig, error) {
	ctx, cancel := context.WithCancel(context.Background())

	conf, err := config.Init()
	if err != nil {
		cancel()
		return ctx, nil, nil, fmt.Errorf("加载配置失败：%w", err)
	}
	if !conf.IsProd() {
		data, err := json.MarshalIndent(conf, "", "  ")
		if err != nil {
			cancel()
			return ctx, nil, nil, fmt.Errorf("无法初始化配置%w", err)
		}
		fmt.Println("================ 当前系统配置 ================")
		fmt.Println(string(data))
		fmt.Println("============================================")
	}

	otelShutdown, err := setupOTelSDK(ctx, conf.Otel)
	if err != nil {
		fmt.Printf("otel sdk初始化失败：%v", err)
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

func SetupRouter(conf *config.AppConfig, authSvc *auth.Service, userSvc *user.Service, walletSvc *wallet.Service, logger *zap.Logger, mp *metric.MeterProvider) (*gin.Engine, error) {
	var r *gin.Engine
	if conf.IsDev() {
		r = gin.Default()
	} else {
		gin.SetMode(gin.ReleaseMode)
		r = gin.New()
	}
	err := r.SetTrustedProxies([]string{})
	if err != nil {
		return nil, err
	}

	r.Use(middleware.InjectConfig(conf))
	r.Use(middleware.InjectLoggerMiddleware(logger))
	r.Use(middleware.TraceMiddleware("e-commerce"))
	r.Use(middleware.RequestLogMiddleware())

	v1 := r.Group("/api/v1")
	{
		h := auth.NewHandler(authSvc)
		accessTokenAuthMiddleware := middleware.AccessTokenAuth(authSvc)
		refreshTokenAuthMiddleware := middleware.RefreshTokenAuth(authSvc)

		v1.POST("/auth/login", h.Login)

		authGroup := v1.Group("/auth").Use(accessTokenAuthMiddleware)
		authGroup.POST("/fetch-refresh-token", h.FetchRefreshToken)
		authGroup.POST("/logout", h.Logout)

		v1.Group("/auth").Use(refreshTokenAuthMiddleware).
			POST("/fetch-access-token", h.FetchAccessToken)

		userH := user.NewHandler(userSvc, authSvc)
		v1.POST("/user/register", userH.Register)
		v1.Group("/user").Use(accessTokenAuthMiddleware).GET("/good")

		walletH := wallet.NewHandler(walletSvc)
		walletGroup := v1.Group("/wallet").Use(accessTokenAuthMiddleware)
		walletGroup.POST("/deposit", walletH.Deposit)
	}
	return r, nil
}

func Run(ctx context.Context, config config.AppConfig) {
	logger := clog.L(ctx)
	mp := otel.GetMeterProvider()

	db := database.Init(ctx, logger, database.Config{
		Host:            config.Database.Host,
		Port:            config.Database.Port,
		User:            config.Database.User,
		Password:        config.Database.Password,
		DBName:          config.Database.DBName,
		SSLMode:         config.Database.SSLMode,
		MaxIdleConns:    config.Database.MaxIdleConns,
		MaxOpenConns:    config.Database.MaxOpenConns,
		ConnMaxLifetime: config.Database.ConnMaxLifetime,
		TimeZone:        config.Database.TimeZone,
		LogLevel:        config.Database.LogLevel,
	})
	if err := db.AutoMigrate(&model.User{}, &model.UserWallet{}, &model.WalletLog{}); err != nil {
		logger.Fatal("数据库AutoMigrate失败")
	}
	rdb := redis.Init(ctx, redis.Config{
		Host:     config.Redis.Host,
		Port:     config.Redis.Port,
		Password: config.Redis.Password,
		DB:       config.Redis.DB,
		PoolSize: config.Redis.PoolSize,
	})

	walletRepo := wallet.NewRepository(db, rdb)
	walletSvc := wallet.NewService(walletRepo)

	authRepo := auth.NewRepository(db, rdb, &config.Auth)
	authSvc := auth.NewService(authRepo, &config.Auth)

	userMeter := mp.Meter("user_api")
	userMetrics, err := user.NewMetrics(userMeter)
	if err != nil {
		logger.Error("metrics初始化失败", zap.Error(err))
	}
	userRepo := user.NewRepository(db)
	userSvc := user.NewService(userRepo, walletRepo, userMetrics)

	r, err := SetupRouter(&config, authSvc, userSvc, walletSvc, logger, &mp)
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
