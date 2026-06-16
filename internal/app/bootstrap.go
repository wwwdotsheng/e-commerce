package app

import (
	"context"
	"e-commerce/internal/auth"
	"e-commerce/internal/config"
	"e-commerce/internal/coupon"
	"e-commerce/internal/middleware"
	"e-commerce/internal/model"
	"e-commerce/internal/order"
	"e-commerce/internal/product"
	"e-commerce/internal/user"
	"e-commerce/internal/wallet"
	"e-commerce/pkg/clog"
	"e-commerce/pkg/dbconn"
	"e-commerce/pkg/mq"
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
		fmt.Printf("otel sdk初始化失败：%v\n", err)
	}

	// --- 日志初始化
	logger := clog.Init(clog.Config{
		ConsoleLevel: conf.Log.ConsoleLevel,
		FileLevel:    conf.Log.FileLevel,
		Filename:     conf.Log.Filename,
		MaxSize:      conf.Log.MaxSize,
		MaxBackups:   conf.Log.MaxBackups,
		MaxAge:       conf.Log.MaxAge,
	})
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

func SetupRouter(
	conf *config.AppConfig,
	authSvc *auth.Service,
	userSvc *user.Service,
	walletSvc *wallet.Service,
	productSvc *product.Service,
	orderSvc *order.Service,
	couponH *coupon.Handler,
	logger *zap.Logger,
	mp *metric.MeterProvider,
) (*gin.Engine, error) {
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

	// 健康检查（不限流）
	r.GET("/health", HealthHandler)
	r.GET("/ready", ReadyHandler)

	r.GET("/swagger/doc.yaml", swaggerDoc)
	r.GET("/swagger", swaggerUI)

	v1 := r.Group("/api/v1")
	{
		h := auth.NewHandler(authSvc)
		accessTokenAuthMiddleware := middleware.AccessTokenAuth(authSvc)
		refreshTokenAuthMiddleware := middleware.RefreshTokenAuth(authSvc)

		// 登录接口限流
		loginGroup := v1.Group("")
		loginGroup.Use(middleware.RateLimitMiddleware(5, 10))
		loginGroup.POST("/auth/login", h.Login)

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

		productH := product.NewHandler(productSvc)
		productGroup := v1.Group("/product").Use(accessTokenAuthMiddleware)
		productGroup.POST("/create", productH.CreateProduct)
		productGroup.GET("/list", productH.ListProducts)
		productGroup.GET("/:id", productH.GetProduct)
		productGroup.PATCH("/:id", productH.UpdateProductProperty)
		productGroup.POST("/:id/status", productH.UpdateProductStatus)
		productGroup.DELETE("/:id", productH.DeleteProduct)

		orderH := order.NewHandler(orderSvc)
		orderGroup := v1.Group("/order").Use(accessTokenAuthMiddleware)
		orderGroup.POST("/create", orderH.CreateOrder)
		orderGroup.GET("/list", orderH.ListOrders)

		couponGroup := v1.Group("/coupon").Use(accessTokenAuthMiddleware)
		couponGroup.POST("/template", couponH.CreateTemplate)
		couponGroup.POST("/grant", couponH.GrantCoupon)
		couponGroup.GET("/list", couponH.ListUserCoupons)
	}
	return r, nil
}

func Run(ctx context.Context, config config.AppConfig) error {
	logger := clog.L(ctx)
	mp := otel.GetMeterProvider()

	db, err := dbconn.Init(ctx, logger, dbconn.Config{
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
	if err != nil {
		return fmt.Errorf("数据库初始化失败: %w", err)
	}

	if !config.IsProd() {
		if err := db.AutoMigrate(
			&model.User{},
			&model.UserWallet{},
			&model.WalletLog{},
			&model.Product{},
			&model.Order{},
			&model.StockChangeLog{},
			&model.CouponTemplate{},
			&model.UserCoupon{},
		); err != nil {
			return fmt.Errorf("数据库 AutoMigrate 失败: %w", err)
		}
	}

	rdb, err := redis.Init(ctx, redis.Config{
		Host:     config.Redis.Host,
		Port:     config.Redis.Port,
		Password: config.Redis.Password,
		DB:       config.Redis.DB,
		PoolSize: config.Redis.PoolSize,
	})
	if err != nil {
		return fmt.Errorf("Redis 初始化失败: %w", err)
	}

	mqCh, mqCleanup, err := mq.InitMq(ctx, logger, mq.Config{
		User:     config.RabbitMQ.User,
		Password: config.RabbitMQ.Password,
		Host:     config.RabbitMQ.Host,
		Port:     config.RabbitMQ.Port,
	})
	if err != nil {
		return fmt.Errorf("RabbitMQ 初始化失败: %w", err)
	}
	defer mqCleanup()

	walletRepo := wallet.NewRepository(db, rdb)

	authRepo := auth.NewRepository(db, rdb, &config.Auth)
	authSvc := auth.NewService(authRepo, &config.Auth)

	userMeter := mp.Meter("user_api")
	userMetrics, err := user.NewMetrics(userMeter)
	if err != nil {
		logger.Error("metrics初始化失败", zap.Error(err))
	}
	userRepo := user.NewRepository(db)

	bizMeter := mp.Meter("business_api")
	walletMetrics, err := wallet.NewMetrics(bizMeter)
	if err != nil {
		logger.Error("wallet metrics初始化失败", zap.Error(err))
	}
	orderMetrics, err := order.NewMetrics(bizMeter)
	if err != nil {
		logger.Error("order metrics初始化失败", zap.Error(err))
	}
	couponMetrics, err := coupon.NewMetrics(bizMeter)
	if err != nil {
		logger.Error("coupon metrics初始化失败", zap.Error(err))
	}

	walletSvc := wallet.NewService(walletRepo, walletMetrics)
	userSvc := user.NewService(userRepo, walletRepo, userMetrics)

	productRepo := product.NewRepository(db)
	productSvc := product.NewService(db, productRepo)

	orderRepo := order.NewRepository(db, mqCh, &config.OrderMQ)
	if err := orderRepo.SetupMQ(&config.OrderMQ); err != nil {
		return fmt.Errorf("初始化 order MQ 失败: %w", err)
	}
	couponRepo := coupon.NewRepository(db)
	couponSvc := coupon.NewService(db, couponRepo, couponMetrics)
	couponH := coupon.NewHandler(couponSvc)

	orderSvc := order.NewService(db, orderRepo, productRepo, couponRepo, orderMetrics)

	orderMqHandler := order.NewMqHandler(orderSvc)
	if err := orderMqHandler.ListenTimeout(ctx, mqCh, config.OrderMQ.ConsumerQueue); err != nil {
		return fmt.Errorf("启动订单消费者失败: %w", err)
	}

	r, err := SetupRouter(&config, authSvc, userSvc, walletSvc, productSvc, orderSvc, couponH, logger, &mp)
	if err != nil {
		return fmt.Errorf("初始化路由失败: %w", err)
	}

	// 注入 DB/Redis/MQ 用于健康检查
	healthDB = db
	healthRDB = rdb
	healthMQCh = mqCh

	addr := fmt.Sprintf("0.0.0.0:%d", config.App.Port)
	if config.App.SSL {
		if err := r.RunTLS(addr, config.App.SSLCrtPath, config.App.SSLKeyPath); err != nil {
			return fmt.Errorf("TLS 服务器启动失败: %w", err)
		}
	} else {
		if err := r.Run(addr); err != nil {
			return fmt.Errorf("服务器启动失败: %w", err)
		}
	}

	return nil
}
