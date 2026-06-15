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

var _ = Describe("WalletApi", Ordered, func() {
	var walletToken string

	BeforeAll(func() {
		walletUser := "wallet_" + uuid.New().String()[:8]
		regBody, _ := json.Marshal(map[string]string{
			"user_name": walletUser,
			"email":     walletUser + "@test.com",
			"password":  "test123456",
		})
		regReq, _ := http.NewRequest(http.MethodPost, "/api/v1/user/register", bytes.NewBuffer(regBody))
		regReq.Header.Set("Content-Type", "application/json")
		testRouter.ServeHTTP(httptest.NewRecorder(), regReq)
		_, resp := doLogin(walletUser+"@test.com", "test123456")
		var loginData struct {
			AccessToken string `json:"access_token"`
		}
		json.Unmarshal(resp.Data, &loginData)
		walletToken = loginData.AccessToken
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
})
