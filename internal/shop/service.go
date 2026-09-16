package shop

import (
	"context"
	"e-commerce/internal/model"

	"github.com/google/uuid"
)

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Create(ctx context.Context, ownerID uuid.UUID, name, desc string) (*model.Shop, error) {
	shop := &model.Shop{
		OwnerID:     ownerID,
		Name:        name,
		Description: desc,
	}
	if err := s.repo.Create(ctx, shop); err != nil {
		return nil, err
	}
	return shop, nil
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (*model.Shop, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *Service) List(ctx context.Context, pageNum, pageSize int) ([]*model.Shop, int64, error) {
	return s.repo.List(ctx, pageNum, pageSize)
}
