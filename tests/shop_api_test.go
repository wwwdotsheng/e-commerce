package tests

import (
	"bytes"
	"e-commerce/pkg/errno"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// ShopApi 覆盖 /api/v1/shop/* 三个接口。这个模块之前没有测试。
//
// 注意 list 接口的分页参数是 required（和 coupon/list 的 omitempty 不一样），
// 不传会直接返回 A00001。
var _ = Describe("ShopApi", Ordered, func() {
	var (
		shopToken string
		shopEmail string
		shopID    string
	)

	// doShop 发一个请求。withAuth=true 时带裸 token。
	doShop := func(method, path string, body interface{}, withAuth bool) (*httptest.ResponseRecorder, Response) {
		buf := bytes.NewBuffer(nil)
		if body != nil {
			b, _ := json.Marshal(body)
			buf = bytes.NewBuffer(b)
		}
		req, _ := http.NewRequest(method, path, buf)
		req.Header.Set("Content-Type", "application/json")
		if withAuth {
			req.Header.Set("Authorization", shopToken)
		}
		w := httptest.NewRecorder()
		testRouter.ServeHTTP(w, req)

		var resp Response
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		return w, resp
	}

	BeforeAll(func() {
		userName := "shop_" + uuid.New().String()[:8]
		shopEmail = userName + "@test.com"

		regBody, _ := json.Marshal(map[string]string{
			"user_name": userName,
			"email":     shopEmail,
			"password":  "test123456",
		})
		regReq, _ := http.NewRequest(http.MethodPost, "/api/v1/user/register", bytes.NewBuffer(regBody))
		regReq.Header.Set("Content-Type", "application/json")
		testRouter.ServeHTTP(httptest.NewRecorder(), regReq)

		_, resp := doLogin(shopEmail, "test123456")
		var loginData struct {
			AccessToken string `json:"access_token"`
		}
		_ = json.Unmarshal(resp.Data, &loginData)
		shopToken = loginData.AccessToken
		Expect(shopToken).NotTo(BeEmpty(), "拿不到 token，后续用例无法进行")
	})

	Describe("POST /api/v1/shop/create", func() {
		It("创建店铺成功", func() {
			w, resp := doShop(http.MethodPost, "/api/v1/shop/create", map[string]interface{}{
				"name":        "测试店铺-" + uuid.New().String()[:6],
				"description": "集成测试创建的店铺",
			}, true)
			Expect(w.Code).To(Equal(http.StatusOK))
			Expect(resp.Code).To(Equal(errno.OK.FullCode()))

			var data struct {
				ID string `json:"id"`
			}
			_ = json.Unmarshal(resp.Data, &data)
			Expect(data.ID).NotTo(BeEmpty())
			shopID = data.ID

			// 落库验证
			var cnt int64
			testDB.Table("shops").Where("id = ?", data.ID).Count(&cnt)
			Expect(cnt).To(Equal(int64(1)))
		})

		It("description 可以省略", func() {
			_, resp := doShop(http.MethodPost, "/api/v1/shop/create", map[string]interface{}{
				"name": "无描述店铺-" + uuid.New().String()[:6],
			}, true)
			Expect(resp.Code).To(Equal(errno.OK.FullCode()))
		})

		DescribeTable("参数校验失败应返回 A00001",
			func(body map[string]interface{}) {
				w, resp := doShop(http.MethodPost, "/api/v1/shop/create", body, true)
				Expect(w.Code).To(Equal(http.StatusOK))
				Expect(resp.Code).To(Equal(errno.ErrInvalidParam.FullCode()))
			},
			Entry("缺 name", map[string]interface{}{"description": "只有描述"}),
			Entry("name 只有 1 个字符", map[string]interface{}{"name": "a"}),
			Entry("description 超过 500 字符", map[string]interface{}{
				"name":        "正常名字",
				"description": strings.Repeat("x", 501),
			}),
		)
	})

	Describe("GET /api/v1/shop/list", func() {
		It("带分页参数查询成功", func() {
			w, resp := doShop(http.MethodGet, "/api/v1/shop/list?page_num=1&page_size=10", nil, true)
			Expect(w.Code).To(Equal(http.StatusOK))
			Expect(resp.Code).To(Equal(errno.OK.FullCode()))
		})

		It("不带分页参数应返回 A00001（这个接口的分页是 required）", func() {
			_, resp := doShop(http.MethodGet, "/api/v1/shop/list", nil, true)
			Expect(resp.Code).To(Equal(errno.ErrInvalidParam.FullCode()))
		})

		It("page_size 超过上限 20 应返回 A00001", func() {
			_, resp := doShop(http.MethodGet, "/api/v1/shop/list?page_num=1&page_size=100", nil, true)
			Expect(resp.Code).To(Equal(errno.ErrInvalidParam.FullCode()))
		})
	})

	Describe("GET /api/v1/shop/:id", func() {
		It("查询刚创建的店铺", func() {
			Expect(shopID).NotTo(BeEmpty(), "依赖前面创建店铺的用例")
			w, resp := doShop(http.MethodGet, "/api/v1/shop/"+shopID, nil, true)
			Expect(w.Code).To(Equal(http.StatusOK))
			Expect(resp.Code).To(Equal(errno.OK.FullCode()))
		})

		It("不存在的 ID 应返回 A00002（记录不存在），而不是 B01001", func() {
			_, resp := doShop(http.MethodGet, "/api/v1/shop/"+uuid.New().String(), nil, true)
			Expect(resp.Code).To(Equal(errno.ErrNotFoundRecord.FullCode()))
		})
	})

	Describe("认证", func() {
		It("不带 token 应返回 A02103", func() {
			_, resp := doShop(http.MethodGet, "/api/v1/shop/list?page_num=1&page_size=5", nil, false)
			Expect(resp.Code).To(Equal(errno.ErrAuthInvalidToken.FullCode()))
		})

		It("带伪造 token 应返回 A02103", func() {
			req, _ := http.NewRequest(http.MethodGet, "/api/v1/shop/list?page_num=1&page_size=5", nil)
			req.Header.Set("Authorization", "FAKE.TOKEN.HERE")
			w := httptest.NewRecorder()
			testRouter.ServeHTTP(w, req)

			var resp Response
			_ = json.Unmarshal(w.Body.Bytes(), &resp)
			Expect(resp.Code).To(Equal(errno.ErrAuthInvalidToken.FullCode()))
		})
	})
})
