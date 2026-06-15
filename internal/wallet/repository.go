package wallet

import (
	"context"
	"e-commerce/internal/model"
	"e-commerce/internal/pkg/database"
	"errors"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	repoErrDepositRecordAlreadyExists = errors.New("deposit record already exists")
)

var constraintMap = map[string]error{
	model.ConstraintWalletLogIdempotencyKey: repoErrDepositRecordAlreadyExists,
}

type Repository struct {
	*database.BaseRepo
	rdb *redis.Client
}

func NewRepository(db *gorm.DB, rdb *redis.Client) *Repository {
	return &Repository{BaseRepo: database.NewBaseRepo(db), rdb: rdb}
}

func (repo *Repository) CreateDefaultAccount(ctx context.Context, userID uuid.UUID) error {
	record := &model.UserWallet{
		UserID:    userID,
		Balance:   0.00,
		UpdatedAt: time.Now(),
	}
	return repo.GetDB(ctx).Create(record).Error
}

func (repo *Repository) Deposit(ctx context.Context, userID uuid.UUID, sessionID string, input *DepositInput) error {
	log := &model.WalletLog{
		UserID:         userID,
		SessionID:      sessionID,
		Amount:         input.Amount,
		Type:           "deposit",
		IdempotencyKey: input.IdempotencyKey,
	}
	if err := repo.GetDB(ctx).Create(log).Error; err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.SQLState() == pgerrcode.UniqueViolation {
			if mapErr, exists := constraintMap[pgErr.ConstraintName]; exists {
				return mapErr
			}
		}
		return fmt.Errorf("execute query error %w", err)
	}

	return repo.GetDB(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"balance":    gorm.Expr("user_wallets.balance + ?", input.Amount),
			"updated_at": time.Now(),
		}),
	}).Create(&model.UserWallet{
		UserID:    userID,
		Balance:   input.Amount,
		UpdatedAt: time.Now(),
	}).Error
}
