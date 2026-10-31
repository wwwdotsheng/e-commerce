// Package acceptance verifies a deployed service through its HTTP ingress.
package acceptance

import (
	"bytes"
	"e-commerce/pkg/errno"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

type envelope struct {
	Code string          `json:"code"`
	Data json.RawMessage `json:"data"`
}
type apiClient struct {
	t          *testing.T
	base, host string
	client     *http.Client
}

func newClient(t *testing.T) *apiClient {
	t.Helper()
	base := strings.TrimRight(os.Getenv("API_TEST_BASE_URL"), "/")
	if base == "" {
		t.Skip("set API_TEST_BASE_URL to explicitly select a deployed service")
	}
	u, err := url.Parse(base)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Path != "" {
		t.Fatal("API_TEST_BASE_URL must be an http(s) origin without credentials, path, or query")
	}
	return &apiClient{t: t, base: base, host: os.Getenv("API_TEST_HOST"), client: &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}}
}
func (a *apiClient) request(method, path, token string, body any) []byte {
	a.t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			a.t.Fatal(err)
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, a.base+path, reader)
	if err != nil {
		a.t.Fatal(err)
	}
	if a.host != "" {
		req.Host = a.host
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", token)
	}
	resp, err := a.client.Do(req)
	if err != nil {
		a.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		a.t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		a.t.Fatalf("%s %s: HTTP %d", method, path, resp.StatusCode)
	}
	return raw
}
func (a *apiClient) business(method, path, token string, body any, want string) json.RawMessage {
	a.t.Helper()
	var r envelope
	if err := json.Unmarshal(a.request(method, path, token, body), &r); err != nil {
		a.t.Fatalf("%s: invalid JSON: %v", path, err)
	}
	if r.Code != want {
		a.t.Fatalf("%s: business code %q, expected %q", path, r.Code, want)
	}
	return r.Data
}
func decode[T any](t *testing.T, raw []byte) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	return v
}
func TestKubernetesSmoke(t *testing.T) {
	a := newClient(t)
	health := decode[struct {
		Status string `json:"status"`
	}](t, a.request("GET", "/health", "", nil))
	if health.Status != "ok" {
		t.Fatalf("health: %q", health.Status)
	}
	ready := decode[struct {
		Status map[string]string `json:"status"`
	}](t, a.request("GET", "/ready", "", nil))
	for _, name := range []string{"database", "redis", "rabbitmq"} {
		if ready.Status[name] != "ok" {
			t.Fatalf("%s readiness: %q", name, ready.Status[name])
		}
	}
	if !bytes.Contains(a.request("GET", "/swagger/doc.yaml", "", nil), []byte("openapi:")) {
		t.Fatal("OpenAPI document missing")
	}
	for _, token := range []string{"", "invalid-token"} {
		a.business("GET", "/api/v1/order/list?page_num=1&page_size=20", token, nil, errno.ErrAuthInvalidToken.FullCode())
	}
}

type account struct{ Token, RefreshToken, ID string }

func (a *apiClient) register(name string, seller ...bool) account {
	a.t.Helper()
	email := name + "@example.test"
	password := uuid.NewString()[:24]
	path := "/api/v1/user/register/buyer"
	body := map[string]string{"user_name": name, "email": email, "password": password}
	if len(seller) > 0 && seller[0] {
		path = "/api/v1/user/register/seller"
		body["shop_name"] = name
		body["application_note"] = "Acceptance test seller"
	}
	a.business("POST", path, "", body, errno.OK.FullCode())
	login := decode[struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}](a.t,
		a.business("POST", "/api/v1/auth/login", "", map[string]string{"email": email, "password": password}, errno.OK.FullCode()))
	if len(login.AccessToken) != 43 || login.RefreshToken == "" {
		a.t.Fatal("login did not return opaque tokens")
	}
	id := decode[struct {
		ID string `json:"id"`
	}](a.t, a.business("GET", "/api/v1/user/me", login.AccessToken, nil, errno.OK.FullCode())).ID
	if len(seller) > 0 && seller[0] {
		a.business("POST", "/api/v1/admin/seller-applications/"+id+"/review", os.Getenv("API_TEST_ADMIN_TOKEN"), map[string]string{"decision": "approve"}, errno.OK.FullCode())
	}

	return account{Token: login.AccessToken, RefreshToken: login.RefreshToken, ID: id}
}

type orderItem struct {
	ID        string  `json:"id"`
	ProductID string  `json:"product_id"`
	Status    int     `json:"status"`
	Quantity  int     `json:"quantity"`
	Discount  float64 `json:"discount_amount"`
}
type orders struct {
	Orders []orderItem `json:"orders"`
	Total  int64       `json:"total"`
}

