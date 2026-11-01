package tests

import (
	"bytes"
	"e-commerce/internal/model"
	"e-commerce/pkg/errno"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("UserApi", Ordered, func() {
	var doRegister = func(userName, email, password string) (*httptest.ResponseRecorder, Response) {
		dto := map[string]string{
			"user_name": userName,
			"email":     email,
			"password":  password,
		}
		body, _ := json.Marshal(dto)
		req, _ := http.NewRequest(http.MethodPost, "/api/v1/user/register/buyer", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		testRouter.ServeHTTP(w, req)

		var resp Response
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		return w, resp
	}

	BeforeAll(func() {
		testDB.Exec("delete from users where email = ?", "test@example.com")
		testDB.Exec("delete from users where email = ?", "new@example.com")
	})

	Describe("Post /api/v1/user/register", func() {
		Context("业务规则验证", func() {
			It("创建用户", func() {
				w, resp := doRegister("test-user", "test@example.com", "123456789")

				Expect(resp.Code).To(Equal(errno.OK.FullCode()))
				Expect(resp.UserMsg).To(Equal(errno.OK.Message))
				Expect(w.Code).To(Equal(http.StatusOK))

				var count int64
				testDB.Table("users").Where("email = ?", "test@example.com").Count(&count)
				Expect(count).To(Equal(int64(1)))
			})

			DescribeTable("冲突路径: 验证注册限制",
				func(u, e, p string, expectedErr *errno.Errno) {
					w, resp := doRegister(u, e, p)
					Expect(w.Code).To(Equal(http.StatusOK))
					Expect(resp.Code).To(Equal(expectedErr.FullCode()))
					Expect(resp.UserMsg).To(Equal(expectedErr.Message))
				},
				Entry("邮箱重复注册", "new-user-name", "test@example.com", "123456789", errno.ErrUserEmailExisted),
				Entry("用户名重复注册", "test-user", "new@example.com", "123456789", errno.ErrUserNameExisted),

				Entry("密码格式验证", "test-user", "test@example.com", "123", errno.ErrInvalidParam),
				Entry("邮箱格式验证", "test-user", "test@", "123456789", errno.ErrInvalidParam),
				Entry("用户名格式验证", "1", "test@example.com", "123456789", errno.ErrInvalidParam),
			)

			It("并发重复注册只会成功一次，其余返回 A01102", func() {
				email := uuid.NewString() + "@example.com"
				body, _ := json.Marshal(map[string]string{
					"user_name": "race_" + uuid.NewString()[:8],
					"email":     email,
					"password":  "RaceTest-password",
				})

				const attempts = 6
				codes := make(chan string, attempts)
				var wg sync.WaitGroup
				for i := 0; i < attempts; i++ {
					wg.Add(1)
					go func() {
						defer GinkgoRecover()
						defer wg.Done()
						req, _ := http.NewRequest(http.MethodPost, "/api/v1/user/register/buyer", bytes.NewBuffer(body))
						req.Header.Set("Content-Type", "application/json")
						w := httptest.NewRecorder()
						testRouter.ServeHTTP(w, req)
						var resp Response
						_ = json.Unmarshal(w.Body.Bytes(), &resp)
						codes <- resp.Code
					}()
				}
				wg.Wait()
				close(codes)

				success := 0
				for code := range codes {
					if code == errno.OK.FullCode() {
						success++
						continue
					}
					// 邮箱和用户名重复共用 A01102，谁先命中由数据库唯一约束决定。
					Expect(code).To(Equal(errno.ErrUserEmailExisted.FullCode()))
				}
				Expect(success).To(Equal(1))

				var count int64
				Expect(testDB.Model(&model.User{}).Where("email = ?", email).Count(&count).Error).To(Succeed())
				Expect(count).To(Equal(int64(1)))

				DeferCleanup(func() {
					Expect(testDB.Exec("DELETE FROM user_wallets WHERE user_id IN (SELECT id FROM users WHERE email = ?)", email).Error).To(Succeed())
					Expect(testDB.Unscoped().Where("email = ?", email).Delete(&model.User{}).Error).To(Succeed())
				})
			})
		})
	})
})
