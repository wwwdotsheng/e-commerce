package user

import (
	"context"
	"errors"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

type RegisterInput struct {
	UserName string
	Email    string
	Password string
}

type LoginInput struct {
	Email    string
	Password string
}

type Service struct {
	repo *Repository
}

var (
	svcUserNameRegisteredErr = errors.New("username registered")
	svcEmailRegisteredErr    = errors.New("email registered")
	svcNotFoundUserErr       = errors.New("not found user")
	svcPasswordVerifyFailErr = errors.New("password verify fail")
)

func NewService(repository *Repository) *Service {
	return &Service{repo: repository}
}

func (svc *Service) register(ctx context.Context, input *RegisterInput) error {
	bytes, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("password hash failed: %w", err)
	}

	err = svc.repo.createUser(ctx, &User{
		Email:    input.Email,
		Password: string(bytes),
		UserName: input.UserName,
	})

	if err != nil {
		if errors.Is(err, dbUserNameAlreadyExists) {
			return svcUserNameRegisteredErr
		} else if errors.Is(err, dbEmailAlreadyExists) {
			return svcEmailRegisteredErr
		}
		return fmt.Errorf("register error: %w", err)
	}

	return nil
}

func (svc *Service) login(ctx context.Context, input *LoginInput) (*User, error) {
	user, err := svc.repo.findUserByEmail(ctx, input.Email)
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
