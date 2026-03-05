package user

import (
	"e-commerce/internal/auth"
	"e-commerce/pkg/clog"
	"e-commerce/pkg/errno"
	"e-commerce/pkg/res"
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
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

func NewHandler(userSvc *Service, authSvc *auth.Service) *Handler {
	return &Handler{userSvc: userSvc, authSvc: authSvc}
}

func (h *Handler) Register(c *gin.Context) {
	ctx := c.Request.Context()
	tracer := otel.Tracer("user-handler")
	ctx, span := tracer.Start(ctx, "RegisterOperation")
	defer span.End()
	logger := clog.L(ctx)

	var registerDTO RegisterDTO
	if err := c.ShouldBindJSON(&registerDTO); err != nil {
		res.WriteResponse(c, errno.ErrInvalidParam, nil)
		return
	}

	span.SetAttributes(
		attribute.Bool("user.email", registerDTO.Email != ""),
		attribute.Bool("user.username", registerDTO.UserName != ""),
	)

	err := h.userSvc.Register(ctx, &RegisterInput{
		UserName: registerDTO.UserName,
		Email:    registerDTO.Email,
		Password: registerDTO.Password,
	})

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		res.WriteResponse(c, err, nil)
		return
	}

	span.SetStatus(codes.Ok, "registered")
	logger.Info("用户注册成功")
	res.WriteResponse(c, nil, nil)
}
