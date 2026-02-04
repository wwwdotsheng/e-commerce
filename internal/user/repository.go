package user

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

var (
	dbEmailAlreadyExists    = errors.New("email already exists")
	dbUserNameAlreadyExists = errors.New("user_name already exists")
	dbNotFoundUser          = errors.New("not found user")
)

var constraintMap = map[string]error{
	ConstraintUserEmail: dbEmailAlreadyExists,
	ConstraintUserName:  dbUserNameAlreadyExists,
}

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db}
}

func (repo *Repository) createUser(ctx context.Context, user *User) error {
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

func (repo *Repository) findUserByEmail(ctx context.Context, email string) (*User, error) {
	var user User
	result := repo.db.WithContext(ctx).Where("email = ?", email).First(&user)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, dbNotFoundUser
		}
		return nil, fmt.Errorf("execute query error %w", result.Error)
	}

	return &user, nil
}
