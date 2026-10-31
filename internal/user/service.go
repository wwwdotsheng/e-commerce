package user

import (
	"context"
	"e-commerce/internal/model"
	"e-commerce/internal/pkg/database"
	"e-commerce/internal/wallet"
	"e-commerce/pkg/errno"
	"errors"
	"fmt"
	"strings"

	"go.opentelemetry.io/otel"
	"golang.org/x/crypto/bcrypt"
)

type RegisterInput struct {
	UserType        model.UserType
	ShopName        string
	ApplicationNote string
	UserName        string
	Email           string
	Password        string
}

type Service struct {
	repo       *Repository
	walletRepo *wallet.Repository
	metrics    *Metrics
}

func NewService(repository *Repository, walletRepo *wallet.Repository, metrics *Metrics) *Service {
	return &Service{repo: repository, walletRepo: walletRepo, metrics: metrics}
}

func (svc *Service) Register(ctx context.Context, input *RegisterInput) (err error) {
	if input.UserType == "" {
		input.UserType = model.UserTypeBuyer
	}
	if input.UserType != model.UserTypeBuyer && input.UserType != model.UserTypeSeller {
		return errno.ErrInvalidParam
	}
	if input.UserType == model.UserTypeSeller && (strings.TrimSpace(input.ShopName) == "" || strings.TrimSpace(input.ApplicationNote) == "") {
		return errno.ErrInvalidParam
	}
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

	if err := database.ExecuteTransaction(ctx, svc.repo.GetDB(ctx), func(txCtx context.Context) error {
		status := model.UserStatusActive
		if input.UserType == model.UserTypeSeller {
			status = model.UserStatusPending
		}
		user := &model.User{
			UserType: input.UserType, Status: status, ShopName: input.ShopName, ApplicationNote: input.ApplicationNote,
			UserName: input.UserName,
			Email:    input.Email,
			Password: string(bytes),
		}
		if err := svc.repo.CreateUser(txCtx, user); err != nil {
			if errors.Is(err, repoErrUserNameAlreadyExists) {
				errCode = MetErrCodeUserRegistered
				return errno.ErrUserNameExisted
			} else if errors.Is(err, repoErrEmailAlreadyExists) {
				errCode = MetErrCodeEmailRegistered
				return errno.ErrUserEmailExisted
			}
			return errno.ErrInternalServer.WithRaw(err)
		}
		if err := svc.walletRepo.CreateDefaultAccount(txCtx, user.ID); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return fmt.Errorf("transaction failed: %w", err)
	}
	return nil
}
