package order

type CreateOrderBody struct {
	ProductID      string `json:"product_id" binding:"required"`
	Quantity       int    `json:"quantity" binding:"required,min=1"`
	IdempotencyKey string `json:"idempotency_key" binding:"required"`
}

type ListOrdersQuery struct {
	PageNum  int `form:"page_num" binding:"required,gt=0"`
	PageSize int `form:"page_size" binding:"required,max=20"`
}