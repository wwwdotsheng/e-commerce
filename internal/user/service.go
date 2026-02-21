package user

import (
	"context"
	"e-commerce/internal/model"
	"errors"
	"fmt"

	"go.opentelemetry.io/otel"
	"golang.org/x/crypto/bcrypt"
)

type RegisterInput struct {
	UserName string
	Email    string
	Password string
}

type Service struct {
	repo    *Repository
	metrics *Metrics
}

var (
	svcUserNameRegisteredErr = errors.New("username registered")
	svcEmailRegisteredErr    = errors.New("email registered")
)

func NewService(repository *Repository, metrics *Metrics) *Service {
	return &Service{repo: repository, metrics: metrics}
}

func (svc *Service) Register(ctx context.Context, input *RegisterInput) (err error) {
	var errCode = MetErrCodeInternal
	defer func() {
		if err != nil {
			svc.metrics.AddUserRegistrationTotal(ctx, MetRegStatusFail, errCode)
		} else {
			svc.metrics.AddUserRegistrationTotal(ctx, MetRegStatusSuccess, MetErrCodeNone)
		}
	}()

	_, span := otel.Tracer("user-service").Start(ctx, "BcryptHash")
	bytes, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)

	if err != nil {
		span.RecordError(err)
		span.End()
		return fmt.Errorf("password hash failed: %w", err)
	}
	span.End()

	err = svc.repo.CreateUser(ctx, &model.User{
		Email:    input.Email,
		Password: string(bytes),
		UserName: input.UserName,
	})

	if err != nil {
		if errors.Is(err, dbUserNameAlreadyExists) {
			errCode = MetErrCodeUserRegistered
			return svcUserNameRegisteredErr
		} else if errors.Is(err, dbEmailAlreadyExists) {
			errCode = MetErrCodeEmailRegistered
			return svcEmailRegisteredErr
		}
		return fmt.Errorf("register error: %w", err)
	}

	return nil
}
