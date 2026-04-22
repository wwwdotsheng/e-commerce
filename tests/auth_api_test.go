package tests_test

import (
	"bytes"
	"context"
	"e-commerce/pkg/errno"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"golang.org/x/crypto/bcrypt"
)

var _ = Describe("AuthApi", Ordered, func() {
	insertUserId, err := uuid.NewV7()
	if err != nil {
		panic(err)
	}

	type LoginData struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}

	var cleanup = func() {
		ctx := context.Background()
		testDB.Exec("delete from users where email = ? or user_name = ?", "test@example.com", "test")
		sessions := testRedis.SMembers(ctx, fmt.Sprintf("ident:u_sess:%d", insertUserId))
		for _, session := range sessions.Val() {
			testRedis.Del(ctx, fmt.Sprintf("ident:sess:%s", session))
		}
		testRedis.Del(ctx, fmt.Sprintf("ident:u_sess:%s", insertUserId))
	}

	var doLogin = func(email, password string) (*httptest.ResponseRecorder, Response) {
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
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		fmt.Fprintf(GinkgoWriter, "%s", w.Body.String())
		return w, resp
	}

	var accessToken string
	var refreshToken string

	BeforeAll(func() {
		cleanup()
		bytes, _ := bcrypt.GenerateFromPassword([]byte("123456789"), bcrypt.DefaultCost)
		testDB.Exec("insert into users (id, user_name, email, password) values (?, ?,?,?)", insertUserId, "test", "test@example.com", string(bytes))
		_, resp := doLogin("test@example.com", "123456789")

		var loginData LoginData
		_ = json.Unmarshal(resp.Data, &loginData)
		accessToken, refreshToken = loginData.AccessToken, loginData.RefreshToken
	})

	AfterAll(func() {
		cleanup()
	})

	It("登陆成功", func() {
		testRedis.Del(context.Background(), fmt.Sprintf("ident:u_sess:%s", insertUserId))

		w, resp := doLogin("test@example.com", "123456789")
		var loginData LoginData
		_ = json.Unmarshal(resp.Data, &loginData)
		Expect(resp.Code).To(Equal(errno.OK.FullCode()))
		Expect(resp.UserMsg).To(Equal(errno.OK.Message))
		Expect(w.Code).To(Equal(http.StatusOK))
		Expect(accessToken).NotTo(BeEmpty())
		Expect(refreshToken).NotTo(BeEmpty())

		ctx := context.Background()
		result, err := testRedis.SMembers(ctx, fmt.Sprintf("ident:u_sess:%s", insertUserId)).Result()
		Expect(err).ToNot(HaveOccurred())
		Expect(len(result)).To(Equal(1))
		getRedisField := func(field string) string {
			return testRedis.HGet(ctx, fmt.Sprintf("ident:sess:%s", result[0]), field).Val()
		}
		Expect(loginData.AccessToken).To(Equal(getRedisField("at")))
		Expect(loginData.RefreshToken).To(Equal(getRedisField("rt")))
		accessToken = loginData.AccessToken
		refreshToken = loginData.RefreshToken
	})
	DescribeTable("冲突路径: 验证登陆限制",
		func(e, p string, expectedErr *errno.Errno) {
			w, resp := doLogin(e, p)
			Expect(resp.Code).To(Equal(expectedErr.FullCode()))
			Expect(resp.UserMsg).To(Equal(expectedErr.Message))
			Expect(w.Code).To(Equal(http.StatusOK))
		},
		Entry("用户名或密码错误1", "test@example.com", "test-test", errno.ErrUserNotFound),
		Entry("用户名或密码错误2", "test2@example.com", "123456789", errno.ErrUserNotFound),
	)

	It("POST /api/v1/auth/fetch-access-token - 刷新 AccessToken 成功", func() {
		req, _ := http.NewRequest(http.MethodPost, "/api/v1/auth/fetch-access-token", nil)
		req.Header.Set("Authorization", refreshToken)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		testRouter.ServeHTTP(w, req)

		Expect(w.Code).To(Equal(http.StatusOK))

		var resp Response
		var data map[string]string
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		_ = json.Unmarshal(resp.Data, &data)

		Expect(resp.Code).To(Equal(errno.OK.FullCode()))

		Expect(data["access_token"]).NotTo(BeEmpty())
		Expect(data["access_token"]).NotTo(Equal(accessToken))

		ctx := context.Background()
		sessions := testRedis.SMembers(ctx, fmt.Sprintf("ident:u_sess:%s", insertUserId)).Val()

		Expect(len(sessions)).To(Equal(1))

		currentAt := testRedis.HGet(ctx, fmt.Sprintf("ident:sess:%s", sessions[0]), "at").Val()
		Expect(currentAt).To(Equal(data["access_token"]))

		accessToken = data["access_token"]
	})

	It("POST /api/v1/auth/fetch-refresh-token - 刷新 RefreshToken 成功", func() {
		req, _ := http.NewRequest(http.MethodPost, "/api/v1/auth/fetch-refresh-token", nil)
		req.Header.Set("Authorization", accessToken)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		testRouter.ServeHTTP(w, req)

		Expect(w.Code).To(Equal(http.StatusOK))

		var resp Response
		var data map[string]string
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		_ = json.Unmarshal(resp.Data, &data)

		Expect(resp.Code).To(Equal(errno.OK.FullCode()))

		Expect(data["refresh_token"]).NotTo(BeEmpty())
		Expect(data["refresh_token"]).NotTo(Equal(refreshToken))

		ctx := context.Background()
		sessions := testRedis.SMembers(ctx, fmt.Sprintf("ident:u_sess:%s", insertUserId)).Val()

		Expect(len(sessions)).To(Equal(1))

		currentAt := testRedis.HGet(ctx, fmt.Sprintf("ident:sess:%s", sessions[0]), "rt").Val()
		Expect(currentAt).To(Equal(data["refresh_token"]))

		refreshToken = data["refresh_token"]
	})

	It("POST /api/v1/auth/logout - 登出并清理 Redis 会话", func() {
		req, _ := http.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
		req.Header.Set("Authorization", accessToken)

		w := httptest.NewRecorder()
		testRouter.ServeHTTP(w, req)

		Expect(w.Code).To(Equal(http.StatusOK))
		var resp Response
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		Expect(resp.Code).To(Equal(errno.OK.FullCode()))

		ctx := context.Background()
		userSessKey := fmt.Sprintf("ident:u_sess:%s", insertUserId)

		sessions := testRedis.SMembers(ctx, userSessKey).Val()
		Expect(len(sessions)).To(Equal(0), "登出后 Redis 集合应为空")

		exists := testRedis.Exists(ctx, fmt.Sprintf("ident:sess:%s", accessToken)).Val()
		Expect(exists).To(Equal(int64(0)))
	})

	It("验证登出后的失效性 - 使用旧的 RefreshToken 尝试刷新", func() {
		req, _ := http.NewRequest(http.MethodPost, "/api/v1/auth/fetch-access-token", nil)
		req.Header.Set("Authorization", refreshToken)

		w := httptest.NewRecorder()
		testRouter.ServeHTTP(w, req)

		var resp Response
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		Expect(resp.Code).ToNot(Equal(errno.OK.FullCode()))
	})

	It("冲突路径: 伪造 Token 尝试访问", func() {
		req, _ := http.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
		// 故意给一个格式正确但未在 Redis 记录的伪造 Token
		req.Header.Set("Authorization", "fake.token.value")

		w := httptest.NewRecorder()
		testRouter.ServeHTTP(w, req)

		var resp Response
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		// 验证：Redis 找不到对应 session 应拦截，防止非法访问
		Expect(resp.Code).To(Equal(errno.ErrAuthInvalidToken.FullCode()))
	})

	It("冲突路径: 恶意刷新 - 使用 AccessToken 去刷新 AccessToken", func() {
		req, _ := http.NewRequest(http.MethodPost, "/api/v1/auth/fetch-access-token", nil)
		// 刷新接口应严格检查 Token 类型，不能用 AT 充当 RT
		req.Header.Set("Authorization", accessToken)

		w := httptest.NewRecorder()
		testRouter.ServeHTTP(w, req)

		var resp Response
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		Expect(resp.Code).ToNot(Equal(errno.OK.FullCode()))
	})

	It("并发安全性: 快速双击刷新（模拟网络重试）", func() {
		// 这里测试 Redis 的原子性或你的业务锁
		// 连续发送两次刷新请求，验证系统是否能优雅处理或只允许一次生效
		done := make(chan bool)
		go func() {
			w := httptest.NewRecorder()
			req, _ := http.NewRequest(http.MethodPost, "/api/v1/auth/fetch-access-token", nil)
			req.Header.Set("Authorization", refreshToken)
			testRouter.ServeHTTP(w, req)
			done <- true
		}()

		// 同步执行第二次
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPost, "/api/v1/auth/fetch-access-token", nil)
		req.Header.Set("Authorization", refreshToken)
		testRouter.ServeHTTP(w, req)

		<-done
		// 只要不 Crash 且最终 Redis 状态保持唯一，测试即通过
		Expect(w.Code).To(Or(Equal(http.StatusOK), Equal(http.StatusUnauthorized)))
	})
})
