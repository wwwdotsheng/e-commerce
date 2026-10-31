package user

import (
	"e-commerce/internal/app/identity"
	"e-commerce/internal/model"
	"e-commerce/internal/pkg/response"
	"e-commerce/pkg/errno"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type sellerApplicationBody struct {
	ShopName        string `json:"shop_name" binding:"required,max=128"`
	ApplicationNote string `json:"application_note" binding:"required,max=1000"`
}

func (h *Handler) GetCurrentAccount(c *gin.Context) {
	a := identity.GetAccountInfo(c.Request.Context())
	result, err := h.userSvc.GetAccount(c.Request.Context(), a.AccountId)
	response.Write(c, err, result)
}
func (h *Handler) ResubmitSellerApplication(c *gin.Context) {
	var body sellerApplicationBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.WriteInvalidParam(c, err)
		return
	}
	a := identity.GetAccountInfo(c.Request.Context())
	response.Write(c, h.userSvc.ResubmitSellerApplication(c.Request.Context(), a.AccountId, body.ShopName, body.ApplicationNote), nil)
}
func (h *Handler) ListSellerApplications(c *gin.Context) {
	var query struct {
		Status   model.UserStatus `form:"status" binding:"omitempty,oneof=pending active rejected disabled"`
		PageNum  int              `form:"page_num" binding:"omitempty,min=1"`
		PageSize int              `form:"page_size" binding:"omitempty,min=1,max=50"`
	}
	if err := c.ShouldBindQuery(&query); err != nil {
		response.WriteInvalidParam(c, err)
		return
	}
	if query.Status == "" {
		query.Status = model.UserStatusPending
	}
	if query.PageNum == 0 {
		query.PageNum = 1
	}
	if query.PageSize == 0 {
		query.PageSize = 10
	}
	items, total, err := h.userSvc.ListSellerApplications(c.Request.Context(), query.Status, query.PageNum, query.PageSize)
	response.Write(c, err, gin.H{"applications": items, "total": total})
}
func (h *Handler) GetSellerApplication(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.WriteInvalidParam(c, err)
		return
	}
	item, err := h.userSvc.GetAccount(c.Request.Context(), id)
	if err == nil && item.UserType != model.UserTypeSeller {
		err = errno.ErrNotFoundRecord
	}
	response.Write(c, err, item)
}
func (h *Handler) ReviewSellerApplication(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.WriteInvalidParam(c, err)
		return
	}
	var body struct {
		Decision string `json:"decision" binding:"required,oneof=approve reject"`
		Reason   string `json:"reason" binding:"max=1000"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		response.WriteInvalidParam(c, err)
		return
	}
	a := identity.GetAccountInfo(c.Request.Context())
	response.Write(c, h.userSvc.ReviewSellerApplication(c.Request.Context(), a.AccountId, id, body.Decision, body.Reason), nil)
}
