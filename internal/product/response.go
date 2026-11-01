package product

import (
	"e-commerce/internal/model"
	"time"

	"github.com/shopspring/decimal"
)

type Item struct {
	ID         string          `json:"id"`
	SellerID   string          `json:"seller_id"`
	SellerName string          `json:"seller_name"`
	Name       string          `json:"name"`
	Price      decimal.Decimal `json:"price"`
	Status     string          `json:"status"`
	CreatedAt  string          `json:"created_at"`
}
type Detail struct {
	Item
	Description string `json:"description"`
}
type ListProductsResponse struct {
	Products []Item `json:"products"`
	Total    int64  `json:"total"`
}

func FormatItem(p *model.Product, sellerName string) *Item {
	return &Item{ID: p.ID.String(), SellerID: p.SellerID.String(), SellerName: sellerName, Name: p.Name, Price: p.Price, Status: string(p.Status), CreatedAt: p.CreatedAt.Format(time.RFC3339)}
}
func FormatDetail(p *model.Product, sellerName string) *Detail {
	return &Detail{Item: *FormatItem(p, sellerName), Description: p.Description}
}
