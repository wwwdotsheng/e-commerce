package user

import (
	"e-commerce/internal/auth"
	"e-commerce/internal/model"
	"e-commerce/internal/pkg/response"
	"e-commerce/pkg/clog"

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
	SellerName      string `json:"seller_name" binding:"max=128"`
	ApplicationNote string `json:"application_note" binding:"max=1000"`
	UserName        string `json:"user_name" binding:"required,min=3,max=30"`
	Email           string `json:"email" binding:"required,email"`
	Password        string `json:"password" binding:"required,min=8,max=32"`
}

func NewHandler(userSvc *Service, authSvc *auth.Service) *Handler {
	return &Handler{userSvc: userSvc, authSvc: authSvc}
}

func (h *Handler) RegisterBuyer(c *gin.Context)  { h.register(c, model.UserTypeBuyer) }
func (h *Handler) RegisterSeller(c *gin.Context) { h.register(c, model.UserTypeSeller) }
func (h *Handler) register(c *gin.Context, userType model.UserType) {
	ctx := c.Request.Context()
	tracer := otel.Tracer("user-handler")
	ctx, span := tracer.Start(ctx, "RegisterOperation")
	defer span.End()
	logger := clog.L(ctx)

	var registerDTO RegisterDTO
	if err := c.ShouldBindJSON(&registerDTO); err != nil {
		response.WriteInvalidParam(c, err)
		return
	}

	span.SetAttributes(
		attribute.Bool("user.email", registerDTO.Email != ""),
		attribute.Bool("user.username", registerDTO.UserName != ""),
	)

	err := h.userSvc.Register(ctx, &RegisterInput{
		UserType: userType, SellerName: registerDTO.SellerName, ApplicationNote: registerDTO.ApplicationNote,
		UserName: registerDTO.UserName,
		Email:    registerDTO.Email,
		Password: registerDTO.Password,
	})

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		response.Write(c, err, nil)
		return
	}

	span.SetStatus(codes.Ok, "registered")
	logger.Info("用户注册成功")
	response.Write(c, nil, nil)
}
