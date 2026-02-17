package middleware

import (
	"e-commerce/pkg/clog"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func InjectLoggerMiddleware(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		newCtx := clog.WithLogger(ctx, logger)
		c.Request = c.Request.WithContext(newCtx)
		c.Next()
	}
}
