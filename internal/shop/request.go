package shop

type CreateShopBody struct {
	Name        string `json:"name" binding:"required,min=2,max=128"`
	Description string `json:"description" binding:"max=500"`
}

type ListShopsQuery struct {
	PageNum  int `form:"page_num" binding:"required,gt=0"`
	PageSize int `form:"page_size" binding:"required,max=20"`
}
