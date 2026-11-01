package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// 登录路由挂了 IP 级令牌桶（5 req/s、burst 10）。这里用独立 IP 耗尽突发容量，
// 验证超限后返回 429 且响应体是可识别的限流错误。
var _ = Describe("登录限流", func() {
	It("同一 IP 超过突发容量后返回 429", func() {
		// 每个用例使用独立 IP，避免与其他用例共享同一个全局令牌桶。
		id := uuid.New()
		ip := fmt.Sprintf("198.51.%d.%d", int(id[1])%250+1, int(id[2])%250+1)

		doLogin := func() *httptest.ResponseRecorder {
			body := []byte(`{"email":"rate-limit@example.com","password":"wrong-password"}`)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.RemoteAddr = ip + ":4321"
			w := httptest.NewRecorder()
			testRouter.ServeHTTP(w, req)
			return w
		}

		// 先耗尽 burst；密码错误的登录在查库阶段就返回，速度足够快。
		for i := 0; i < 10; i++ {
			Expect(doLogin().Code).To(Equal(http.StatusOK))
		}

		// 令牌桶按 5 req/s 回填，允许极少量重试避免机器慢造成的偶发回填。
		limited := false
		for i := 0; i < 5 && !limited; i++ {
			w := doLogin()
			if w.Code != http.StatusTooManyRequests {
				continue
			}
			limited = true
			var payload map[string]string
			Expect(json.Unmarshal(w.Body.Bytes(), &payload)).To(Succeed())
			Expect(payload["error"]).To(Equal("rate_limit_exceeded"))
		}
		Expect(limited).To(BeTrue(), "连续请求未触发登录限流")
	})
})
