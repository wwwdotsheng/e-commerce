package wallet

import (
	"e-commerce/internal/app/identity"
	"e-commerce/internal/pkg/response"
	"e-commerce/pkg/errno"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type Handler struct {
	wallSvc *Service
}

type DepositDTO struct {
	Amount         float64 `json:"amount" binding:"required"`
	IdempotencyKey string  `json:"idempotency_key" binding:"required"`
}

func NewHandler(wallSvc *Service) *Handler {
	return &Handler{wallSvc: wallSvc}
}

func (h *Handler) Deposit(c *gin.Context) {
	ctx := c.Request.Context()
	// logger := clog.L(ctx)

	var depositDTO DepositDTO
	if err := c.ShouldBindJSON(&depositDTO); err != nil {
		response.Write(c, errno.ErrInvalidParam.WithRaw(err), nil)
		return
	}

	accountInfo := identity.GetAccountInfo(ctx)
	if accountInfo == nil {
		response.Write(c, errno.ErrInternalServer, nil)
		return
	}

	userID, err := uuid.Parse(accountInfo.AccountId.String())
	if err != nil {
		response.Write(c, errno.ErrInternalServer, nil)
		return
	}

	err = h.wallSvc.Deposit(ctx, userID, accountInfo.SessionID, &DepositInput{
		Amount:         depositDTO.Amount,
		IdempotencyKey: depositDTO.IdempotencyKey,
	})
	if err != nil {
		response.Write(c, errno.ErrInternalServer.WithRaw(err), nil)
		return
	}

	response.Write(c, nil, nil)
}
