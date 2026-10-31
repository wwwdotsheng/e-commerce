package auth

import (
	"context"
	"crypto/rand"
	"e-commerce/internal/app/identity"
	"e-commerce/internal/config"
	"e-commerce/internal/model"
	"e-commerce/pkg/errno"
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type TokenType string

var (
	AccessToken  TokenType = "access_token"
	RefreshToken TokenType = "refresh_token"
)

type LoginInput struct {
	Email    string
	Password string
}

type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

type Service struct {
	repo   *Repository
	config *config.AuthSection
}

func NewService(repo *Repository, config *config.AuthSection) *Service {
	return &Service{repo: repo, config: config}
}

func genSessionId() (string, error) {
	result, err := generateRandomString(24)
	if err != nil {
		return "", fmt.Errorf("generate session id token fail: %w", err)
	}
	return result, nil
}

// VerifyToken resolves an opaque token through the server-side session.
func (svc *Service) VerifyToken(ctx context.Context, userToken string, tokenType TokenType) (*identity.AccountInfo, error) {
	if tokenType != AccessToken && tokenType != RefreshToken {
		return nil, errno.ErrAuthInvalidToken
	}
	decoded, err := base64.RawURLEncoding.DecodeString(userToken)
	if err != nil || len(decoded) != 32 || base64.RawURLEncoding.EncodeToString(decoded) != userToken {
		return nil, errno.ErrAuthInvalidToken
	}
	account, err := svc.repo.verifyOpaqueToken(ctx, userToken, tokenType)
	if errors.Is(err, redis.Nil) || errors.Is(err, RepoErrSessionNotFound) {
		return nil, errno.ErrAuthInvalidToken
	}
	if errors.Is(err, errTokenExpired) {
		return nil, errno.ErrAuthTokenExpired
	}
	if err != nil {
		return nil, errno.ErrInternalServer.WithRaw(err)
	}
	user, err := svc.repo.FindUserByID(ctx, account.AccountId)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errno.ErrAuthInvalidToken
	}
	if err != nil {
		return nil, errno.ErrInternalServer.WithRaw(err)
	}
	if user.Status == model.UserStatusDisabled {
		_ = svc.repo.DelSession(ctx, account.AccountId, account.SessionID)
		return nil, errno.ErrAuthNotPermission
	}
	account.UserType, account.Status = user.UserType, user.Status
	return account, nil
}

func (svc *Service) generateToken(userID uuid.UUID, sessionID string, tokenType TokenType) (string, error) {
	var random [32]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", fmt.Errorf("generate random token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(random[:]), nil
}

func (svc *Service) Login(ctx context.Context, input *LoginInput) (*TokenPair, error) {
	user, err := svc.repo.FindUserByEmail(ctx, input.Email)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errno.ErrUserNotFound
	}
	if err != nil {
		return nil, errno.ErrInternalServer.WithRaw(err)
	}

	err = bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(input.Password))
	if err != nil {
		return nil, errno.ErrUserNotFound
	}

	if user.Status == model.UserStatusDisabled {
		return nil, errno.ErrAuthNotPermission
	}
	sid, err := genSessionId()
	if err != nil {
		return nil, err
	}

	at, err := svc.generateToken(user.ID, sid, AccessToken)
	if err != nil {
		return nil, err
	}

	rt, err := svc.generateToken(user.ID, sid, RefreshToken)
	if err != nil {
		return nil, err
	}

	err = svc.repo.createSession(ctx, user.ID, sid, at, rt, svc.config.RefreshTokenExpire)
	if err != nil {
		return nil, err
	}

	return &TokenPair{
		AccessToken:  at,
		RefreshToken: rt,
	}, nil
}

func (svc *Service) fetchAccessToken(ctx context.Context, accountInfo *identity.AccountInfo) (string, error) {
	newAccessToken, err := svc.generateToken(accountInfo.AccountId, accountInfo.SessionID, AccessToken)
	if err != nil {
		return "", err
	}

	err = svc.repo.SetAccessToken(ctx, accountInfo.AccountId, accountInfo.SessionID, newAccessToken)
	if err != nil {
		if errors.Is(err, RepoErrSessionNotFound) {
			return "", errno.ErrAuthInvalidToken
		}
		return "", errno.ErrInternalServer.WithRaw(err)
	}
	return newAccessToken, nil
}

func (svc *Service) fetchRefreshToken(ctx context.Context, accountInfo *identity.AccountInfo) (string, error) {
	newRefreshToken, err := svc.generateToken(accountInfo.AccountId, accountInfo.SessionID, RefreshToken)
	if err != nil {
		return "", err
	}

	err = svc.repo.SetRefreshToken(ctx, accountInfo.AccountId, accountInfo.SessionID, newRefreshToken)
	if err != nil {
		if errors.Is(err, RepoErrSessionNotFound) {
			return "", errno.ErrAuthInvalidToken
		}
		return "", errno.ErrInternalServer.WithRaw(err)
	}
	return newRefreshToken, nil
}

func (svc *Service) logout(ctx context.Context, accountInfo *identity.AccountInfo) error {
	err := svc.repo.DelSession(ctx, accountInfo.AccountId, accountInfo.SessionID)
	if err != nil {
		return errno.ErrInternalServer.WithRaw(err)
	}
	return nil
}
