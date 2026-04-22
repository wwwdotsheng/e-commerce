package middleware

import (
	"e-commerce/internal/app/identity"
	"e-commerce/internal/auth"
	"e-commerce/internal/pkg/response"
	"e-commerce/pkg/clog"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func AccessTokenAuth(authService *auth.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		logger := clog.L(ctx)

		token := c.GetHeader("Authorization")
		accountInfo, err := authService.VerifyToken(ctx, token, auth.AccessToken)
		if err != nil {
			response.Write(c, err, nil)
			c.Abort()
			return
		}

		ctx = identity.SetAccountInfo(ctx, accountInfo)
		newLogger := logger.With(
			zap.String("account_id", accountInfo.AccountId.String()),
			zap.String("session_id", accountInfo.SessionID),
		)
		newCtx := clog.WithLogger(ctx, newLogger)
		c.Request = c.Request.WithContext(newCtx)
		c.Next()
		return
	}
}

func RefreshTokenAuth(authService *auth.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		logger := clog.L(ctx)

		token := c.GetHeader("Authorization")
		accountInfo, err := authService.VerifyToken(ctx, token, auth.RefreshToken)
		if err != nil {
			response.Write(c, err, nil)
			c.Abort()
			return
		}

		ctx = identity.SetAccountInfo(ctx, accountInfo)
		newLogger := logger.With(
			zap.String("account_id", accountInfo.AccountId.String()),
			zap.String("session_id", accountInfo.SessionID),
		)
		newCtx := clog.WithLogger(ctx, newLogger)
		c.Request = c.Request.WithContext(newCtx)
		c.Next()
		return
	}
}
