package tests

import (
	"bytes"
	"context"
	"e-commerce/internal/auth"
	"e-commerce/internal/config"
	"e-commerce/pkg/errno"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("OpaqueTokenLifecycle", func() {
	It("validates token types, rotation, expiry and logout against Redis", func() {
		ctx := context.Background()
		name := "opaque_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:16]
		email := name + "@example.com"
		payload, _ := json.Marshal(map[string]string{"user_name": name, "email": email, "password": "opaque-test-password"})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/user/register", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		testRouter.ServeHTTP(w, req)
		var response Response
		Expect(json.Unmarshal(w.Body.Bytes(), &response)).To(Succeed())
		Expect(response.Code).To(Equal(errno.OK.FullCode()))
		cfg := &config.AuthSection{AccessTokenExpire: time.Second, RefreshTokenExpire: 5 * time.Second}
		repo := auth.NewRepository(testDB, testRedis, cfg)
		svc := auth.NewService(repo, cfg)
		pair, err := svc.Login(ctx, &auth.LoginInput{Email: email, Password: "opaque-test-password"})
		Expect(err).NotTo(HaveOccurred())
		Expect(pair.AccessToken).To(HaveLen(43))
		Expect(pair.RefreshToken).To(HaveLen(43))
		account, err := svc.VerifyToken(ctx, pair.AccessToken, auth.AccessToken)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { Expect(repo.DelSession(ctx, account.AccountId, account.SessionID)).To(Succeed()) })
		_, err = svc.VerifyToken(ctx, pair.RefreshToken, auth.AccessToken)
		Expect(err).To(Equal(errno.ErrAuthInvalidToken))
		_, err = svc.VerifyToken(ctx, pair.AccessToken, auth.RefreshToken)
		Expect(err).To(Equal(errno.ErrAuthInvalidToken))
		newAccess := strings.Repeat("A", 43)
		newRefresh := strings.Repeat("B", 42) + "A"
		Expect(repo.SetAccessToken(ctx, account.AccountId, account.SessionID, newAccess)).To(Succeed())
		_, err = svc.VerifyToken(ctx, pair.AccessToken, auth.AccessToken)
		Expect(err).To(Equal(errno.ErrAuthInvalidToken))
		_, err = svc.VerifyToken(ctx, newAccess, auth.AccessToken)
		Expect(err).NotTo(HaveOccurred())
		Expect(repo.SetRefreshToken(ctx, account.AccountId, account.SessionID, newRefresh)).To(Succeed())
		_, err = svc.VerifyToken(ctx, pair.RefreshToken, auth.RefreshToken)
		Expect(err).To(Equal(errno.ErrAuthInvalidToken))
		_, err = svc.VerifyToken(ctx, newRefresh, auth.RefreshToken)
		Expect(err).NotTo(HaveOccurred())
		Eventually(func() error { _, err := svc.VerifyToken(ctx, newAccess, auth.AccessToken); return err }, 2*time.Second, 50*time.Millisecond).Should(Equal(errno.ErrAuthTokenExpired))
		_, err = svc.VerifyToken(ctx, newRefresh, auth.RefreshToken)
		Expect(err).NotTo(HaveOccurred())
		Expect(repo.DelSession(ctx, account.AccountId, account.SessionID)).To(Succeed())
		_, err = svc.VerifyToken(ctx, newRefresh, auth.RefreshToken)
		Expect(err).To(Equal(errno.ErrAuthInvalidToken))
		_, err = svc.VerifyToken(ctx, newAccess, auth.AccessToken)
		Expect(err).To(Equal(errno.ErrAuthInvalidToken))
	})
})
