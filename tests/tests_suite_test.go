package tests_test

import (
	"context"
	"e-commerce/internal/app"
	"e-commerce/internal/auth"
	"e-commerce/internal/model"
	"e-commerce/internal/user"
	"e-commerce/pkg/clog"
	"e-commerce/pkg/database"
	"e-commerce/pkg/redis"
	"encoding/json"
	"log"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	goredis "github.com/go-redis/redis/v8"
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
	appStopFunc    func()
)

var _ = BeforeSuite(func() {
	ctx, stop, config, err := app.Bootstrap("../configs/config.test.yaml")
	if err != nil {
		log.Fatalf("应用启动失败：%v", err)
	}

	defer stop()

	// TestContainer启动
	pgContainer, err = postgres.Run(
		ctx,
		"harbor.local/dockerhub/postgres:15-alpine",
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
		return
	}

	pgHost, err := pgContainer.Host(ctx)
	if err != nil {
		log.Fatalf("failed to get host: %v", err)
	}

	config.Database.Port = pgPort.Int()
	config.Database.Host = pgHost

	redisContainer, err = redis2.Run(
		ctx,
		"harbor.local/dockerhub/redis:7-alpine", // 确保镜像是 redis
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

	// 路由初始化 ======================================================
	logger := clog.L(ctx)
	mp := otel.GetMeterProvider()

	testDB = database.Init(ctx, config.Database)
	if err := testDB.AutoMigrate(&model.User{}); err != nil {
		logger.Fatal("数据库AutoMigrate失败")
	}
	testRedis = redis.Init(ctx, config.Redis)

	authRepo := auth.NewRepository(testDB, testRedis, &config.Auth)
	authSvc := auth.NewService(authRepo, &config.Auth)

	userMeter := mp.Meter("user_api")
	userMetrics, err := user.NewMetrics(userMeter)
	if err != nil {
		logger.Fatal("metrics初始化失败", zap.Error(err))
	}
	userRepo := user.NewRepository(testDB)
	userSvc := user.NewService(userRepo, userMetrics)

	testRouter, err = app.SetupRouter(authSvc, userSvc, logger, &mp)
	if err != nil {
		logger.Fatal("初始化路由失败", zap.Error(err))
	}
})

var _ = AfterSuite(func() {
	log.Println("正在清理测试资源...")
	if appStopFunc != nil {
		appStopFunc()
	}
	ctx := context.Background()
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
})
