package auth

import (
	"e-commerce/internal/pkg/app"
	"e-commerce/pkg/clog"
	"errors"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"net/http"
)

type Handler struct {
	authSvc *Service
}

type LoginDTO struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

func NewHandler(authSvc *Service) *Handler {
	return &Handler{authSvc: authSvc}
}

func (h *Handler) Login(c *gin.Context) {
	ctx := c.Request.Context()
	logger := clog.L(ctx)

	var loginInput LoginInput
	if err := c.ShouldBindJSON(&loginInput); err != nil {
		app.BadRequest(c)
		return
	}

	user, err := h.authSvc.Login(ctx, &loginInput)
	if err != nil {
		if errors.Is(err, svcNotFoundUserErr) || errors.Is(err, svcPasswordVerifyFailErr) {
			app.Fail(c, http.StatusBadRequest, nil, "账号或密码错误")
			return
		}
		logger.Error("登录出现错误", zap.Error(err))
		app.InternalError(c)
		return
	}

	tokenPair, err := h.authSvc.CreateSession(ctx, user.ID)
	if err != nil {
		logger.Error("创建session时出现问题", zap.Error(err))
		app.InternalError(c)
		return
	}

	app.Success(c, tokenPair)
}
