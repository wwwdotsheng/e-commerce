package middleware

import (
	"e-commerce/internal/app/identity"
	"e-commerce/internal/auth"
	"e-commerce/internal/pkg/app"
	"e-commerce/pkg/clog"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func Auth(authService *auth.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		logger := clog.L(ctx)

		token := c.GetHeader("Authorization")
		if token == "" {
			logger.Info("空令牌访问")
			app.Fail(c, http.StatusForbidden, nil, "无权限访问")
			c.Abort()
			return
		}

		accountInfo, err := authService.Verify(ctx, token)
		if err != nil {
			if errors.Is(err, auth.NotPermission) {
				app.Fail(c, http.StatusForbidden, nil, "无权限访问")
				c.Abort()
				return
			}
			app.InternalError(c)
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
