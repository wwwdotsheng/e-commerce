package identity

import "context"

type userIdentityCtx struct{}

var userIdentityCtxKey = userIdentityCtx{}

type AccountInfo struct {
	AccountId uint   `json:"account_id"`
	SessionID string `json:"session_id"`
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
