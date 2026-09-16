package shop

import (
	"context"
	"e-commerce/internal/model"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Create(ctx context.Context, s *model.Shop) error {
	return r.db.WithContext(ctx).Create(s).Error
}

func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (*model.Shop, error) {
	var s model.Shop
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&s).Error
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *Repository) List(ctx context.Context, pageNum, pageSize int) ([]*model.Shop, int64, error) {
	var shops []*model.Shop
	var total int64

	q := r.db.WithContext(ctx).Model(&model.Shop{})
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := q.
		Offset((pageNum - 1) * pageSize).
		Limit(pageSize).
		Order("created_at DESC").
		Find(&shops).Error
	return shops, total, err
}

func (r *Repository) ListByOwner(ctx context.Context, ownerID uuid.UUID) ([]*model.Shop, error) {
	var shops []*model.Shop
	err := r.db.WithContext(ctx).Where("owner_id = ?", ownerID).Find(&shops).Error
	return shops, err
}
