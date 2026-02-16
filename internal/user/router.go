package user

import (
	"e-commerce/internal/auth"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/metric"
	"gorm.io/gorm"
)

// RegisterRouters TODO 这里是不是参数过于多了
func RegisterRouters(r *gin.RouterGroup, authSvc *auth.Service, authMiddleware gin.HandlerFunc, db *gorm.DB, meter metric.Meter) {
	metrics, err := NewMetrics(meter)
	// TODO 错误处理如何编写？直接panic？
	if err != nil {
		panic(err)
	}

	tg := r.Group("/user")
	repo := NewRepository(db)
	svc := NewService(repo, metrics)
	h := NewHandler(svc, authSvc)
	{
		tg.POST("/register", h.register)
		tg.POST("/login", h.login)
	}

	tg.Use(authMiddleware)
	{
		tg.GET("/good")
	}
}
