package user

import (
	"e-commerce/internal/auth"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func RegisterRouters(r *gin.RouterGroup, authSvc *auth.Service, authMiddleware gin.HandlerFunc, db *gorm.DB) {
	tg := r.Group("/user")
	repo := NewRepository(db)
	svc := NewService(repo)
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
