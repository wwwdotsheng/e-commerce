package tests

import (
	"bytes"
	"e-commerce/internal/model"
	"e-commerce/pkg/errno"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/goccy/go-json"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("WalletApi", Ordered, func() {
	var walletToken string
	var walletUserID uuid.UUID

	BeforeAll(func() {
		walletUser := "wallet_" + uuid.New().String()[:8]
		regBody, _ := json.Marshal(map[string]string{
			"user_name": walletUser,
			"email":     walletUser + "@test.com",
			"password":  "test123456",
		})
		regReq, _ := http.NewRequest(http.MethodPost, "/api/v1/user/register/buyer", bytes.NewBuffer(regBody))
		regReq.Header.Set("Content-Type", "application/json")
		testRouter.ServeHTTP(httptest.NewRecorder(), regReq)
		_, resp := doLogin(walletUser+"@test.com", "test123456")
		var loginData struct {
			AccessToken string `json:"access_token"`
		}
		json.Unmarshal(resp.Data, &loginData)
		walletToken = loginData.AccessToken
		Expect(walletToken).NotTo(BeEmpty())

		var account model.User
		Expect(testDB.Where("email = ?", walletUser+"@test.com").First(&account).Error).To(Succeed())
		walletUserID = account.ID
	})

	It("钱包充值成功", func() {
		body, _ := json.Marshal(map[string]interface{}{
			"amount":          100.0,
			"idempotency_key": "wallet-test-" + uuid.New().String(),
		})
		req, _ := http.NewRequest(http.MethodPost, "/api/v1/wallet/deposit", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", walletToken)
		w := httptest.NewRecorder()
		testRouter.ServeHTTP(w, req)

		var resp Response
		json.Unmarshal(w.Body.Bytes(), &resp)
		Expect(resp.Code).To(Equal(errno.OK.FullCode()))
	})

	It("钱包充值金额非法", func() {
		body, _ := json.Marshal(map[string]interface{}{
			"amount":          -1,
			"idempotency_key": "wallet-invalid-" + uuid.New().String(),
		})
		req, _ := http.NewRequest(http.MethodPost, "/api/v1/wallet/deposit", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", walletToken)
		w := httptest.NewRecorder()
		testRouter.ServeHTTP(w, req)

		var resp Response
		json.Unmarshal(w.Body.Bytes(), &resp)
		Expect(resp.Code).To(Equal(errno.ErrWalletInvalidDepositAmount.FullCode()))
	})

	It("钱包充值幂等", func() {
		key := "wallet-idempotent-" + uuid.New().String()

		body, _ := json.Marshal(map[string]interface{}{
			"amount":          200.0,
			"idempotency_key": key,
		})
		req, _ := http.NewRequest(http.MethodPost, "/api/v1/wallet/deposit", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", walletToken)
		w := httptest.NewRecorder()
		testRouter.ServeHTTP(w, req)

		var resp1 Response
		json.Unmarshal(w.Body.Bytes(), &resp1)
		Expect(resp1.Code).To(Equal(errno.OK.FullCode()))

		req2, _ := http.NewRequest(http.MethodPost, "/api/v1/wallet/deposit", bytes.NewBuffer(body))
		req2.Header.Set("Content-Type", "application/json")
		req2.Header.Set("Authorization", walletToken)
		w2 := httptest.NewRecorder()
		testRouter.ServeHTTP(w2, req2)

		var resp2 Response
		json.Unmarshal(w2.Body.Bytes(), &resp2)

		Expect(resp2.Code).To(Equal(errno.OK.FullCode()))
	})

	It("钱包充值未鉴权", func() {
		body, _ := json.Marshal(map[string]interface{}{
			"amount":          100.0,
			"idempotency_key": "wallet-noauth-" + uuid.New().String(),
		})
		req, _ := http.NewRequest(http.MethodPost, "/api/v1/wallet/deposit", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		testRouter.ServeHTTP(w, req)

		var resp Response
		json.Unmarshal(w.Body.Bytes(), &resp)
		Expect(resp.Code).To(Equal(errno.ErrAuthInvalidToken.FullCode()))
	})

	It("充值幂等键非法时返回 A03103 且不落账", func() {
		deposit := func(key string) Response {
			GinkgoHelper()
			body, _ := json.Marshal(map[string]interface{}{
				"amount":          10.0,
				"idempotency_key": key,
			})
			req, _ := http.NewRequest(http.MethodPost, "/api/v1/wallet/deposit", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", walletToken)
			w := httptest.NewRecorder()
			testRouter.ServeHTTP(w, req)

			var resp Response
			Expect(json.Unmarshal(w.Body.Bytes(), &resp)).To(Succeed())
			return resp
		}
		balance := func() float64 {
			GinkgoHelper()
			req, _ := http.NewRequest(http.MethodGet, "/api/v1/wallet/balance", nil)
			req.Header.Set("Authorization", walletToken)
			w := httptest.NewRecorder()
			testRouter.ServeHTTP(w, req)

			var resp Response
			Expect(json.Unmarshal(w.Body.Bytes(), &resp)).To(Succeed())
			var data struct {
				Balance float64 `json:"balance"`
			}
			Expect(json.Unmarshal(resp.Data, &data)).To(Succeed())
			return data.Balance
		}
		logCount := func() int64 {
			GinkgoHelper()
			var count int64
			Expect(testDB.Model(&model.WalletLog{}).Where("user_id = ?", walletUserID).Count(&count).Error).To(Succeed())
			return count
		}

		beforeBalance, beforeLogs := balance(), logCount()
		for _, c := range []struct {
			name string
			key  string
		}{
			{"占用订单保留前缀", "order:" + uuid.NewString() + ":payment"},
			{"超过列宽", strings.Repeat("x", 65)},
			{"含非法字符", "deposit key"},
		} {
			Expect(deposit(c.key).Code).To(Equal(errno.ErrWalletInvalidIdempotencyKey.FullCode()), c.name)
		}
		Expect(balance()).To(Equal(beforeBalance))
		Expect(logCount()).To(Equal(beforeLogs))
	})
})
