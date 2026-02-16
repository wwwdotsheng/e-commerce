package user

import (
	"e-commerce/internal/auth"
	"e-commerce/internal/pkg/app"
	"e-commerce/pkg/clog"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.uber.org/zap"
)

type Handler struct {
	userSvc *Service
	authSvc *auth.Service
}

type RegisterDTO struct {
	UserName string `json:"user_name" binding:"required,min=3,max=32"`
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=8,max=32"`
}

type LoginDTO struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

func NewHandler(userSvc *Service, authSvc *auth.Service) *Handler {
	return &Handler{userSvc: userSvc, authSvc: authSvc}
}

func (h *Handler) register(c *gin.Context) {
	ctx := c.Request.Context()
	tracer := otel.Tracer("user-handler")
	ctx, span := tracer.Start(ctx, "RegisterOperation")
	defer span.End()
	logger := clog.L(ctx)

	var registerDTO RegisterDTO
	if err := c.ShouldBindJSON(&registerDTO); err != nil {
		app.BadRequest(c)
		return
	}

	span.SetAttributes(
		attribute.String("user.email", registerDTO.Email),
		attribute.String("user.username", registerDTO.UserName),
	)

	err := h.userSvc.register(ctx, &RegisterInput{
		UserName: registerDTO.UserName,
		Email:    registerDTO.Email,
		Password: registerDTO.Password,
	})

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())

		if errors.Is(err, svcUserNameRegisteredErr) {
			logger.Warn("用户名已被注册")
			app.Fail(c, http.StatusBadRequest, nil, "用户名已被注册")
			return
		} else if errors.Is(err, svcEmailRegisteredErr) {
			logger.Warn("用户邮箱已被注册")
			app.Fail(c, http.StatusBadRequest, nil, "用户邮箱已被注册")
			return
		}
		app.InternalError(c)
		return
	}

	span.SetStatus(codes.Ok, "registered")
	logger.Info("用户注册成功", zap.String("email", registerDTO.Email))
	app.Success(c, nil)
}

func (h *Handler) login(c *gin.Context) {
	ctx := c.Request.Context()
	logger := clog.L(ctx)

	var loginInput LoginInput
	if err := c.ShouldBindJSON(&loginInput); err != nil {
		app.BadRequest(c)
		return
	}

	user, err := h.userSvc.login(ctx, &loginInput)
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
