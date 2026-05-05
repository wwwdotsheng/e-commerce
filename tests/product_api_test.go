package tests

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/goccy/go-json"
	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("ProductAPI", func() {
	It("CreateProduct", func() {
		dto := map[string]string{
			"email":    "email",
			"password": "password",
		}
		body, _ := json.Marshal(dto)
		req, _ := http.NewRequest(http.MethodPost, "/api/v1/product/create", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		testRouter.ServeHTTP(w, req)

		var resp Response
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		fmt.Fprintf(GinkgoWriter, "%s", w.Body.String())
	})
})
