package auth

import (
	"e-commerce/internal/app/identity"
	"e-commerce/internal/pkg/response"
	"e-commerce/pkg/errno"

	"github.com/gin-gonic/gin"
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

	var loginDTO LoginDTO
	if err := c.ShouldBindJSON(&loginDTO); err != nil {
		response.Write(c, errno.ErrInvalidParam, nil)
		return
	}
	tokenPair, err := h.authSvc.Login(ctx, &LoginInput{
		Email:    loginDTO.Email,
		Password: loginDTO.Password,
	})
	response.Write(c, err, tokenPair)
}

func (h *Handler) FetchAccessToken(c *gin.Context) {
	ctx := c.Request.Context()

	accountInfo := identity.GetAccountInfo(ctx)
	if accountInfo == nil {
		response.Write(c, errno.ErrGetAccountInfo, nil)
		return
	}

	at, err := h.authSvc.fetchAccessToken(ctx, accountInfo)
	response.Write(c, err, gin.H{
		"access_token": at,
	})
}

func (h *Handler) FetchRefreshToken(c *gin.Context) {
	ctx := c.Request.Context()

	accountInfo := identity.GetAccountInfo(ctx)
	if accountInfo == nil {
		response.Write(c, errno.ErrGetAccountInfo, nil)
		return
	}

	rt, err := h.authSvc.fetchRefreshToken(ctx, accountInfo)
	if err != nil {
		response.Write(c, errno.ErrInternalServer.WithRaw(err), nil)
		return
	}

	response.Write(c, err, gin.H{
		"refresh_token": rt,
	})
}

func (h *Handler) Logout(c *gin.Context) {
	ctx := c.Request.Context()

	accountInfo := identity.GetAccountInfo(ctx)
	if accountInfo == nil {
		response.Write(c, errno.ErrGetAccountInfo, nil)
	}

	err := h.authSvc.logout(ctx, accountInfo)
	if err != nil {
		response.Write(c, errno.ErrInternalServer.WithRaw(err), nil)
		return
	}

	response.Write(c, errno.OK, nil)
}
