package product

import (
	"e-commerce/internal/model"
)

type Item struct {
	ID        string  `json:"id"`
	Publisher string  `json:"publisher"`
	ShopID    string  `json:"shop_id"`
	ShopName  string  `json:"shop_name"`
	Name      string  `json:"name"`
	Price     float64 `json:"price"`
	Status    string  `json:"status"`
	CreatedAt string  `json:"created_at"`
}

type Detail struct {
	Item
	Description string `json:"description"`
}

type ListProductsResponse struct {
	Products []Item `json:"products"`
	Total    int64  `json:"total"`
}

func FormatItem(p *model.Product, shopName string) *Item {
	shopID := ""
	if p.ShopID != nil {
		shopID = p.ShopID.String()
	}
	return &Item{
		ID:        p.ID.String(),
		Publisher: p.Publisher.String(),
		ShopID:    shopID,
		ShopName:  shopName,
		Name:      p.Name,
		Price:     p.Price,
		Status:    string(p.Status),
		CreatedAt: p.CreatedAt.Format("2006-01-02 15:04:05"),
	}
}

func FormatDetail(p *model.Product, shopName string) *Detail {
	shopID := ""
	if p.ShopID != nil {
		shopID = p.ShopID.String()
	}
	return &Detail{
		Item: Item{
			ID:        p.ID.String(),
			Publisher: p.Publisher.String(),
			ShopID:    shopID,
			ShopName:  shopName,
			Name:      p.Name,
			Price:     p.Price,
			Status:    string(p.Status),
			CreatedAt: p.CreatedAt.Format("2006-01-02 15:04:05"),
		},
		Description: p.Description,
	}
}
