package middleware

import (
	"e-commerce/internal/config"
	"e-commerce/internal/pkg/contextx"

	"github.com/gin-gonic/gin"
)

func InjectConfig(cfg *config.AppConfig) gin.HandlerFunc {
	return contextx.SetConfig(cfg)
}
