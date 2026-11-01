package wallet

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"e-commerce/internal/app/identity"
	"e-commerce/internal/config"
	"e-commerce/internal/model"
	"e-commerce/internal/pkg/contextx"
	"e-commerce/pkg/clog"
	"e-commerce/pkg/errno"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.uber.org/zap"
)

type depositResponse struct {
	Code string `json:"code"`
}

func newDepositRouter(handler *Handler, withIdentity bool) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(contextx.SetConfig(&config.AppConfig{}))
	r.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(clog.WithLogger(c.Request.Context(), zap.NewNop()))
		c.Next()
	})
	if withIdentity {
		// 注入已登录买家身份，让请求能走到 service 层校验。
		r.Use(func(c *gin.Context) {
			account := &identity.AccountInfo{
				AccountId: uuid.New(),
				UserType:  model.UserTypeBuyer,
				Status:    model.UserStatusActive,
				SessionID: "test-session",
			}
			c.Request = c.Request.WithContext(identity.SetAccountInfo(c.Request.Context(), account))
			c.Next()
		})
	}
	r.POST("/deposit", handler.Deposit)
	return r
}

func performDeposit(t *testing.T, r *gin.Engine, body string) depositResponse {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/deposit", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	var resp depositResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode deposit response: %v; body: %s", err, w.Body.String())
	}
	return resp
}

// 幂等键为空或超过列宽时由参数绑定拦下，但错误码要和业务层保持一致。
func TestDepositIdempotencyKeyBindingUsesBusinessCode(t *testing.T) {
	r := newDepositRouter(NewHandler(nil), false)

	cases := []struct {
		name string
		body string
	}{
		{"空键", `{"amount":10}`},
		{"超过列宽", `{"amount":10,"idempotency_key":"` + strings.Repeat("x", walletLogIdempotencyKeyMaxLen+1) + `"}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			resp := performDeposit(t, r, c.body)
			if resp.Code != errno.ErrWalletInvalidIdempotencyKey.FullCode() {
				t.Fatalf("code=%q，期望=%q", resp.Code, errno.ErrWalletInvalidIdempotencyKey.FullCode())
			}
		})
	}

	t.Run("非法 JSON 仍是参数错误", func(t *testing.T) {
		resp := performDeposit(t, r, `{"amount":`)
		if resp.Code != errno.ErrInvalidParam.FullCode() {
			t.Fatalf("code=%q，期望=%q", resp.Code, errno.ErrInvalidParam.FullCode())
		}
	})
}

// 字符集与保留命名空间的校验发生在写库之前，因此可以用 nil 仓储验证。
func TestDepositRejectsIllegalIdempotencyKeyBeforeWriting(t *testing.T) {
	metrics, err := NewMetrics(otel.Meter("wallet-handler-test"))
	if err != nil {
		t.Fatalf("初始化指标失败: %v", err)
	}
	r := newDepositRouter(NewHandler(NewService(nil, metrics)), true)

	cases := []struct {
		name string
		key  string
	}{
		{"占用订单保留前缀", orderLogKeyPrefix + uuid.NewString() + ":payment"},
		{"含引号以外的非法字符", "deposit key"},
		{"含中文字符", "充值-2026"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			resp := performDeposit(t, r, `{"amount":10,"idempotency_key":"`+c.key+`"}`)
			if resp.Code != errno.ErrWalletInvalidIdempotencyKey.FullCode() {
				t.Fatalf("code=%q，期望=%q", resp.Code, errno.ErrWalletInvalidIdempotencyKey.FullCode())
			}
		})
	}
}
