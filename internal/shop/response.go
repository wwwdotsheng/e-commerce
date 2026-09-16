package shop

import "e-commerce/internal/model"

type Item struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	OwnerID     string `json:"owner_id"`
}

type ListResponse struct {
	Shops []Item `json:"shops"`
	Total int64  `json:"total"`
}

func FormatItem(s *model.Shop) *Item {
	return &Item{
		ID:          s.ID.String(),
		Name:        s.Name,
		Description: s.Description,
		OwnerID:     s.OwnerID.String(),
	}
}
