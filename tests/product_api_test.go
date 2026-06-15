package tests

import (
	"bytes"
	"e-commerce/pkg/errno"
	"net/http"
	"net/http/httptest"

	"github.com/goccy/go-json"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ProductApi", Ordered, func() {
	var productID string
	var prodToken string

	BeforeAll(func() {
		prodUser := "prod_" + uuid.New().String()[:8]
		regBody, _ := json.Marshal(map[string]string{
			"user_name": prodUser,
			"email":     prodUser + "@test.com",
			"password":  "test123456",
		})
		regReq, _ := http.NewRequest(http.MethodPost, "/api/v1/user/register", bytes.NewBuffer(regBody))
		regReq.Header.Set("Content-Type", "application/json")
		testRouter.ServeHTTP(httptest.NewRecorder(), regReq)
		_, resp := doLogin(prodUser+"@test.com", "test123456")
		var loginData struct {
			AccessToken string `json:"access_token"`
		}
		json.Unmarshal(resp.Data, &loginData)
		prodToken = loginData.AccessToken
	})

	It("创建商品成功", func() {
		body, _ := json.Marshal(map[string]interface{}{
			"name":        "测试商品",
			"description": "测试描述",
			"price":       99.9,
			"status":      "active",
			"stock":       100,
		})
		req, _ := http.NewRequest(http.MethodPost, "/api/v1/product/create", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", prodToken)
		w := httptest.NewRecorder()
		testRouter.ServeHTTP(w, req)

		var resp Response
		json.Unmarshal(w.Body.Bytes(), &resp)
		Expect(resp.Code).To(Equal(errno.OK.FullCode()))
	})

	It("创建商品参数错误", func() {
		body, _ := json.Marshal(map[string]string{
			"name": "",
		})
		req, _ := http.NewRequest(http.MethodPost, "/api/v1/product/create", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", prodToken)
		w := httptest.NewRecorder()
		testRouter.ServeHTTP(w, req)

		var resp Response
		json.Unmarshal(w.Body.Bytes(), &resp)
		Expect(resp.Code).To(Equal(errno.ErrInvalidParam.FullCode()))
	})

	It("商品列表", func() {
		req, _ := http.NewRequest(http.MethodGet, "/api/v1/product/list?page_num=1&page_size=10", nil)
		req.Header.Set("Authorization", prodToken)
		w := httptest.NewRecorder()
		testRouter.ServeHTTP(w, req)

		var resp Response
		json.Unmarshal(w.Body.Bytes(), &resp)
		Expect(resp.Code).To(Equal(errno.OK.FullCode()))
		Expect(resp.Data).NotTo(BeNil())
	})

	It("创建商品并获取详情", func() {
		body, _ := json.Marshal(map[string]interface{}{
			"name":        "详情测试商品",
			"description": "详情测试描述",
			"price":       199.0,
			"status":      "active",
			"stock":       50,
		})
		req, _ := http.NewRequest(http.MethodPost, "/api/v1/product/create", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", prodToken)
		w := httptest.NewRecorder()
		testRouter.ServeHTTP(w, req)

		var createResp struct {
			Code string `json:"code"`
		}
		json.Unmarshal(w.Body.Bytes(), &createResp)
		Expect(createResp.Code).To(Equal(errno.OK.FullCode()))

		listReq, _ := http.NewRequest(http.MethodGet, "/api/v1/product/list?page_num=1&page_size=10", nil)
		listReq.Header.Set("Authorization", prodToken)
		lw := httptest.NewRecorder()
		testRouter.ServeHTTP(lw, listReq)

		var listResp struct {
			Code string `json:"code"`
			Data struct {
				Products []struct {
					ID string `json:"id"`
				} `json:"products"`
			} `json:"data"`
		}
		json.Unmarshal(lw.Body.Bytes(), &listResp)
		Expect(listResp.Code).To(Equal(errno.OK.FullCode()))
		Expect(listResp.Data.Products).NotTo(BeEmpty())

		productID = listResp.Data.Products[0].ID

		detailReq, _ := http.NewRequest(http.MethodGet, "/api/v1/product/"+productID, nil)
		detailReq.Header.Set("Authorization", prodToken)
		dw := httptest.NewRecorder()
		testRouter.ServeHTTP(dw, detailReq)

		var detailResp struct {
			Code string `json:"code"`
			Data struct {
				Name string `json:"name"`
			} `json:"data"`
		}
		json.Unmarshal(dw.Body.Bytes(), &detailResp)
		Expect(detailResp.Code).To(Equal(errno.OK.FullCode()))
		Expect(detailResp.Data.Name).To(Equal("详情测试商品"))
	})

	It("更新商品属性成功", func() {
		body, _ := json.Marshal(map[string]interface{}{
			"name":  "更新后的商品名",
			"price": 299.0,
		})
		req, _ := http.NewRequest(http.MethodPatch, "/api/v1/product/"+productID, bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", prodToken)
		w := httptest.NewRecorder()
		testRouter.ServeHTTP(w, req)

		var resp Response
		json.Unmarshal(w.Body.Bytes(), &resp)
		Expect(resp.Code).To(Equal(errno.OK.FullCode()))
	})

	It("更新商品状态成功", func() {
		body, _ := json.Marshal(map[string]string{
			"status": "inactive",
		})
		req, _ := http.NewRequest(http.MethodPost, "/api/v1/product/"+productID+"/status", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", prodToken)
		w := httptest.NewRecorder()
		testRouter.ServeHTTP(w, req)

		var resp Response
		json.Unmarshal(w.Body.Bytes(), &resp)
		Expect(resp.Code).To(Equal(errno.OK.FullCode()))
	})

	It("删除商品成功", func() {
		req, _ := http.NewRequest(http.MethodDelete, "/api/v1/product/"+productID, nil)
		req.Header.Set("Authorization", prodToken)
		w := httptest.NewRecorder()
		testRouter.ServeHTTP(w, req)

		var resp Response
		json.Unmarshal(w.Body.Bytes(), &resp)
		Expect(resp.Code).To(Equal(errno.OK.FullCode()))
	})
})
