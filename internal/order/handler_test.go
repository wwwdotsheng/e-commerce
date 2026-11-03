package order

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"e-commerce/internal/app/identity"
	"e-commerce/internal/config"
	"e-commerce/internal/model"
	"e-commerce/internal/pkg/contextx"
	"e-commerce/pkg/clog"
	"e-commerce/pkg/errno"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

type handlerTestResponse struct {
	Code string          `json:"code"`
	Data json.RawMessage `json:"data"`
}

type handlerOrderServiceStub struct {
	accountID  uuid.UUID
	order      *model.Order
	getErr     error
	gotUserID  uuid.UUID
	gotOrderID uuid.UUID
	getCalled  bool

	payErr       error
	payCalled    bool
	gotSessionID string

	refund    *model.Refund
	refundErr error
}

func (s *handlerOrderServiceStub) CreateOrder(context.Context, uuid.UUID, CreateOrderParam) error {
	return nil
}

func (s *handlerOrderServiceStub) ListOrders(context.Context, ListOrdersParam) ([]*model.Order, int64, error) {
	return nil, 0, nil
}

func (s *handlerOrderServiceStub) GetOrderByID(_ context.Context, userID, orderID uuid.UUID) (*model.Order, error) {
	s.getCalled = true
	s.gotUserID = userID
	s.gotOrderID = orderID
	return s.order, s.getErr
}

func (s *handlerOrderServiceStub) PayOrder(_ context.Context, userID uuid.UUID, sessionID string, orderID uuid.UUID) (*model.Order, error) {
	s.payCalled = true
	s.gotUserID = userID
	s.gotOrderID = orderID
	s.gotSessionID = sessionID
	return s.order, s.payErr
}

func (s *handlerOrderServiceStub) ApplyRefund(context.Context, uuid.UUID, uuid.UUID) (*model.Refund, error) {
	return s.refund, s.refundErr
}

func (s *handlerOrderServiceStub) GetOrderDetail(_ context.Context, userID, orderID uuid.UUID) (*OrderDetail, error) {
	s.getCalled = true
	s.gotUserID = userID
	s.gotOrderID = orderID
	return &OrderDetail{Order: s.order, Refund: s.refund}, s.getErr
}
func newHandlerTestRouter() (*gin.Engine, *handlerOrderServiceStub) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(contextx.SetConfig(&config.AppConfig{}))
	service := &handlerOrderServiceStub{accountID: uuid.New()}
	r.Use(func(c *gin.Context) {
		ctx := identity.SetAccountInfo(c.Request.Context(), &identity.AccountInfo{
			AccountId: service.accountID,
			SessionID: "handler-test-session",
		})
		c.Request = c.Request.WithContext(clog.WithLogger(ctx, zap.NewNop()))
		c.Next()
	})

	h := NewHandler(service)
	r.GET("/orders/:id", h.GetOrderByID)
	r.POST("/orders/:id/pay", h.PayOrder)
	return r, service
}

