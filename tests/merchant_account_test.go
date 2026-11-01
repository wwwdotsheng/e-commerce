package tests

import (
	"e-commerce/internal/model"
	"e-commerce/pkg/errno"
	"encoding/json"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/shopspring/decimal"
	"golang.org/x/crypto/bcrypt"
	"net/http"
	"net/http/httptest"
)

var _ = Describe("商家账号直接管理商品", func() {
	It("独立店铺接口不再开放", func() {
		for _, path := range []string{"/api/v1/shop/create", "/api/v1/shop/list", "/api/v1/shop/" + uuid.NewString()} {
			method := http.MethodGet
			if path == "/api/v1/shop/create" {
				method = http.MethodPost
			}
			w := httptest.NewRecorder()
			testRouter.ServeHTTP(w, httptest.NewRequest(method, path, nil))
			Expect(w.Code).To(Equal(http.StatusNotFound))
		}
	})

	It("发布商品无需店铺且不能伪造商家身份或修改其他商家商品", func() {
		hash, err := bcrypt.GenerateFromPassword([]byte("merchant-password"), bcrypt.MinCost)
		Expect(err).NotTo(HaveOccurred())
		create := func() (model.User, string) {
			id := uuid.New()
			u := model.User{ID: id, UserName: "seller_" + id.String()[:8], Email: id.String() + "@example.com", Password: string(hash), UserType: model.UserTypeSeller, Status: model.UserStatusActive, SellerName: "直接发布的商家"}
			Expect(testDB.Create(&u).Error).To(Succeed())
			r := invariantRequest(http.MethodPost, "/api/v1/auth/login", "", map[string]string{"email": u.Email, "password": "merchant-password"})
			Expect(r.Code).To(Equal(errno.OK.FullCode()))
			var p struct {
				Token string `json:"access_token"`
			}
			Expect(json.Unmarshal(r.Data, &p)).To(Succeed())
			return u, p.Token
		}
		seller, token := create()
		other, otherToken := create()
		name := "测试商品_" + uuid.NewString()
		r := invariantRequest(http.MethodPost, "/api/v1/product/create", token, map[string]interface{}{"name": name, "description": "不需要店铺", "price": 10, "stock": 5, "status": "active", "seller_id": other.ID.String()})
		Expect(r.Code).To(Equal(errno.OK.FullCode()))
		var p model.Product
		Expect(testDB.Where("name = ?", name).First(&p).Error).To(Succeed())
		Expect(p.SellerID).To(Equal(seller.ID))
		detail := invariantRequest(http.MethodGet, "/api/v1/product/"+p.ID.String(), token, nil)
		Expect(detail.Code).To(Equal(errno.OK.FullCode()))
		var fields map[string]interface{}
		Expect(json.Unmarshal(detail.Data, &fields)).To(Succeed())
		Expect(fields["seller_id"]).To(Equal(seller.ID.String()))
		Expect(fields["seller_name"]).To(Equal(seller.SellerName))
		Expect(fields).NotTo(HaveKey("shop_id"))
		Expect(fields).NotTo(HaveKey("shop_name"))
		Expect(fields).NotTo(HaveKey("publisher"))
		Expect(invariantRequest(http.MethodPatch, "/api/v1/product/"+p.ID.String(), otherToken, map[string]interface{}{"price": 20}).Code).To(Equal(errno.ErrProductNotFound.FullCode()))
		Expect(invariantRequest(http.MethodDelete, "/api/v1/product/"+p.ID.String(), otherToken, nil).Code).To(Equal(errno.ErrProductNotFound.FullCode()))
		Expect(testDB.First(&p, "id = ?", p.ID).Error).To(Succeed())
		Expect(p.Price.Equal(decimal.NewFromInt(10))).To(BeTrue())
		Expect(p.Status).To(Equal(model.ProductStatusActive))
	})
})
