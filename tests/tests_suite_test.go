package tests

import (
	"context"
	"e-commerce/internal/app"
	"e-commerce/internal/auth"
	"e-commerce/internal/coupon"
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
	"log"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	goredis "github.com/go-redis/redis/v8"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	redis2 "github.com/testcontainers/testcontainers-go/modules/redis"
	"github.com/testcontainers/testcontainers-go/wait"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.opentelemetry.io/otel"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func TestTests(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Tests Suite")
}

type Response struct {
	Code    string          `json:"code"`
	UserMsg string          `json:"userMsg"`
	DevMsg  string          `json:"devMsg"`
	Data    json.RawMessage `json:"data"`
}

var (
	testDB     *gorm.DB
	testRedis  *goredis.Client
	testRouter *gin.Engine

	pgContainer    testcontainers.Container
	redisContainer testcontainers.Container
	rmqContainer   testcontainers.Container
	mqCh           *amqp.Channel
	mqCleanup      func()
	appStopFunc    func()
)

var logger *zap.Logger

var _ = BeforeSuite(func() {
	if os.Getenv("CONFIG_PATH") == "" {
		os.Setenv("CONFIG_PATH", "../configs/config.yaml")
		os.Setenv("APP_APP_ENV", "test")
	}

	ctx, stop, config, err := app.Bootstrap()
	if err != nil {
		log.Fatalf("应用启动失败：%v", err)
	}

	defer stop()

	// --- PostgreSQL ---
	pgContainer, err = postgres.Run(
		ctx,
		config.ImageRef(config.TestImages.Postgres),
		postgres.WithDatabase(config.Database.DBName),
		postgres.WithUsername(config.Database.User),
		postgres.WithPassword(config.Database.Password),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(15*time.Second),
		),
	)
	if err != nil {
		log.Fatalf("failed to start postgres container: %v", err)
	}

	pgPort, err := pgContainer.MappedPort(ctx, "5432")
	if err != nil {
		log.Fatalf("failed to get mapped port: %v", err)
	}

	pgHost, err := pgContainer.Host(ctx)
	if err != nil {
		log.Fatalf("failed to get host: %v", err)
	}

	config.Database.Port = pgPort.Int()
	config.Database.Host = pgHost

	// --- Redis ---
	redisContainer, err = redis2.Run(
		ctx,
		config.ImageRef(config.TestImages.Redis),
	)
	if err != nil {
		log.Fatalf("failed to start redis container: %v", err)
	}

	redisHost, err := redisContainer.Host(ctx)
	if err != nil {
		log.Fatalf("failed to get redis host: %v", err)
	}
	redisPort, err := redisContainer.MappedPort(ctx, "6379")
	if err != nil {
		log.Fatalf("failed to get redis mapped port: %v", err)
	}

	config.Redis.Host = redisHost
	config.Redis.Port = redisPort.Int()

	// --- RabbitMQ ---
	rmqContainer, err = testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        config.ImageRef(config.TestImages.RabbitMQ),
			ExposedPorts: []string{"5672/tcp"},
			WaitingFor: wait.ForLog("TCP listener on").
				WithStartupTimeout(60 * time.Second),
		},
		Started: true,
	})
	if err != nil {
		log.Fatalf("failed to start rabbitmq container: %v", err)
	}

	rmqHost, err := rmqContainer.Host(ctx)
	if err != nil {
		log.Fatalf("failed to get rmq host: %v", err)
	}
	rmqPort, err := rmqContainer.MappedPort(ctx, "5672/tcp")
	if err != nil {
		log.Fatalf("failed to get rmq mapped port: %v", err)
	}

	config.RabbitMQ.Host = rmqHost
	config.RabbitMQ.Port = rmqPort.Int()

	// 路由初始化 ======================================================
	logger = clog.L(ctx)
	mp := otel.GetMeterProvider()

	testDB, err = dbconn.Init(ctx, logger, dbconn.Config{
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
	if err := testDB.AutoMigrate(
		&model.User{},
		&model.UserWallet{},
		&model.WalletLog{},
		&model.Product{},
		&model.Order{},
		&model.StockChangeLog{},
	); err != nil {
		logger.Fatal("数据库AutoMigrate失败")
	}
	testRedis, err = redis.Init(ctx, redis.Config{
		Host:     config.Redis.Host,
		Port:     config.Redis.Port,
		Password: config.Redis.Password,
		DB:       config.Redis.DB,
		PoolSize: config.Redis.PoolSize,
	})

	mqCh, mqCleanup, err = mq.InitMq(ctx, logger, mq.Config{
		User:     config.RabbitMQ.User,
		Password: config.RabbitMQ.Password,
		Host:     config.RabbitMQ.Host,
		Port:     config.RabbitMQ.Port,
	})

	// --- Service 初始化 ---
	authRepo := auth.NewRepository(testDB, testRedis, &config.Auth)
	authSvc := auth.NewService(authRepo, &config.Auth)

	walletRepo := wallet.NewRepository(testDB, testRedis)

	userMeter := mp.Meter("user_api")
	userMetrics, err := user.NewMetrics(userMeter)
	if err != nil {
		logger.Fatal("metrics初始化失败", zap.Error(err))
	}
	userRepo := user.NewRepository(testDB)
	userSvc := user.NewService(userRepo, walletRepo, userMetrics)

	bizMeter := mp.Meter("business_api")
	walletMetrics, err := wallet.NewMetrics(bizMeter)
	if err != nil {
		logger.Fatal("wallet metrics初始化失败", zap.Error(err))
	}
	orderMetrics, err := order.NewMetrics(bizMeter)
	if err != nil {
		logger.Fatal("order metrics初始化失败", zap.Error(err))
	}
	couponMetrics, err := coupon.NewMetrics(bizMeter)
	if err != nil {
		logger.Fatal("coupon metrics初始化失败", zap.Error(err))
	}

	walletSvc := wallet.NewService(walletRepo, walletMetrics)

	productRepo := product.NewRepository(testDB)
	productSvc := product.NewService(testDB, productRepo)

	orderRepo := order.NewRepository(testDB, mqCh, &config.OrderMQ)
	if err := orderRepo.SetupMQ(&config.OrderMQ); err != nil {
		logger.Fatal("初始化order mq失败", zap.Error(err))
	}
	couponRepo := coupon.NewRepository(testDB)
	couponH := coupon.NewHandler(coupon.NewService(testDB, couponRepo, couponMetrics))
	orderSvc := order.NewService(testDB, orderRepo, productRepo, couponRepo, orderMetrics)

	testRouter, err = app.SetupRouter(config, authSvc, userSvc, walletSvc, productSvc, orderSvc, couponH, logger, &mp)
	if err != nil {
		logger.Fatal("初始化路由失败", zap.Error(err))
	}
})

var _ = AfterSuite(func() {
	logger.Info("正在清理测试资源...")
	if appStopFunc != nil {
		appStopFunc()
	}
	ctx := context.Background()

	if mqCleanup != nil {
		mqCleanup()
	}

	defer func() {
		if err := pgContainer.Terminate(ctx); err != nil {
			log.Fatalf("failed to terminate postgres container: %v", err)
		}
	}()
	defer func() {
		if err := redisContainer.Terminate(ctx); err != nil {
			log.Fatalf("failed to terminate redis container: %v", err)
		}
	}()
	defer func() {
		if rmqContainer != nil {
			if err := rmqContainer.Terminate(ctx); err != nil {
				log.Fatalf("failed to terminate rabbitmq container: %v", err)
			}
		}
	}()
})