func performHandlerRequest(t *testing.T, r http.Handler, method, target, body string) handlerTestResponse {
	t.Helper()
	req := httptest.NewRequest(method, target, bytes.NewBufferString(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("HTTP status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
	var response handlerTestResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v; body: %s", err, w.Body.String())
	}
	return response
}

func TestGetOrderValidatesOrderID(t *testing.T) {
	r, service := newHandlerTestRouter()
	orderID := uuid.New()
	service.order = &model.Order{
		ID:            orderID,
		UserID:        service.accountID,
		ProductId:     uuid.New(),
		Quantity:      2,
		SnapshotTitle: "Test product",
		SnapshotPrice: decimal.RequireFromString("12.5"),
		PayableAmount: decimal.NewFromInt(25),
		Status:        model.OrderStatusProcessing,
		CreatedAt:     time.Date(2026, 10, 30, 12, 0, 0, 0, time.Local),
		UpdatedAt:     time.Date(2026, 10, 30, 12, 1, 0, 0, time.Local),
	}

	got := performHandlerRequest(t, r, http.MethodGet, "/orders/"+orderID.String(), "")
	if got.Code != errno.OK.FullCode() {
		t.Fatalf("response code = %q, want %q", got.Code, errno.OK.FullCode())
	}
	if !service.getCalled || service.gotUserID != service.accountID || service.gotOrderID != orderID {
		t.Fatalf("GetOrderByID called with user %s and order %s; want user %s and order %s", service.gotUserID, service.gotOrderID, service.accountID, orderID)
	}
	var detail DetailResponse
	if err := json.Unmarshal(got.Data, &detail); err != nil {
		t.Fatalf("decode detail response: %v", err)
	}
	if detail.ID != orderID.String() || detail.SnapshotTitle != "Test product" || !detail.TotalAmount.Equal(decimal.NewFromInt(25)) {
		t.Fatalf("unexpected detail response: %+v", detail)
	}

	service.getCalled = false
	invalid := performHandlerRequest(t, r, http.MethodGet, "/orders/not-a-uuid", "")
	if invalid.Code != errno.ErrInvalidParam.FullCode() {
		t.Fatalf("invalid UUID response code = %q, want %q", invalid.Code, errno.ErrInvalidParam.FullCode())
	}
	if service.getCalled {
		t.Fatal("GetOrderByID should not be called for an invalid UUID")
	}
}

func TestGetOrderReturnsServiceError(t *testing.T) {
	r, service := newHandlerTestRouter()
	service.getErr = errno.ErrNotFoundRecord
	got := performHandlerRequest(t, r, http.MethodGet, "/orders/"+uuid.NewString(), "")
	if got.Code != errno.ErrNotFoundRecord.FullCode() {
		t.Fatalf("response code = %q, want %q", got.Code, errno.ErrNotFoundRecord.FullCode())
	}
}

func TestPayOrderPassesIdentityAndReturnsOrder(t *testing.T) {
	r, service := newHandlerTestRouter()
	orderID := uuid.New()
	paidAt := time.Date(2026, 11, 1, 10, 0, 0, 0, time.Local)
	service.order = &model.Order{
		ID:            orderID,
		UserID:        service.accountID,
		ProductId:     uuid.New(),
		Quantity:      1,
		SnapshotTitle: "Paid product",
		SnapshotPrice: decimal.NewFromInt(30),
		PayableAmount: decimal.NewFromInt(30),
		Status:        model.OrderStatusPaid,
		PaidAt:        &paidAt,
		CreatedAt:     time.Date(2026, 10, 31, 12, 0, 0, 0, time.Local),
		UpdatedAt:     time.Date(2026, 10, 31, 12, 1, 0, 0, time.Local),
	}

	got := performHandlerRequest(t, r, http.MethodPost, "/orders/"+orderID.String()+"/pay", "")
	if got.Code != errno.OK.FullCode() {
		t.Fatalf("response code = %q, want %q", got.Code, errno.OK.FullCode())
	}
	if !service.payCalled {
		t.Fatal("PayOrder should be called")
	}
	if service.gotUserID != service.accountID || service.gotOrderID != orderID {
		t.Fatalf("PayOrder called with user %s and order %s; want user %s and order %s", service.gotUserID, service.gotOrderID, service.accountID, orderID)
	}
	if service.gotSessionID != "handler-test-session" {
		t.Fatalf("PayOrder session = %q, want the authenticated session", service.gotSessionID)
	}
	var detail DetailResponse
	if err := json.Unmarshal(got.Data, &detail); err != nil {
		t.Fatalf("decode pay response: %v", err)
	}
	if detail.Status != int(model.OrderStatusPaid) || detail.PaidAt == nil {
		t.Fatalf("unexpected pay response: %+v", detail)
	}
}

func TestPayOrderValidatesOrderIDAndReturnsServiceError(t *testing.T) {
	r, service := newHandlerTestRouter()

	invalid := performHandlerRequest(t, r, http.MethodPost, "/orders/not-a-uuid/pay", "")
	if invalid.Code != errno.ErrInvalidParam.FullCode() {
		t.Fatalf("invalid UUID response code = %q, want %q", invalid.Code, errno.ErrInvalidParam.FullCode())
	}
	if service.payCalled {
		t.Fatal("PayOrder should not be called for an invalid UUID")
	}

	service.payErr = errno.ErrWalletBalanceInsufficient
	got := performHandlerRequest(t, r, http.MethodPost, "/orders/"+uuid.NewString()+"/pay", "")
	if got.Code != errno.ErrWalletBalanceInsufficient.FullCode() {
		t.Fatalf("response code = %q, want %q", got.Code, errno.ErrWalletBalanceInsufficient.FullCode())
	}
}