func (a *apiClient) orders(token string) orders {
	return decode[orders](a.t, a.business("GET", "/api/v1/order/list?page_num=1&page_size=20", token, nil, errno.OK.FullCode()))
}
func (a *apiClient) awaitTimeout(token, orderID string) {
	a.t.Helper()
	deadline := time.Now().Add(75 * time.Second)
	for {
		for _, o := range a.orders(token).Orders {
			if o.ID == orderID && o.Status == 3 {
				return
			}
		}
		if time.Now().After(deadline) {
			a.t.Fatalf("order %s did not time out within 75s", orderID)
		}
		time.Sleep(200 * time.Millisecond)
	}
}
func TestKubernetesBusinessFlow(t *testing.T) {
	a := newClient(t)
	if os.Getenv("API_TEST_ALLOW_WRITES") != "1" {
		t.Skip("set API_TEST_ALLOW_WRITES=1 to create isolated test accounts and business data")
	}
	runID := "e2e_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	t.Logf("test data prefix: %s (accounts/orders/coupons retained; no deletion APIs)", runID)
	if os.Getenv("API_TEST_ADMIN_TOKEN") == "" {
		t.Fatal("API_TEST_ADMIN_TOKEN is required to approve the isolated seller account")
	}
	seller, buyer := a.register(runID+"_s", true), a.register(runID+"_b")
	t.Cleanup(func() {
		for _, u := range []account{seller, buyer} {
			r := decode[envelope](t, a.request("POST", "/api/v1/auth/logout", u.Token, nil))
			if r.Code != errno.OK.FullCode() {
				t.Errorf("test-session logout failed: %s", r.Code)
			}
			revoked := decode[envelope](t, a.request("GET", "/api/v1/order/list?page_num=1&page_size=20", u.Token, nil))
			if revoked.Code == errno.OK.FullCode() {
				t.Error("logged-out token still has order access")
			}
		}
	})
	refreshed := decode[struct {
		AccessToken string `json:"access_token"`
	}](t, a.business("POST", "/api/v1/auth/fetch-access-token", seller.RefreshToken, nil, errno.OK.FullCode()))
	if refreshed.AccessToken == "" {
		t.Fatal("refresh did not return an access token")
	}
	seller.Token = refreshed.AccessToken
	deposit := map[string]any{"amount": 100, "idempotency_key": runID + "_deposit"}
	a.business("POST", "/api/v1/wallet/deposit", buyer.Token, deposit, errno.OK.FullCode())
	a.business("POST", "/api/v1/wallet/deposit", buyer.Token, deposit, errno.OK.FullCode())
	productName := runID + "_product"
	a.business("POST", "/api/v1/product/create", seller.Token, map[string]any{"name": productName, "description": "Kubernetes acceptance fixture", "price": 100, "status": "active", "stock": 5}, errno.OK.FullCode())
	var productID string
	for page := 1; page <= 100 && productID == ""; page++ {
		list := decode[struct {
			Products []struct{ ID, Name, Publisher string }
			Total    int64
		}](t,
			a.business("GET", fmt.Sprintf("/api/v1/product/list?page_num=%d&page_size=20", page), seller.Token, nil, errno.OK.FullCode()))
		for _, p := range list.Products {
			if p.Name == productName && p.Publisher == seller.ID {
				productID = p.ID
			}
		}
		if len(list.Products) < 20 {
			break
		}
	}
	if productID == "" {
		t.Fatal("created product is missing from paginated list")
	}
	t.Cleanup(func() { a.business("DELETE", "/api/v1/product/"+productID, seller.Token, nil, errno.OK.FullCode()) })
	template := decode[struct {
		ID string `json:"id"`
	}](t, a.business("POST", "/api/v1/coupon/template", seller.Token,
		map[string]any{"name": runID + "_coupon", "type": "fixed_amount", "discount_value": 10, "min_amount": 100, "total_qty": 1, "per_user_limit": 1,
			"start_time": time.Now().UTC().Add(-time.Hour).Format(time.DateTime), "end_time": time.Now().UTC().Add(24 * time.Hour).Format(time.DateTime)}, errno.OK.FullCode()))
	if template.ID == "" {
		t.Fatal("missing coupon template ID")
	}
	held := decode[struct {
		ID string `json:"id"`
	}](t, a.business("POST", "/api/v1/coupon/grant", seller.Token, map[string]string{"template_id": template.ID, "user_id": buyer.ID}, errno.OK.FullCode()))
	if held.ID == "" {
		t.Fatal("missing user coupon ID")
	}
	payload := map[string]any{"product_id": productID, "quantity": 5, "coupon_id": held.ID, "idempotency_key": runID + "_order"}
	a.business("POST", "/api/v1/order/create", buyer.Token, payload, errno.OK.FullCode())
	a.business("POST", "/api/v1/order/create", buyer.Token, payload, errno.OK.FullCode())
	list := a.orders(buyer.Token)
	if list.Total != 1 || len(list.Orders) != 1 {
		t.Fatalf("idempotent order created %d records", list.Total)
	}
	first := list.Orders[0]
	if first.ProductID != productID || first.Quantity != 5 || first.Discount != 10 {
		t.Fatal("order snapshot or coupon deduction is incorrect")
	}
	if other := a.orders(seller.Token); other.Total != 0 {
		t.Fatal("seller can see buyer's orders")
	}
	a.business("POST", "/api/v1/order/create", buyer.Token, map[string]any{"product_id": productID, "quantity": 0, "idempotency_key": runID + "_invalid"}, errno.ErrInvalidParam.FullCode())
	a.awaitTimeout(buyer.Token, first.ID)
	coupons := decode[struct{ Coupons []struct{ ID, Status string } }](t, a.business("GET", "/api/v1/coupon/list?page_num=1&page_size=20", buyer.Token, nil, errno.OK.FullCode()))
	returned := false
	for _, c := range coupons.Coupons {
		if c.ID == held.ID && c.Status == "unused" {
			returned = true
		}
	}
	if !returned {
		t.Fatal("timed-out order did not return its coupon")
	}
	// Reusing all inventory plus the coupon verifies release without database access.
	payload["idempotency_key"] = runID + "_reorder"
	a.business("POST", "/api/v1/order/create", buyer.Token, payload, errno.OK.FullCode())
	second := a.orders(buyer.Token)
	if second.Total != 2 {
		t.Fatal("reorder missing")
	}
	for _, o := range second.Orders {
		if o.ID != first.ID {
			a.awaitTimeout(buyer.Token, o.ID)
		}
	}
	t.Log("registration/login, coupon order, replay, isolation, timeout, inventory/coupon reuse passed")
}
