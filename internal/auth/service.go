package auth

import (
	"context"
	"e-commerce/internal/app/identity"
	"e-commerce/internal/config"
	"errors"
)

var (
	NotPermission = errors.New("not permission")
)

type Service struct {
	repo   *Repository
	config *config.AuthSection
}

func NewService(repo *Repository, config *config.AuthSection) *Service {
	return &Service{repo: repo, config: config}
}

// CreateSession 创建Session
func (svc *Service) CreateSession(ctx context.Context, accountId uint) (*TokenPair, error) {
	tokenPair, err := svc.repo.createSession(ctx, accountId)
	if err != nil {
		return nil, err
	}
	return tokenPair, nil
}

// Verify 验证Token是否正常
func (svc *Service) Verify(ctx context.Context, accessToken string) (*identity.AccountInfo, error) {
	accountInfo, err := svc.repo.findAccessToken(ctx, accessToken)
	if err != nil {
		if errors.Is(err, rdbNotFoundAccessToken) {
			return nil, NotPermission
		}
		return nil, err
	}

	return accountInfo, nil
}
