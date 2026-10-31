package product

import "e-commerce/internal/model"

type Item struct {
	ID         string  `json:"id"`
	SellerID   string  `json:"seller_id"`
	SellerName string  `json:"seller_name"`
	Name       string  `json:"name"`
	Price      float64 `json:"price"`
	Status     string  `json:"status"`
	CreatedAt  string  `json:"created_at"`
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
	return &Item{ID: p.ID.String(), SellerID: p.SellerID.String(), SellerName: sellerName, Name: p.Name, Price: p.Price, Status: string(p.Status), CreatedAt: p.CreatedAt.Format("2006-01-02 15:04:05")}
}
func FormatDetail(p *model.Product, sellerName string) *Detail {
	return &Detail{Item: *FormatItem(p, sellerName), Description: p.Description}
}
