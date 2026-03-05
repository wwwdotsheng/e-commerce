package user

import (
	"context"
	"e-commerce/internal/model"
	"errors"
	"fmt"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

var (
	repoErrEmailAlreadyExists    = errors.New("email already exists")
	repoErrUserNameAlreadyExists = errors.New("user_name already exists")
)

var constraintMap = map[string]error{
	model.ConstraintUserEmail: repoErrEmailAlreadyExists,
	model.ConstraintUserName:  repoErrUserNameAlreadyExists,
}

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db}
}

func (repo *Repository) CreateUser(ctx context.Context, user *model.User) error {
	result := repo.db.WithContext(ctx).Create(user)
	if result.Error != nil {
		var pgErr *pgconn.PgError
		if errors.As(result.Error, &pgErr) && pgErr.SQLState() == pgerrcode.UniqueViolation {
			if businessErr, ok := constraintMap[pgErr.ConstraintName]; ok {
				return businessErr
			}
		}
		return fmt.Errorf("execute query error %w", result.Error)
	}

	return nil
}
