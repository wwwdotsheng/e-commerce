package middleware

import (
	"e-commerce/internal/app/identity"
	"e-commerce/internal/auth"
	"e-commerce/pkg/clog"
	"e-commerce/pkg/res"
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
			res.WriteResponse(c, err, nil)
			c.Abort()
			return
		}

		ctx = identity.SetAccountInfo(ctx, accountInfo)
		newLogger := logger.With(
			zap.Uint("account_id", accountInfo.AccountId),
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
			res.WriteResponse(c, err, nil)
			c.Abort()
			return
		}

		ctx = identity.SetAccountInfo(ctx, accountInfo)
		newLogger := logger.With(
			zap.Uint("account_id", accountInfo.AccountId),
			zap.String("session_id", accountInfo.SessionID),
		)
		newCtx := clog.WithLogger(ctx, newLogger)
		c.Request = c.Request.WithContext(newCtx)
		c.Next()
		return
	}
}
