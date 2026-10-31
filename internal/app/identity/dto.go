package identity

import (
	"context"
	"e-commerce/internal/model"

	"github.com/google/uuid"
)

type userIdentityCtx struct{}

var userIdentityCtxKey = userIdentityCtx{}

type AccountInfo struct {
	UserType  model.UserType   `json:"user_type"`
	Status    model.UserStatus `json:"status"`
	AccountId uuid.UUID        `json:"account_id"`
	SessionID string           `json:"session_id"`
}

func SetAccountInfo(ctx context.Context, data *AccountInfo) context.Context {
	return context.WithValue(ctx, userIdentityCtxKey, data)
}

func GetAccountInfo(ctx context.Context) *AccountInfo {
	if data, ok := ctx.Value(userIdentityCtxKey).(*AccountInfo); ok {
		return data
	}
	return nil
}
