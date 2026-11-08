package tests

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

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

	It("并发申请只产生一条退款和一条任务", func() {
		o := createPaidOrder(1)
		const n = 5
		results := make(chan error, n)
		var wg sync.WaitGroup
		for i := 0; i < n; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := testOrderService.ApplyRefund(context.Background(), buyer.ID, o.ID)
				results <- err
			}()
		}
		wg.Wait()
		close(results)
		for err := range results {
			Expect(err).NotTo(HaveOccurred())
		}
		var refundCount, jobCount int64
		Expect(testDB.Model(&model.Refund{}).Where("order_id = ?", o.ID).Count(&refundCount).Error).To(Succeed())
		Expect(testDB.Model(&model.RefundJob{}).
			Where("refund_id IN (SELECT id FROM refunds WHERE order_id = ?)", o.ID).Count(&jobCount).Error).To(Succeed())
		Expect(refundCount).To(Equal(int64(1)))
		Expect(jobCount).To(Equal(int64(1)))
	})
	It("worker 处理成功：退款成功、订单已退款、任务删除", func() {
		Expect(testDB.Model(&model.Product{}).Where("id = ?", product.ID).
			Update("price", decimal.NewFromInt(99)).Error).To(Succeed())
		o := createPaidOrder(1)
		r := decode(apply(o.ID, buyerToken))
		Expect(r.Status).To(Equal(string(model.RefundStatusPending)))

		_, err := testRefundWorker.ProcessRefundJobs(context.Background())
		Expect(err).NotTo(HaveOccurred())

		var stored model.Refund
		Expect(testDB.First(&stored, "id = ?", r.ID).Error).To(Succeed())
		Expect(stored.Status).To(Equal(model.RefundStatusSucceeded))
		Expect(stored.RefundedAt).NotTo(BeNil())

		var order model.Order
		Expect(testDB.First(&order, "id = ?", o.ID).Error).To(Succeed())
		Expect(order.Status).To(Equal(model.OrderStatusRefunded))

		var jobs int64
		Expect(testDB.Model(&model.RefundJob{}).Where("refund_id = ?", r.ID).Count(&jobs).Error).To(Succeed())
		Expect(jobs).To(Equal(int64(0)))
	})

	It("worker 遇到明确失败：进入等待重试并保留任务", func() {
		o := createPaidOrder(1)
		r := decode(apply(o.ID, buyerToken))

		_, err := testRefundWorker.ProcessRefundJobs(context.Background())
		Expect(err).NotTo(HaveOccurred())

		var stored model.Refund
		Expect(testDB.First(&stored, "id = ?", r.ID).Error).To(Succeed())
		Expect(stored.Status).To(Equal(model.RefundStatusWaitRetry))

		var order model.Order
		Expect(testDB.First(&order, "id = ?", o.ID).Error).To(Succeed())
		Expect(order.Status).To(Equal(model.OrderStatusPaid))

		var job model.RefundJob
		Expect(testDB.First(&job, "refund_id = ?", r.ID).Error).To(Succeed())
		Expect(job.JobType).To(Equal(model.RefundJobTypeCreate))
		Expect(job.CreateAttempts).To(Equal(1))
	})

	It("worker 重试耗尽后标记失败并删除任务", func() {
		o := createPaidOrder(1)
		r := decode(apply(o.ID, buyerToken))

		_, err := testRefundWorker.ProcessRefundJobs(context.Background())
		Expect(err).NotTo(HaveOccurred())

		Expect(testDB.Model(&model.RefundJob{}).Where("refund_id = ?", r.ID).
			Updates(map[string]any{"create_attempts": 5, "next_run_at": time.Now().Add(-time.Second)}).Error).To(Succeed())
		_, err = testRefundWorker.ProcessRefundJobs(context.Background())
		Expect(err).NotTo(HaveOccurred())

		var stored model.Refund
		Expect(testDB.First(&stored, "id = ?", r.ID).Error).To(Succeed())
		Expect(stored.Status).To(Equal(model.RefundStatusFailed))

		var order model.Order
		Expect(testDB.First(&order, "id = ?", o.ID).Error).To(Succeed())
		Expect(order.Status).To(Equal(model.OrderStatusPaid))

		var jobs int64
		Expect(testDB.Model(&model.RefundJob{}).Where("refund_id = ?", r.ID).Count(&jobs).Error).To(Succeed())
		Expect(jobs).To(Equal(int64(0)))
	})

	It("worker 结果不明后查询确认成功", func() {
		Expect(testDB.Model(&model.Product{}).Where("id = ?", product.ID).
			Update("price", decimal.NewFromInt(98)).Error).To(Succeed())
		o := createPaidOrder(1)
		r := decode(apply(o.ID, buyerToken))

		_, err := testRefundWorker.ProcessRefundJobs(context.Background())
		Expect(err).NotTo(HaveOccurred())

		var afterCreate model.Refund
		Expect(testDB.First(&afterCreate, "id = ?", r.ID).Error).To(Succeed())
		Expect(afterCreate.Status).To(Equal(model.RefundStatusWaitRetry))
		var job model.RefundJob
		Expect(testDB.First(&job, "refund_id = ?", r.ID).Error).To(Succeed())
		Expect(job.JobType).To(Equal(model.RefundJobTypeQuery))

		Expect(testDB.Model(&model.RefundJob{}).Where("id = ?", job.ID).
			Update("next_run_at", time.Now().Add(-time.Second)).Error).To(Succeed())
		_, err = testRefundWorker.ProcessRefundJobs(context.Background())
		Expect(err).NotTo(HaveOccurred())

		var stored model.Refund
		Expect(testDB.First(&stored, "id = ?", r.ID).Error).To(Succeed())
		Expect(stored.Status).To(Equal(model.RefundStatusSucceeded))
		var order model.Order
		Expect(testDB.First(&order, "id = ?", o.ID).Error).To(Succeed())
		Expect(order.Status).To(Equal(model.OrderStatusRefunded))
	})
	It("worker 退款成功退回未过期优惠券", func() {
		template := model.CouponTemplate{
			ID: uuid.New(), Name: "退款退券", Type: model.CouponTypeFixed, DiscountValue: decimal.NewFromInt(10),
			TotalQty: 5, RemainingQty: 5, PerUserLimit: 1,
			StartTime: time.Now().Add(-time.Hour), EndTime: time.Now().Add(time.Hour), Status: model.CouponStatusActive,
		}
		coupon := model.UserCoupon{
			ID: uuid.New(), UserID: buyer.ID, TemplateID: template.ID,
			Status: model.UserCouponStatusUnused, ExpireTime: time.Now().Add(time.Hour),
		}
		Expect(testDB.Create(&template).Error).To(Succeed())
		Expect(testDB.Create(&coupon).Error).To(Succeed())
		DeferCleanup(func() {
			Expect(testDB.Delete(&model.UserCoupon{}, "id = ?", coupon.ID).Error).To(Succeed())
			Expect(testDB.Delete(&model.CouponTemplate{}, "id = ?", template.ID).Error).To(Succeed())
		})

		key := uuid.NewString()
		Expect(invariantRequest(http.MethodPost, "/api/v1/order/create", buyerToken, map[string]any{
			"product_id": product.ID.String(), "quantity": 1, "coupon_id": coupon.ID.String(), "idempotency_key": key,
		}).Code).To(Equal(errno.OK.FullCode()))
		var o model.Order
		Expect(testDB.First(&o, "idempotency_key = ?", key).Error).To(Succeed())
		Expect(invariantRequest(http.MethodPost, "/api/v1/order/"+o.ID.String()+"/pay", buyerToken, nil).Code).
			To(Equal(errno.OK.FullCode()))

		Expect(apply(o.ID, buyerToken).Code).To(Equal(errno.OK.FullCode()))
		_, err := testRefundWorker.ProcessRefundJobs(context.Background())
		Expect(err).NotTo(HaveOccurred())

		var stored model.UserCoupon
		Expect(testDB.First(&stored, "id = ?", coupon.ID).Error).To(Succeed())
		Expect(stored.Status).To(Equal(model.UserCouponStatusUnused))
		Expect(stored.UsedOrderID).To(BeNil())
	})

	It("worker 退款成功时已过期优惠券标记 expired", func() {
		template := model.CouponTemplate{
			ID: uuid.New(), Name: "退款退过期券", Type: model.CouponTypeFixed, DiscountValue: decimal.NewFromInt(10),
			TotalQty: 5, RemainingQty: 5, PerUserLimit: 1,
			StartTime: time.Now().Add(-time.Hour), EndTime: time.Now().Add(time.Hour), Status: model.CouponStatusActive,
		}
		coupon := model.UserCoupon{
			ID: uuid.New(), UserID: buyer.ID, TemplateID: template.ID,
			Status: model.UserCouponStatusUnused, ExpireTime: time.Now().Add(time.Hour),
		}
		Expect(testDB.Create(&template).Error).To(Succeed())
		Expect(testDB.Create(&coupon).Error).To(Succeed())
		DeferCleanup(func() {
			Expect(testDB.Delete(&model.UserCoupon{}, "id = ?", coupon.ID).Error).To(Succeed())
			Expect(testDB.Delete(&model.CouponTemplate{}, "id = ?", template.ID).Error).To(Succeed())
		})

		key := uuid.NewString()
		Expect(invariantRequest(http.MethodPost, "/api/v1/order/create", buyerToken, map[string]any{
			"product_id": product.ID.String(), "quantity": 1, "coupon_id": coupon.ID.String(), "idempotency_key": key,
		}).Code).To(Equal(errno.OK.FullCode()))
		var o model.Order
		Expect(testDB.First(&o, "idempotency_key = ?", key).Error).To(Succeed())
		Expect(invariantRequest(http.MethodPost, "/api/v1/order/"+o.ID.String()+"/pay", buyerToken, nil).Code).
			To(Equal(errno.OK.FullCode()))
		Expect(testDB.Model(&model.UserCoupon{}).Where("id = ?", coupon.ID).
			Update("expire_time", time.Now().Add(-time.Minute)).Error).To(Succeed())

		Expect(apply(o.ID, buyerToken).Code).To(Equal(errno.OK.FullCode()))
		_, err := testRefundWorker.ProcessRefundJobs(context.Background())
		Expect(err).NotTo(HaveOccurred())

		var stored model.UserCoupon
		Expect(testDB.First(&stored, "id = ?", coupon.ID).Error).To(Succeed())
		Expect(stored.Status).To(Equal(model.UserCouponStatusExpired))
		Expect(stored.UsedOrderID).To(BeNil())
	})
	It("worker 查询次数耗尽后进入 manual", func() {
		Expect(testDB.Model(&model.Product{}).Where("id = ?", product.ID).
			Update("price", decimal.NewFromInt(98)).Error).To(Succeed())
		o := createPaidOrder(1)
		r := decode(apply(o.ID, buyerToken))

		_, err := testRefundWorker.ProcessRefundJobs(context.Background())
		Expect(err).NotTo(HaveOccurred())
		var job model.RefundJob
		Expect(testDB.First(&job, "refund_id = ?", r.ID).Error).To(Succeed())
		Expect(testDB.Model(&model.RefundJob{}).Where("id = ?", job.ID).
			Updates(map[string]any{"query_attempts": 10, "next_run_at": time.Now().Add(-time.Second)}).Error).To(Succeed())

		_, err = testRefundWorker.ProcessRefundJobs(context.Background())
		Expect(err).NotTo(HaveOccurred())

		var stored model.Refund
		Expect(testDB.First(&stored, "id = ?", r.ID).Error).To(Succeed())
		Expect(stored.Status).To(Equal(model.RefundStatusManual))
		Expect(stored.FailureReason).To(Equal("query attempts exhausted"))
		var jobs int64
		Expect(testDB.Model(&model.RefundJob{}).Where("refund_id = ?", r.ID).Count(&jobs).Error).To(Succeed())
		Expect(jobs).To(Equal(int64(0)))
		var order model.Order
		Expect(testDB.First(&order, "id = ?", o.ID).Error).To(Succeed())
		Expect(order.Status).To(Equal(model.OrderStatusPaid))
	})
	It("退款金额取订单应付快照，商品改价不影响", func() {
		o := createPaidOrder(2) // 100 * 2 = 200
		Expect(testDB.Model(&model.Product{}).Where("id = ?", product.ID).
			Update("price", decimal.NewFromInt(999)).Error).To(Succeed())

		r := decode(apply(o.ID, buyerToken))
		Expect(r.Amount).To(Equal(float64(200)))
	})
})
