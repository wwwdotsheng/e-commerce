package middleware

import (
	"context"
	"e-commerce/internal/pkg/app"
	"e-commerce/pkg/clog"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

func TraceRequest(ctx context.Context) gin.HandlerFunc {
	logger := clog.L(ctx)
	return func(c *gin.Context) {
		start := time.Now()
		requestID, err := uuid.NewV7()

		// TODO Fallback永远比Abort好
		if err != nil {
			logger.Error("生成UUID错误", zap.Error(err))
			app.InternalError(c)
			c.Abort()
			return
		}

		scopedLogger := logger.With(zap.String("request_id", requestID.String()))

		ctx := clog.WithLogger(c.Request.Context(), scopedLogger)
		c.Request = c.Request.WithContext(ctx)

		c.Header("x-request-id", requestID.String())

		c.Next()

		latency := time.Since(start)
		scopedLogger.Info("请求完成",
			zap.Int("status", c.Writer.Status()),
			zap.Duration("latency", latency),
			zap.String("path", c.Request.URL.Path),
			zap.String("method", c.Request.Method),
			zap.String("ip", c.ClientIP()),
			zap.String("ua", c.Request.UserAgent()),
			zap.String("errors", c.Errors.ByType(gin.ErrorTypePrivate).String()),
		)
	}
}
