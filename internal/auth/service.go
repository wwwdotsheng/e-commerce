package auth

import (
	"context"
	"e-commerce/internal/app/identity"
	"e-commerce/internal/config"
	"e-commerce/pkg/errno"
	"errors"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/golang-jwt/jwt/v5"
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

type TokenPayload struct {
	identity.AccountInfo
	TokenType TokenType `json:"token_type"`
	jwt.RegisteredClaims
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

// CreateSession 创建Session
func (svc *Service) CreateSession(ctx context.Context, userID uint) (string, error) {
	sid, err := genSessionId()
	if err != nil {
		return "", err
	}

	tokenPayload := TokenPayload{
		AccountInfo: identity.AccountInfo{
			AccountId: userID,
			SessionID: sid,
		},
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(svc.config.AccessTokenExpire)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, tokenPayload)
	return token.SignedString([]byte(svc.config.TokenSecret))
}

// VerifyToken 验证Token是否正常
func (svc *Service) VerifyToken(ctx context.Context, userToken string, tokenType TokenType) (*identity.AccountInfo, error) {
	payload := &TokenPayload{}
	token, err := jwt.ParseWithClaims(userToken, payload, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errno.ErrAuthInvalidToken
		}
		return []byte(svc.config.TokenSecret), nil
	})
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, errno.ErrAuthTokenExpired.WithRaw(err)
		} else if errors.Is(err, jwt.ErrTokenMalformed) {
			return nil, errno.ErrAuthInvalidToken.WithRaw(err)
		}
		return nil, errno.ErrAuthInvalidToken.WithRaw(err)
	}

	claims, ok := token.Claims.(*TokenPayload)
	if (!ok || !token.Valid) || (claims.TokenType != tokenType) {
		return nil, errno.ErrAuthInvalidToken
	}

	var findToken string
	if claims.TokenType == AccessToken {
		findToken, err = svc.repo.getAccessToken(ctx, claims.SessionID)
	} else if claims.TokenType == RefreshToken {
		findToken, err = svc.repo.getRefreshToken(ctx, claims.SessionID)
	}

	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, errno.ErrAuthInvalidToken.WithRaw(err)
		}
		return nil, errno.ErrInternalServer.WithRaw(err)
	}
	if findToken != userToken {
		return nil, errno.ErrAuthSessionRevoked
	}
	return &claims.AccountInfo, nil
}

func (svc *Service) generateToken(userID uint, sessionID string, tokenType TokenType) (string, error) {
	jti, err := generateRandomString(5)
	if err != nil {
		return "", fmt.Errorf("generate token jti fail: %w", err)
	}
	var tokenPayload TokenPayload
	if tokenType == AccessToken {
		tokenPayload = TokenPayload{
			AccountInfo: identity.AccountInfo{
				AccountId: userID,
				SessionID: sessionID,
			},
			TokenType: tokenType,
			RegisteredClaims: jwt.RegisteredClaims{
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(svc.config.AccessTokenExpire)),
				IssuedAt:  jwt.NewNumericDate(time.Now()),
				ID:        jti,
			},
		}
	} else if tokenType == RefreshToken {
		tokenPayload = TokenPayload{
			AccountInfo: identity.AccountInfo{
				AccountId: userID,
				SessionID: sessionID,
			},
			TokenType: tokenType,
			RegisteredClaims: jwt.RegisteredClaims{
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(svc.config.RefreshTokenExpire)),
				IssuedAt:  jwt.NewNumericDate(time.Now()),
				ID:        jti,
			},
		}
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, tokenPayload)
	tokenStr, err := token.SignedString([]byte(svc.config.TokenSecret))
	if err != nil {
		return "", fmt.Errorf("generate token error: %w", err)
	}
	return tokenStr, nil
}

func (svc *Service) Login(ctx context.Context, input *LoginInput) (*TokenPair, error) {
	user, err := svc.repo.FindUserByEmail(ctx, input.Email)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errno.ErrUserNotFound
	}

	err = bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(input.Password))
	if err != nil {
		return nil, errno.ErrUserNotFound
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
