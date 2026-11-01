package tests

import (
	"bytes"
	"e-commerce/internal/model"
	"e-commerce/pkg/errno"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// CouponApi 覆盖 /api/v1/coupon/* 三个接口。
//
// 这个模块之前没有测试。它的 CreateTemplate 有一个容易踩的坑：
// start_time / end_time 用的是 "2006-01-02 15:04:05" 格式，不是 RFC3339。
// 传 RFC3339 会在 service 里 time.Parse 失败，被 response.Write 包成 B01001。
var _ = Describe("CouponApi", Ordered, func() {
	var (
		couponToken  string
		couponEmail  string
		couponUserID string
		templateID   string
	)

	// doCoupon 发一个请求。withAuth=true 时带裸 token（这个项目不加 Bearer 前缀）。
	doCoupon := func(method, path string, body interface{}, withAuth bool) (*httptest.ResponseRecorder, Response) {
		buf := bytes.NewBuffer(nil)
		if body != nil {
			b, _ := json.Marshal(body)
			buf = bytes.NewBuffer(b)
		}
		req, _ := http.NewRequest(method, path, buf)
		req.Header.Set("Content-Type", "application/json")
		if withAuth {
			req.Header.Set("Authorization", couponToken)
		}
		w := httptest.NewRecorder()
		testRouter.ServeHTTP(w, req)

		var resp Response
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		return w, resp
	}

	// validTemplate 给出一份合法的创建请求，可按需覆盖字段（传 nil 表示删掉该字段）
	validTemplate := func(overrides map[string]interface{}) map[string]interface{} {
		base := map[string]interface{}{
			"name":           "测试券-" + uuid.New().String()[:6],
			"type":           "fixed_amount",
			"discount_value": 10,
			"min_amount":     100,
			"total_qty":      100,
			"per_user_limit": 1,
			"start_time":     time.Now().Format("2006-01-02 15:04:05"),
			"end_time":       time.Now().AddDate(1, 0, 0).Format("2006-01-02 15:04:05"),
		}
		for k, v := range overrides {
			if v == nil {
				delete(base, k)
			} else {
				base[k] = v
			}
		}
		return base
	}

	BeforeAll(func() {
		userName := "cpn_" + uuid.New().String()[:8]
		couponEmail = userName + "@test.com"

		regBody, _ := json.Marshal(map[string]string{
			"user_name": userName,
			"email":     couponEmail,
			"password":  "test123456",
		})
		regReq, _ := http.NewRequest(http.MethodPost, "/api/v1/user/register/buyer", bytes.NewBuffer(regBody))
		regReq.Header.Set("Content-Type", "application/json")
		testRouter.ServeHTTP(httptest.NewRecorder(), regReq)

		_, resp := doLogin(couponEmail, "test123456")
		var loginData struct {
			AccessToken string `json:"access_token"`
		}
		_ = json.Unmarshal(resp.Data, &loginData)
		couponToken = loginData.AccessToken
		Expect(couponToken).NotTo(BeEmpty(), "拿不到 token，后续用例无法进行")

		var account model.User
		Expect(testDB.Where("email = ?", couponEmail).First(&account).Error).To(Succeed())
		couponUserID = account.ID.String()
		Expect(couponUserID).NotTo(BeEmpty(), "拿不到 account_id，grant 用例无法进行")
	})

	Describe("POST /api/v1/coupon/template", func() {
		It("创建满减券模板成功", func() {
			w, resp := doCoupon(http.MethodPost, "/api/v1/coupon/template", validTemplate(nil), true)
			Expect(w.Code).To(Equal(http.StatusOK))
			Expect(resp.Code).To(Equal(errno.OK.FullCode()))

			var data struct {
				ID string `json:"id"`
			}
			_ = json.Unmarshal(resp.Data, &data)
			Expect(data.ID).NotTo(BeEmpty())
			templateID = data.ID

			// 落库验证
			var cnt int64
			testDB.Table("coupon_template").Where("id = ?", data.ID).Count(&cnt)
			Expect(cnt).To(Equal(int64(1)))
		})

		It("创建折扣券模板成功", func() {
			_, resp := doCoupon(http.MethodPost, "/api/v1/coupon/template", validTemplate(map[string]interface{}{
				"type":          "percentage",
				"discount_rate": 0.8,
				"max_deduction": 50,
			}), true)
			Expect(resp.Code).To(Equal(errno.OK.FullCode()))
		})

		DescribeTable("参数校验失败应返回 A00001",
			func(overrides map[string]interface{}) {
				w, resp := doCoupon(http.MethodPost, "/api/v1/coupon/template", validTemplate(overrides), true)
				Expect(w.Code).To(Equal(http.StatusOK))
				Expect(resp.Code).To(Equal(errno.ErrInvalidParam.FullCode()))
			},
			Entry("缺 name", map[string]interface{}{"name": nil}),
			Entry("type 不在枚举内", map[string]interface{}{"type": "unknown"}),
			Entry("total_qty 为 0", map[string]interface{}{"total_qty": 0}),
			Entry("discount_rate 超过 1", map[string]interface{}{"type": "percentage", "discount_rate": 1.5}),
			Entry("缺 start_time", map[string]interface{}{"start_time": nil}),
		)

		It("接受带时区的 RFC3339 时间", func() {
			_, resp := doCoupon(http.MethodPost, "/api/v1/coupon/template", validTemplate(map[string]interface{}{
				"start_time": time.Now().Add(-time.Hour).Format(time.RFC3339),
				"end_time":   time.Now().AddDate(1, 0, 0).Format(time.RFC3339),
			}), true)
			Expect(resp.Code).To(Equal(errno.OK.FullCode()))
		})

		DescribeTable("时间非法或顺序错误应返回 A00001",
			func(overrides map[string]interface{}) {
				w, resp := doCoupon(http.MethodPost, "/api/v1/coupon/template", validTemplate(overrides), true)
				Expect(w.Code).To(Equal(http.StatusOK))
				Expect(resp.Code).To(Equal(errno.ErrInvalidParam.FullCode()))
			},
			Entry("start_time 非法日期", map[string]interface{}{
				"start_time": "2026-13-01 00:00:00",
			}),
			Entry("start_time 非法时区偏移", map[string]interface{}{
				"start_time": "2026-06-01T00:00:00+25:00",
			}),
			Entry("end_time 早于 start_time", map[string]interface{}{
				"start_time": "2027-01-01 00:00:00",
				"end_time":   "2026-01-01 00:00:00",
			}),
		)
	})

	Describe("POST /api/v1/coupon/grant", func() {
		It("给指定用户发券成功", func() {
			Expect(templateID).NotTo(BeEmpty(), "依赖前面创建模板的用例")
			_, resp := doCoupon(http.MethodPost, "/api/v1/coupon/grant", map[string]interface{}{
				"template_id": templateID,
				"user_id":     couponUserID,
			}, true)
			Expect(resp.Code).To(Equal(errno.OK.FullCode()))
		})

		It("template_id 不是 UUID 应返回 A00001", func() {
			_, resp := doCoupon(http.MethodPost, "/api/v1/coupon/grant", map[string]interface{}{
				"template_id": "not-a-uuid",
				"user_id":     couponUserID,
			}, true)
			Expect(resp.Code).To(Equal(errno.ErrInvalidParam.FullCode()))
		})

		It("缺 user_id 应返回 A00001（这个接口要求运营指定收券人）", func() {
			_, resp := doCoupon(http.MethodPost, "/api/v1/coupon/grant", map[string]interface{}{
				"template_id": templateID,
			}, true)
			Expect(resp.Code).To(Equal(errno.ErrInvalidParam.FullCode()))
		})
	})

	Describe("GET /api/v1/coupon/list", func() {
		It("带分页参数查询成功", func() {
			w, resp := doCoupon(http.MethodGet, "/api/v1/coupon/list?page_num=1&page_size=10", nil, true)
			Expect(w.Code).To(Equal(http.StatusOK))
			Expect(resp.Code).To(Equal(errno.OK.FullCode()))
		})

		It("不带分页参数也能返回", func() {
			// ListUserCouponsParam 的分页 tag 是 omitempty，不传应该也 OK
			_, resp := doCoupon(http.MethodGet, "/api/v1/coupon/list", nil, true)
			Expect(resp.Code).To(Equal(errno.OK.FullCode()))
		})
	})

	Describe("认证", func() {
		It("不带 token 应返回 A02103", func() {
			_, resp := doCoupon(http.MethodGet, "/api/v1/coupon/list", nil, false)
			Expect(resp.Code).To(Equal(errno.ErrAuthInvalidToken.FullCode()))
		})

		It("带伪造 token 应返回 A02103", func() {
			req, _ := http.NewRequest(http.MethodGet, "/api/v1/coupon/list", nil)
			req.Header.Set("Authorization", "FAKE.TOKEN.HERE")
			w := httptest.NewRecorder()
			testRouter.ServeHTTP(w, req)

			var resp Response
			_ = json.Unmarshal(w.Body.Bytes(), &resp)
			Expect(resp.Code).To(Equal(errno.ErrAuthInvalidToken.FullCode()))
		})
	})
})
