package wallet

import (
	"e-commerce/internal/app/identity"
	"e-commerce/internal/pkg/response"
	"e-commerce/pkg/errno"
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
)

type Handler struct {
	wallSvc *Service
}

type DepositDTO struct {
	Amount         float64 `json:"amount" binding:"required"`
	IdempotencyKey string  `json:"idempotency_key" binding:"required,max=64"`
}

func NewHandler(wallSvc *Service) *Handler {
	return &Handler{wallSvc: wallSvc}
}

// isIdempotencyKeyBindingError 判断参数绑定失败是否只由幂等键字段引起。
func isIdempotencyKeyBindingError(err error) bool {
	var validationErrors validator.ValidationErrors
	if !errors.As(err, &validationErrors) {
		return false
	}
	for _, fieldError := range validationErrors {
		if fieldError.StructField() == "IdempotencyKey" {
			return true
		}
	}
	return false
}

func (h *Handler) Deposit(c *gin.Context) {
	ctx := c.Request.Context()

	var depositDTO DepositDTO
	if err := c.ShouldBindJSON(&depositDTO); err != nil {
		// 幂等键为空或超长时与业务层保持同一个错误码，避免同一类问题出现两种返回码。
		if isIdempotencyKeyBindingError(err) {
			response.Write(c, errno.ErrWalletInvalidIdempotencyKey, nil)
			return
		}
		response.WriteInvalidParam(c, err)
		return
	}

	accountInfo := identity.GetAccountInfo(ctx)
	if accountInfo == nil {
		response.Write(c, errno.ErrGetAccountInfo, nil)
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
		response.Write(c, err, nil)
		return
	}

	response.Write(c, nil, nil)
}

type BalanceResponse struct {
	Balance float64 `json:"balance"`
}

func (h *Handler) GetBalance(c *gin.Context) {
	account := identity.GetAccountInfo(c.Request.Context())
	if account == nil {
		response.Write(c, errno.ErrGetAccountInfo, nil)
		return
	}
	balance, err := h.wallSvc.GetBalance(c.Request.Context(), account.AccountId)
	response.Write(c, err, BalanceResponse{Balance: balance})
}
