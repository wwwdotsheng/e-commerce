package tests

import (
	"bytes"
	"e-commerce/internal/model"
	"e-commerce/pkg/errno"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"golang.org/x/crypto/bcrypt"
)

type loginData struct {
	AccessToken string `json:"access_token"`
}

type orderItem struct {
	ID            string  `json:"id"`
	ProductID     string  `json:"product_id"`
	Quantity      int     `json:"quantity"`
	SnapshotTitle string  `json:"snapshot_title"`
	SnapshotPrice float64 `json:"snapshot_price"`
	Status        int     `json:"status"`
	CreatedAt     string  `json:"created_at"`
}

type listOrdersResponse struct {
	Orders []orderItem `json:"orders"`
	Total  int64       `json:"total"`
}

var _ = Describe("OrderApi", Ordered, func() {
	var (
		publisherID      uuid.UUID
		buyerID          uuid.UUID
		productID        uuid.UUID
		lowStockProduct  = uuid.New()
		accessToken      string
	)

	var doLogin = func(email, password string) string {
		dto := map[string]string{
			"email":    email,
			"password": password,
		}
		body, _ := json.Marshal(dto)
		req, _ := http.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		testRouter.ServeHTTP(w, req)

		var resp Response
		var data loginData
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		_ = json.Unmarshal(resp.Data, &data)
		return data.AccessToken
	}

	var doCreateOrder = func(token string, bodyMap map[string]interface{}) (*httptest.ResponseRecorder, Response) {
		body, _ := json.Marshal(bodyMap)
		req, _ := http.NewRequest(http.MethodPost, "/api/v1/order/create", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", token)
		w := httptest.NewRecorder()
		testRouter.ServeHTTP(w, req)

		var resp Response
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		return w, resp
	}

	var doListOrders = func(token string, params map[string]string) (*httptest.ResponseRecorder, Response) {
		req, _ := http.NewRequest(http.MethodGet, "/api/v1/order/list", nil)
		req.Header.Set("Authorization", token)
		q := req.URL.Query()
		for k, v := range params {
			q.Set(k, v)
		}
		req.URL.RawQuery = q.Encode()
		w := httptest.NewRecorder()
		testRouter.ServeHTTP(w, req)

		var resp Response
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		return w, resp
	}

	BeforeAll(func() {
		pwHash, _ := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.DefaultCost)

		publisherID = uuid.New()
		testDB.Exec(`INSERT INTO users (id, user_name, email, password, created_at, updated_at) VALUES (?, ?, ?, ?, NOW(), NOW())`,
			publisherID, "order-publisher", "order-pub@test.com", string(pwHash))

		buyerID = uuid.New()
		testDB.Exec(`INSERT INTO users (id, user_name, email, password, created_at, updated_at) VALUES (?, ?, ?, ?, NOW(), NOW())`,
			buyerID, "order-buyer", "order-buyer@test.com", string(pwHash))

		productID = uuid.New()
		testDB.Exec(`INSERT INTO products (id, publisher, name, description, price, stock, frozen_stock, status, version, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NOW(), NOW())`,
			productID, publisherID, "Test Item", "desc", 99.99, 10, 0, "active", 1)

		testDB.Exec(`INSERT INTO products (id, publisher, name, description, price, stock, frozen_stock, status, version, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NOW(), NOW())`,
			lowStockProduct, publisherID, "Low Stock", "desc", 9.99, 0, 0, "active", 1)

		accessToken = doLogin("order-buyer@test.com", "password123")
		Expect(accessToken).NotTo(BeEmpty())
	})

	AfterAll(func() {
		testDB.Exec("DELETE FROM orders WHERE user_id = ?", buyerID)
		testDB.Exec("DELETE FROM products WHERE id IN (?, ?)", productID, lowStockProduct)
		testDB.Exec("DELETE FROM users WHERE id IN (?, ?)", publisherID, buyerID)
	})

	Describe("POST /api/v1/order/create", func() {
		It("正常创建订单并扣减库存", func() {
			key := fmt.Sprintf("create-happy-%d", time.Now().UnixNano())
			_, resp := doCreateOrder(accessToken, map[string]interface{}{
				"product_id":      productID.String(),
				"quantity":        2,
				"idempotency_key": key,
			})
			Expect(resp.Code).To(Equal(errno.OK.FullCode()))

			var order model.Order
			err := testDB.Where("idempotency_key = ?", key).First(&order).Error
			Expect(err).ToNot(HaveOccurred())
			Expect(order.SnapshotTitle).To(Equal("Test Item"))
			Expect(order.SnapshotPrice).To(Equal(99.99))
			Expect(order.Quantity).To(Equal(2))
			Expect(order.Status).To(Equal(model.OrderStatusProcessing))

			var p model.Product
			testDB.Where("id = ?", productID).First(&p)
			Expect(p.Stock).To(Equal(8))
		})

		DescribeTable("冲突路径",
			func(bodyMap map[string]interface{}, expectedErr *errno.Errno) {
				_, resp := doCreateOrder(accessToken, bodyMap)
				Expect(resp.Code).To(Equal(expectedErr.FullCode()))
				Expect(resp.UserMsg).To(Equal(expectedErr.Message))
			},
			Entry("库存不足", map[string]interface{}{
				"product_id": lowStockProduct.String(), "quantity": 1, "idempotency_key": uuid.New().String(),
			}, errno.ErrProductStockInsufficient),
			Entry("商品不存在", map[string]interface{}{
				"product_id": uuid.New().String(), "quantity": 1, "idempotency_key": uuid.New().String(),
			}, errno.ErrOrderProductIdNotFound),
			Entry("缺少product_id", map[string]interface{}{
				"quantity": 1, "idempotency_key": uuid.New().String(),
			}, errno.ErrInvalidParam),
			Entry("数量为0", map[string]interface{}{
				"product_id": productID.String(), "quantity": 0, "idempotency_key": uuid.New().String(),
			}, errno.ErrInvalidParam),
		)

		It("幂等键重复返回冲突", func() {
			key := fmt.Sprintf("dup-key-%d", time.Now().UnixNano())
			_, resp1 := doCreateOrder(accessToken, map[string]interface{}{
				"product_id":      productID.String(),
				"quantity":        1,
				"idempotency_key": key,
			})
			Expect(resp1.Code).To(Equal(errno.OK.FullCode()))

			_, resp2 := doCreateOrder(accessToken, map[string]interface{}{
				"product_id":      productID.String(),
				"quantity":        1,
				"idempotency_key": key,
			})
			Expect(resp2.Code).To(Equal(errno.OK.FullCode()))
		})
	})

	Describe("GET /api/v1/order/list", func() {
		It("返回用户订单列表", func() {
				testDB.Exec("DELETE FROM orders WHERE user_id = ?", buyerID)
			for i := 0; i < 3; i++ {
				key := fmt.Sprintf("list-data-%d-%d", i, time.Now().UnixNano())
				doCreateOrder(accessToken, map[string]interface{}{
					"product_id":      productID.String(),
					"quantity":        1,
					"idempotency_key": key,
				})
			}

			_, resp := doListOrders(accessToken, map[string]string{
				"page_num": "1",
				"page_size": "2",
			})
			Expect(resp.Code).To(Equal(errno.OK.FullCode()))

			var listResp listOrdersResponse
			_ = json.Unmarshal(resp.Data, &listResp)
			Expect(len(listResp.Orders)).To(Equal(2))
			Expect(listResp.Total).To(Equal(int64(3)))
			Expect(listResp.Orders[0].SnapshotTitle).To(Equal("Test Item"))

			_, resp2 := doListOrders(accessToken, map[string]string{
				"page_num": "2",
				"page_size": "2",
			})
			Expect(resp2.Code).To(Equal(errno.OK.FullCode()))

			var listResp2 listOrdersResponse
			_ = json.Unmarshal(resp2.Data, &listResp2)
			Expect(len(listResp2.Orders)).To(Equal(1))
		})
	})
})