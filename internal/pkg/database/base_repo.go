package database

import (
	"context"

	"gorm.io/gorm"
)

type BaseRepo struct {
	db *gorm.DB
}

func NewBaseRepo(db *gorm.DB) *BaseRepo {
	return &BaseRepo{db: db}
}

func (r *BaseRepo) GetDB(ctx context.Context) *gorm.DB {
	if tx, ok := ctx.Value(TxCtxKey).(*gorm.DB); ok {
		return tx
	}
	return r.db.WithContext(ctx)
}
