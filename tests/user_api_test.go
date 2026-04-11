package tests_test

import (
	"bytes"
	"e-commerce/pkg/errno"
	"encoding/json"
	"net/http"
	"net/http/httptest"

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
		req, _ := http.NewRequest(http.MethodPost, "/api/v1/user/register", bytes.NewBuffer(body))
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
		})
	})
})
