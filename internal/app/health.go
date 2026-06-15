package app

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"
)

var (
	healthDB   *gorm.DB
	healthRDB  *redis.Client
	healthMQCh *amqp.Channel
)

// HealthHandler 存活检查（K8s liveness probe）
func HealthHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// ReadyHandler 就绪检查（K8s readiness probe）
func ReadyHandler(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()

	checks := gin.H{}

	if healthDB != nil {
		sqlDB, err := healthDB.DB()
		if err != nil || sqlDB.PingContext(ctx) != nil {
			checks["database"] = "unhealthy"
		} else {
			checks["database"] = "ok"
		}
	} else {
		checks["database"] = "not_initialized"
	}

	if healthRDB != nil {
		if _, err := healthRDB.Ping(ctx).Result(); err != nil {
			checks["redis"] = "unhealthy"
		} else {
			checks["redis"] = "ok"
		}
	} else {
		checks["redis"] = "not_initialized"
	}

	if healthMQCh != nil && !healthMQCh.IsClosed() {
		checks["rabbitmq"] = "ok"
	} else {
		checks["rabbitmq"] = "unhealthy"
	}

	allHealthy := true
	for _, v := range checks {
		if v != "ok" {
			allHealthy = false
			break
		}
	}

	status := http.StatusOK
	if !allHealthy {
		status = http.StatusServiceUnavailable
	}

	c.JSON(status, gin.H{"status": checks})
}
