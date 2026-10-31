package tests

import (
	"e-commerce/internal/model"
	"e-commerce/pkg/errno"
	"encoding/json"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"net/http"
	"strings"
	"sync"
)

var _ = Describe("卖家注册与审核", func() {
	It("校验审核权限、拒绝后重新提交、并发审核及禁用后的会话失效", func() {
		register := func(kind string) (string, string) {
			name := "review_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:16]
			email := name + "@example.com"
			r := invariantRequest(http.MethodPost, "/api/v1/user/register/"+kind, "", map[string]string{"user_name": name, "email": email, "password": "ReviewTest-password", "seller_name": "Review shop", "application_note": "Seller application", "user_type": "admin", "status": "active"})
			Expect(r.Code).To(Equal(errno.OK.FullCode()))
			var u model.User
			Expect(testDB.Where("email = ?", email).First(&u).Error).To(Succeed())
			Expect(u.UserType).To(Equal(model.UserType(kind)))
			return u.ID.String(), email
		}
		login := func(email string) string {
			r := invariantRequest(http.MethodPost, "/api/v1/auth/login", "", map[string]string{"email": email, "password": "ReviewTest-password"})
			Expect(r.Code).To(Equal(errno.OK.FullCode()))
			var p struct {
				Token string `json:"access_token"`
			}
			Expect(json.Unmarshal(r.Data, &p)).To(Succeed())
			return p.Token
		}
		adminID, adminEmail := register("buyer")
		Expect(testDB.Model(&model.User{}).Where("id = ?", adminID).Update("user_type", model.UserTypeAdmin).Error).To(Succeed())
		admin := login(adminEmail)
		sellerID, sellerEmail := register("seller")
		seller := login(sellerEmail)
		_, buyerEmail := register("buyer")
		buyer := login(buyerEmail)
		path := "/api/v1/admin/seller-applications/" + sellerID + "/review"
		Expect(invariantRequest(http.MethodPost, path, buyer, map[string]string{"decision": "approve"}).Code).To(Equal(errno.ErrAuthNotPermission.FullCode()))
		Expect(invariantRequest(http.MethodPost, path, seller, map[string]string{"decision": "approve"}).Code).To(Equal(errno.ErrAuthNotPermission.FullCode()))
		Expect(invariantRequest(http.MethodGet, "/api/v1/user/me", seller, nil).Code).To(Equal(errno.OK.FullCode()))
		Expect(invariantRequest(http.MethodPost, "/api/v1/product/create", seller, map[string]interface{}{"name": "审核前商品", "description": "测试商品", "price": 10, "stock": 1, "status": "active"}).Code).To(Equal(errno.ErrAuthNotPermission.FullCode()))
		Expect(invariantRequest(http.MethodPost, "/api/v1/product/create", buyer, map[string]interface{}{"name": "Test", "description": "Test", "price": 10, "stock": 1, "status": "active"}).Code).To(Equal(errno.ErrAuthNotPermission.FullCode()))
		Expect(invariantRequest(http.MethodGet, "/api/v1/admin/seller-applications?page_num=1&page_size=10", admin, nil).Code).To(Equal(errno.OK.FullCode()))
		detail := invariantRequest(http.MethodGet, "/api/v1/admin/seller-applications/"+sellerID, admin, nil)
		Expect(detail.Code).To(Equal(errno.OK.FullCode()))
		Expect(string(detail.Data)).NotTo(ContainSubstring("password"))
		Expect(invariantRequest(http.MethodPost, path, admin, map[string]string{"decision": "reject", "reason": "   "}).Code).To(Equal(errno.ErrInvalidParam.FullCode()))
		Expect(invariantRequest(http.MethodPost, path, admin, map[string]string{"decision": "reject", "reason": "Please clarify the application"}).Code).To(Equal(errno.OK.FullCode()))
		Expect(invariantRequest(http.MethodPost, path, admin, map[string]string{"decision": "approve"}).Code).To(Equal(errno.ErrSellerApplicationState.FullCode()))
		me := invariantRequest(http.MethodGet, "/api/v1/user/me", seller, nil)
		Expect(string(me.Data)).To(ContainSubstring("Please clarify"))
		Expect(invariantRequest(http.MethodPatch, "/api/v1/user/seller-application", seller, map[string]string{"seller_name": "Updated shop", "application_note": "Updated application"}).Code).To(Equal(errno.OK.FullCode()))
		Expect(invariantRequest(http.MethodPatch, "/api/v1/user/seller-application", seller, map[string]string{"seller_name": "Again", "application_note": "Again"}).Code).To(Equal(errno.ErrSellerApplicationState.FullCode()))
		results := make(chan string, 2)
		var wg sync.WaitGroup
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func() {
				defer GinkgoRecover()
				defer wg.Done()
				results <- invariantRequest(http.MethodPost, path, admin, map[string]string{"decision": "approve"}).Code
			}()
		}
		wg.Wait()
		close(results)
		codes := []string{}
		for code := range results {
			codes = append(codes, code)
		}
		Expect(codes).To(ConsistOf(errno.OK.FullCode(), errno.ErrSellerApplicationState.FullCode()))
		var count int64
		Expect(testDB.Table("seller_application_reviews").Where("user_id = ?", sellerID).Count(&count).Error).To(Succeed())
		Expect(count).To(Equal(int64(2)))
		var u model.User
		Expect(testDB.First(&u, "id = ?", sellerID).Error).To(Succeed())
		Expect(u.Status).To(Equal(model.UserStatusActive))
		Expect(u.ReviewedBy.String()).To(Equal(adminID))
		Expect(invariantRequest(http.MethodPost, "/api/v1/product/create", seller, map[string]interface{}{"name": "审核后直接发布商品", "description": "无需店铺", "price": 10, "stock": 1, "status": "active"}).Code).To(Equal(errno.OK.FullCode()))
		Expect(testDB.Model(&model.User{}).Where("id = ?", sellerID).Update("status", model.UserStatusDisabled).Error).To(Succeed())
		Expect(invariantRequest(http.MethodPost, "/api/v1/auth/login", "", map[string]string{"email": sellerEmail, "password": "ReviewTest-password"}).Code).To(Equal(errno.ErrAuthNotPermission.FullCode()))
		Expect(invariantRequest(http.MethodGet, "/api/v1/user/me", seller, nil).Code).To(Equal(errno.ErrAuthNotPermission.FullCode()))
		Expect(testDB.Model(&model.User{}).Where("id = ?", sellerID).Update("status", model.UserStatusActive).Error).To(Succeed())
		Expect(invariantRequest(http.MethodGet, "/api/v1/user/me", seller, nil).Code).To(Equal(errno.ErrAuthInvalidToken.FullCode()))
	})
	It("卖家注册缺少申请资料时返回参数错误", func() {
		r := invariantRequest(http.MethodPost, "/api/v1/user/register/seller", "", map[string]string{"user_name": "missing_application", "email": "missing@example.com", "password": "test-password"})
		Expect(r.Code).To(Equal(errno.ErrInvalidParam.FullCode()))
	})
})
