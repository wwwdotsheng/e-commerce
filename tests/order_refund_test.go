package tests

import (
	"encoding/json"
	"net/http"

	"e-commerce/internal/model"
	"e-commerce/pkg/errno"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/shopspring/decimal"
	"golang.org/x/crypto/bcrypt"
)

var _ = Describe("订单退款（异步受理）", func() {
	var buyer, seller model.User
	var product model.Product
	var buyerToken, sellerToken string

	newUser := func(kind model.UserType, name string) model.User {
		GinkgoHelper()
		hash, err := bcrypt.GenerateFromPassword([]byte("refund-test-password"), bcrypt.MinCost)
		Expect(err).NotTo(HaveOccurred())
		u := model.User{
			ID: uuid.New(), UserName: name + "-" + uuid.NewString()[:8],
			Email: uuid.NewString() + "@example.test", Password: string(hash),
			UserType: kind, Status: model.UserStatusActive,
		}
		Expect(testDB.Create(&u).Error).To(Succeed())
		return u
	}

	login := func(email string) string {
		GinkgoHelper()
		resp := invariantRequest(http.MethodPost, "/api/v1/auth/login", "", map[string]string{
			"email": email, "password": "refund-test-password",
		})
		Expect(resp.Code).To(Equal(errno.OK.FullCode()))
		var data loginData
		Expect(json.Unmarshal(resp.Data, &data)).To(Succeed())
		return data.AccessToken
	}

	createPaidOrder := func(qty int) model.Order {
		GinkgoHelper()
		key := uuid.NewString()
		Expect(invariantRequest(http.MethodPost, "/api/v1/order/create", buyerToken, map[string]any{
			"product_id": product.ID.String(), "quantity": qty, "idempotency_key": key,
		}).Code).To(Equal(errno.OK.FullCode()))
		var o model.Order
		Expect(testDB.First(&o, "idempotency_key = ?", key).Error).To(Succeed())
		Expect(invariantRequest(http.MethodPost, "/api/v1/order/"+o.ID.String()+"/pay", buyerToken, nil).Code).
			To(Equal(errno.OK.FullCode()))
		Expect(testDB.First(&o, "id = ?", o.ID).Error).To(Succeed())
		return o
	}

	apply := func(orderID uuid.UUID, token string) Response {
		GinkgoHelper()
		return invariantRequest(http.MethodPost, "/api/v1/order/"+orderID.String()+"/refund", token, nil)
	}

	type refundData struct {
		ID     string  `json:"id"`
		Status string  `json:"status"`
		Amount float64 `json:"amount"`
	}
	decode := func(resp Response) refundData {
		GinkgoHelper()
		var d refundData
		Expect(json.Unmarshal(resp.Data, &d)).To(Succeed())
		return d
	}

	BeforeEach(func() {
		buyer = newUser(model.UserTypeBuyer, "refund-buyer")
		seller = newUser(model.UserTypeSeller, "refund-seller")
		Expect(testDB.Create(&model.UserWallet{UserID: buyer.ID, Balance: decimal.NewFromInt(500)}).Error).To(Succeed())
		Expect(testDB.Create(&model.UserWallet{UserID: seller.ID, Balance: decimal.Zero}).Error).To(Succeed())
		product = model.Product{
			ID: uuid.New(), SellerID: seller.ID, Name: "退款测试商品", Description: "fixture",
			Price: decimal.NewFromInt(100), Stock: 10, Status: model.ProductStatusActive, Version: 1,
		}
		Expect(testDB.Create(&product).Error).To(Succeed())
		buyerToken = login(buyer.Email)
		sellerToken = login(seller.Email)
	})

	AfterEach(func() {
		Expect(testDB.Exec("DELETE FROM refund_jobs WHERE refund_id IN (SELECT id FROM refunds WHERE buyer_id = ?)", buyer.ID).Error).To(Succeed())
		Expect(testDB.Exec("DELETE FROM refunds WHERE buyer_id = ?", buyer.ID).Error).To(Succeed())
		Expect(testDB.Where("user_id = ? OR seller_id = ?", buyer.ID, seller.ID).Delete(&model.Order{}).Error).To(Succeed())
		Expect(testDB.Where("product_id = ?", product.ID).Delete(&model.StockChangeLog{}).Error).To(Succeed())
		Expect(testDB.Where("id = ?", product.ID).Delete(&model.Product{}).Error).To(Succeed())
		Expect(testDB.Where("user_id IN ?", []uuid.UUID{buyer.ID, seller.ID}).Delete(&model.WalletLog{}).Error).To(Succeed())
		Expect(testDB.Where("user_id IN ?", []uuid.UUID{buyer.ID, seller.ID}).Delete(&model.UserWallet{}).Error).To(Succeed())
		Expect(testDB.Unscoped().Where("id IN ?", []uuid.UUID{buyer.ID, seller.ID}).Delete(&model.User{}).Error).To(Succeed())
	})

	It("申请只落库：退款 pending、任务 create_refund、订单仍已支付", func() {
		o := createPaidOrder(1)

		resp := apply(o.ID, buyerToken)
		Expect(resp.Code).To(Equal(errno.OK.FullCode()))
		r := decode(resp)
		Expect(r.Status).To(Equal(string(model.RefundStatusPending)))
		Expect(r.Amount).To(Equal(float64(100)))

		var stored model.Refund
		Expect(testDB.First(&stored, "order_id = ?", o.ID).Error).To(Succeed())
		Expect(stored.Status).To(Equal(model.RefundStatusPending))

		var job model.RefundJob
		Expect(testDB.First(&job, "refund_id = ?", stored.ID).Error).To(Succeed())
		Expect(job.JobType).To(Equal(model.RefundJobTypeCreate))
		Expect(job.NextRunAt).NotTo(BeZero())

		var order model.Order
		Expect(testDB.First(&order, "id = ?", o.ID).Error).To(Succeed())
		Expect(order.Status).To(Equal(model.OrderStatusPaid))
	})

	It("重复申请返回同一条退款且只有一条任务", func() {
		o := createPaidOrder(1)
		first := decode(apply(o.ID, buyerToken))
		second := decode(apply(o.ID, buyerToken))
		Expect(second.ID).To(Equal(first.ID))

		var refundCount, jobCount int64
		Expect(testDB.Model(&model.Refund{}).Where("order_id = ?", o.ID).Count(&refundCount).Error).To(Succeed())
		Expect(testDB.Model(&model.RefundJob{}).Where("refund_id = ?", first.ID).Count(&jobCount).Error).To(Succeed())
		Expect(refundCount).To(Equal(int64(1)))
		Expect(jobCount).To(Equal(int64(1)))
	})

	It("查询只读：不推进状态、不改变任务计数", func() {
		o := createPaidOrder(1)
		r := decode(apply(o.ID, buyerToken))

		var before model.RefundJob
		Expect(testDB.First(&before, "refund_id = ?", r.ID).Error).To(Succeed())

		orderDetail := invariantRequest(http.MethodGet, "/api/v1/order/"+o.ID.String(), buyerToken, nil)
		Expect(orderDetail.Code).To(Equal(errno.OK.FullCode()))
		var detail struct {
			Refund *refundData `json:"refund"`
		}
		Expect(json.Unmarshal(orderDetail.Data, &detail)).To(Succeed())
		Expect(detail.Refund).NotTo(BeNil())
		Expect(detail.Refund.Status).To(Equal(string(model.RefundStatusPending)))
		var timeline struct {
			Events []struct {
				Type string `json:"type"`
			} `json:"timeline"`
		}
		Expect(json.Unmarshal(orderDetail.Data, &timeline)).To(Succeed())
		var types []string
		for _, e := range timeline.Events {
			types = append(types, e.Type)
		}
		Expect(types).To(ContainElements("order_created", "payment_succeeded", "refund_requested"))

		var after model.RefundJob
		Expect(testDB.First(&after, "refund_id = ?", r.ID).Error).To(Succeed())
		Expect(after.JobType).To(Equal(before.JobType))
		Expect(after.CreateAttempts).To(Equal(before.CreateAttempts))
		Expect(after.QueryAttempts).To(Equal(before.QueryAttempts))
		Expect(after.NextRunAt.Equal(before.NextRunAt)).To(BeTrue())

		var stored model.Refund
		Expect(testDB.First(&stored, "id = ?", r.ID).Error).To(Succeed())
		Expect(stored.Status).To(Equal(model.RefundStatusPending))
	})

	It("未支付不可退款，非买家被拒绝，订单详情无退款为 null", func() {
		key := uuid.NewString()
		Expect(invariantRequest(http.MethodPost, "/api/v1/order/create", buyerToken, map[string]any{
			"product_id": product.ID.String(), "quantity": 1, "idempotency_key": key,
		}).Code).To(Equal(errno.OK.FullCode()))
		var unpaid model.Order
		Expect(testDB.First(&unpaid, "idempotency_key = ?", key).Error).To(Succeed())
		Expect(apply(unpaid.ID, buyerToken).Code).To(Equal(errno.ErrOrderRefundStatusInvalid.FullCode()))

		paid := createPaidOrder(1)
		var noRefund struct {
			Refund *refundData `json:"refund"`
		}
		Expect(json.Unmarshal(invariantRequest(http.MethodGet, "/api/v1/order/"+paid.ID.String(), buyerToken, nil).Data, &noRefund)).To(Succeed())
		Expect(noRefund.Refund).To(BeNil())
		Expect(apply(paid.ID, sellerToken).Code).To(Equal(errno.ErrAuthNotPermission.FullCode()))
	})

	It("退款金额取订单应付快照，商品改价不影响", func() {
		o := createPaidOrder(2) // 100 * 2 = 200
		Expect(testDB.Model(&model.Product{}).Where("id = ?", product.ID).
			Update("price", decimal.NewFromInt(999)).Error).To(Succeed())

		r := decode(apply(o.ID, buyerToken))
		Expect(r.Amount).To(Equal(float64(200)))
	})
})
