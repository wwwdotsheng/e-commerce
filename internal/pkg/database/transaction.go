package database

import (
	"context"

	"gorm.io/gorm"
)

type txCtxKeyStruct struct{}

var TxCtxKey = txCtxKeyStruct{}

type TransFunc func(ctx context.Context) error

func ExecuteTransaction(ctx context.Context, db *gorm.DB, fn TransFunc) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		ctxWithTx := context.WithValue(ctx, TxCtxKey, tx)
		return fn(ctxWithTx)
	})
}
