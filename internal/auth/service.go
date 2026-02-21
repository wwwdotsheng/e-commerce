package auth

import (
	"context"
	"e-commerce/internal/app/identity"
	"e-commerce/internal/config"
	"e-commerce/internal/model"
	"errors"
	"golang.org/x/crypto/bcrypt"
)

var (
	NotPermission = errors.New("not permission")
)

var (
	svcNotFoundUserErr       = errors.New("not found user")
	svcPasswordVerifyFailErr = errors.New("password verify fail")
)

type LoginInput struct {
	Email    string
	Password string
}

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

func (svc *Service) Login(ctx context.Context, input *LoginInput) (*model.User, error) {
	user, err := svc.repo.FindUserByEmail(ctx, input.Email)
	if err != nil {
		if errors.Is(err, dbNotFoundUser) {
			return nil, svcNotFoundUserErr
		}
		return nil, err
	}

	err = bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(input.Password))
	if err != nil {
		return nil, svcPasswordVerifyFailErr
	}

	return user, nil
}
