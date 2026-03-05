package auth

import (
	"e-commerce/internal/app/identity"
	"e-commerce/pkg/errno"
	"e-commerce/pkg/res"
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

	var loginInput LoginInput
	if err := c.ShouldBindJSON(&loginInput); err != nil {
		res.WriteResponse(c, errno.ErrInvalidParam, nil)
		return
	}

	tokenPair, err := h.authSvc.Login(ctx, &loginInput)
	res.WriteResponse(c, err, tokenPair)
}

func (h *Handler) FetchAccessToken(c *gin.Context) {
	ctx := c.Request.Context()

	accountInfo := identity.GetAccountInfo(ctx)
	if accountInfo == nil {
		res.WriteResponse(c, errno.ErrGetAccountInfo, nil)
		return
	}

	at, err := h.authSvc.fetchAccessToken(ctx, accountInfo)
	res.WriteResponse(c, err, gin.H{
		"access_token": at,
	})
}

func (h *Handler) FetchRefreshToken(c *gin.Context) {
	ctx := c.Request.Context()

	accountInfo := identity.GetAccountInfo(ctx)
	if accountInfo == nil {
		res.WriteResponse(c, errno.ErrGetAccountInfo, nil)
		return
	}

	at, err := h.authSvc.fetchRefreshToken(ctx, accountInfo)
	if err != nil {
		res.WriteResponse(c, errno.ErrInternalServer.WithRaw(err), nil)
		return
	}

	res.WriteResponse(c, errno.OK, gin.H{
		"access_token": at,
	})
}

func (h *Handler) Logout(c *gin.Context) {
	ctx := c.Request.Context()

	accountInfo := identity.GetAccountInfo(ctx)
	if accountInfo == nil {
		res.WriteResponse(c, errno.ErrGetAccountInfo, nil)
	}

	err := h.authSvc.logout(ctx, accountInfo)
	if err != nil {
		res.WriteResponse(c, errno.ErrInternalServer.WithRaw(err), nil)
		return
	}

	res.WriteResponse(c, errno.OK, nil)
}
