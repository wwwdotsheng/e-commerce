package tests

import (
	"context"
	"encoding/json"
	"fmt"
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

	createPaidOrderWithCoupon := func(qty int, couponID uuid.UUID) model.Order {
		GinkgoHelper()
		key := uuid.NewString()
		body := map[string]any{
			"product_id": product.ID.String(), "quantity": qty, "idempotency_key": key,
		}
		if couponID != uuid.Nil {
			body["coupon_id"] = couponID.String()
		}
		Expect(invariantRequest(http.MethodPost, "/api/v1/order/create", buyerToken, body).Code).
			To(Equal(errno.OK.FullCode()))
		var o model.Order
		Expect(testDB.First(&o, "idempotency_key = ?", key).Error).To(Succeed())
		Expect(invariantRequest(http.MethodPost, "/api/v1/order/"+o.ID.String()+"/pay", buyerToken, nil).Code).
			To(Equal(errno.OK.FullCode()))
		Expect(testDB.First(&o, "id = ?", o.ID).Error).To(Succeed())
		return o
	}

	createPaidOrder := func(qty int) model.Order {
		GinkgoHelper()
		return createPaidOrderWithCoupon(qty, uuid.Nil)
	}

	apply := func(orderID uuid.UUID, token string) Response {
		GinkgoHelper()
		return invariantRequest(http.MethodPost, "/api/v1/order/"+orderID.String()+"/refund", token, nil)
	}

	type refundData struct {
		ID            string     `json:"id"`
		Status        string     `json:"status"`
		Amount        float64    `json:"amount"`
		FailureReason string     `json:"failure_reason"`
		RefundedAt    *time.Time `json:"refunded_at"`
	}
	decode := func(resp Response) refundData {
		GinkgoHelper()
		var d refundData
		Expect(json.Unmarshal(resp.Data, &d)).To(Succeed())
		return d
	}

	requestRefund := func(orderID uuid.UUID) refundData {
		GinkgoHelper()
		resp := apply(orderID, buyerToken)
		Expect(resp.Code).To(Equal(errno.OK.FullCode()))
		return decode(resp)
	}

	setBalance := func(userID uuid.UUID, amount int64) {
		GinkgoHelper()
		result := testDB.Model(&model.UserWallet{}).Where("user_id = ?", userID).
			Update("balance", decimal.NewFromInt(amount))
		Expect(result.Error).NotTo(HaveOccurred())
		Expect(result.RowsAffected).To(Equal(int64(1)))
	}

	expectBalances := func(buyerBalance, sellerBalance int64) {
		GinkgoHelper()
		var buyerWallet, sellerWallet model.UserWallet
		Expect(testDB.Take(&buyerWallet, "user_id = ?", buyer.ID).Error).To(Succeed())
		Expect(testDB.Take(&sellerWallet, "user_id = ?", seller.ID).Error).To(Succeed())
		Expect(buyerWallet.Balance.String()).To(Equal(decimal.NewFromInt(buyerBalance).String()), "买家钱包余额")
		Expect(sellerWallet.Balance.String()).To(Equal(decimal.NewFromInt(sellerBalance).String()), "商家钱包余额")
	}

	walletLogs := func() []model.WalletLog {
		GinkgoHelper()
		var logs []model.WalletLog
		Expect(testDB.Where("user_id IN ?", []uuid.UUID{buyer.ID, seller.ID}).Order("id").Find(&logs).Error).To(Succeed())
		return logs
	}

	expectTransfer := func(refundID string, amount int64) {
		GinkgoHelper()
		keyPrefix := "order:refund:" + refundID + ":"
		var logs []model.WalletLog
		Expect(testDB.Where("idempotency_key LIKE ?", keyPrefix+"%").Order("idempotency_key").Find(&logs).Error).To(Succeed())
		Expect(logs).To(HaveLen(2))
		credit, debit := logs[0], logs[1]
		Expect(credit.IdempotencyKey).To(Equal(keyPrefix + "credit"))
		Expect(credit.UserID).To(Equal(buyer.ID))
		Expect(credit.Type).To(Equal(model.WalletLogTypeRefundCredit))
		Expect(credit.Amount.String()).To(Equal(decimal.NewFromInt(amount).String()))
		Expect(debit.IdempotencyKey).To(Equal(keyPrefix + "debit"))
		Expect(debit.UserID).To(Equal(seller.ID))
		Expect(debit.Type).To(Equal(model.WalletLogTypeRefundDebit))
		Expect(debit.Amount.String()).To(Equal(decimal.NewFromInt(-amount).String()))
	}

	expectRefundState := func(orderID uuid.UUID, refundID string, refundStatus model.RefundStatus, orderStatus model.OrderStatus) model.Refund {
		GinkgoHelper()
		var stored model.Refund
		Expect(testDB.Take(&stored, "order_id = ?", orderID).Error).To(Succeed())
		Expect(stored.ID.String()).To(Equal(refundID))
		Expect(stored.BuyerID).To(Equal(buyer.ID))
		Expect(stored.Status).To(Equal(refundStatus))
		var order model.Order
		Expect(testDB.Take(&order, "id = ?", orderID).Error).To(Succeed())
		Expect(order.Status).To(Equal(orderStatus))
		return stored
	}

	expectJobCount := func(refundID string, expected int64) {
		GinkgoHelper()
		var count int64
		Expect(testDB.Model(&model.RefundJob{}).Where("refund_id = ?", refundID).Count(&count).Error).To(Succeed())
		Expect(count).To(Equal(expected))
	}

	type timelineEvent struct {
		Type     string `json:"type"`
		RefundID string `json:"refund_id"`
	}
	expectRefundDetail := func(orderID uuid.UUID, refundID string, refundStatus model.RefundStatus, orderStatus model.OrderStatus, eventType string) (refundData, Response) {
		GinkgoHelper()
		resp := invariantRequest(http.MethodGet, "/api/v1/order/"+orderID.String(), buyerToken, nil)
		Expect(resp.Code).To(Equal(errno.OK.FullCode()))
		var detail struct {
			Status   model.OrderStatus `json:"status"`
			Refund   *refundData       `json:"refund"`
			Timeline []timelineEvent   `json:"timeline"`
		}
		Expect(json.Unmarshal(resp.Data, &detail)).To(Succeed())
		Expect(detail.Status).To(Equal(orderStatus))
		Expect(detail.Refund).NotTo(BeNil())
		Expect(detail.Refund.ID).To(Equal(refundID))
		Expect(detail.Refund.Status).To(Equal(string(refundStatus)))
		Expect(detail.Timeline).To(ContainElement(timelineEvent{Type: eventType, RefundID: refundID}))
		return *detail.Refund, resp
	}

	newRefundCoupon := func(discount int64) model.UserCoupon {
		GinkgoHelper()
		now := time.Now()
		template := model.CouponTemplate{
			ID: uuid.New(), Name: "退款退券", Type: model.CouponTypeFixed, DiscountValue: decimal.NewFromInt(discount),
			TotalQty: 5, RemainingQty: 5, PerUserLimit: 1,
			StartTime: now.Add(-time.Hour), EndTime: now.Add(time.Hour), Status: model.CouponStatusActive,
		}
		Expect(testDB.Create(&template).Error).To(Succeed())
		DeferCleanup(func() {
			Expect(testDB.Delete(&model.CouponTemplate{}, "id = ?", template.ID).Error).To(Succeed())
		})
		coupon := model.UserCoupon{
			ID: uuid.New(), UserID: buyer.ID, TemplateID: template.ID,
			Status: model.UserCouponStatusUnused, ExpireTime: now.Add(time.Hour),
		}
		Expect(testDB.Create(&coupon).Error).To(Succeed())
		DeferCleanup(func() {
			Expect(testDB.Delete(&model.UserCoupon{}, "id = ?", coupon.ID).Error).To(Succeed())
		})
		return coupon
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

	It("申请只落库：退款 pending、任务 transfer_refund、订单仍已支付", func() {
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
		Expect(job.JobType).To(Equal(model.RefundJobTypeTransfer))
		Expect(job.NextRunAt).NotTo(BeZero())

		var order model.Order
		Expect(testDB.First(&order, "id = ?", o.ID).Error).To(Succeed())
		Expect(order.Status).To(Equal(model.OrderStatusPaid))
	})

	It("重复申请返回同一条退款且只有一条任务", func() {
		o := createPaidOrder(1)
		first := requestRefund(o.ID)
		second := requestRefund(o.ID)
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

	It("并发申请返回同一退款，重复执行只转账一次", func() {
		o := createPaidOrder(1)
		const n = 5
		start := make(chan struct{})
		results := make(chan refundData, n)
		var wg sync.WaitGroup
		for range n {
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer GinkgoRecover()
				<-start
				results <- requestRefund(o.ID)
			}()
		}
		close(start)
		wg.Wait()
		Expect(results).To(HaveLen(n))
		close(results)
		var refunds []model.Refund
		Expect(testDB.Where("order_id = ?", o.ID).Find(&refunds).Error).To(Succeed())
		Expect(refunds).To(HaveLen(1))
		refundID := refunds[0].ID.String()
		for r := range results {
			Expect(r.ID).To(Equal(refundID))
			Expect(r.Status).To(Equal(string(model.RefundStatusPending)))
		}
		expectJobCount(refundID, 1)
		var job model.RefundJob
		Expect(testDB.Take(&job, "refund_id = ?", refundID).Error).To(Succeed())

		beforeLogs := walletLogs()
		startWorkers := make(chan struct{})
		workerErrors := make(chan error, 2)
		for range cap(workerErrors) {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-startWorkers
				_, err := testRefundWorker.ProcessRefundJobs(context.Background())
				workerErrors <- err
			}()
		}
		close(startWorkers)
		wg.Wait()
		close(workerErrors)
		for err := range workerErrors {
			Expect(err).NotTo(HaveOccurred())
		}
		expectRefundState(o.ID, refundID, model.RefundStatusSucceeded, model.OrderStatusRefunded)
		expectJobCount(refundID, 0)
		expectBalances(500, 0)
		expectTransfer(refundID, 100)
		afterLogs := walletLogs()
		Expect(afterLogs).To(HaveLen(len(beforeLogs) + 2))

		// Replay the original job even if the second scanner did not see it before deletion.
		Expect(testDB.Create(&job).Error).To(Succeed())
		_, err := testRefundWorker.ProcessRefundJobs(context.Background())
		Expect(err).NotTo(HaveOccurred())
		replayed := requestRefund(o.ID)
		Expect(replayed.ID).To(Equal(refundID))
		Expect(replayed.Status).To(Equal(string(model.RefundStatusSucceeded)))
		expectRefundState(o.ID, refundID, model.RefundStatusSucceeded, model.OrderStatusRefunded)
		expectJobCount(refundID, 0)
		expectBalances(500, 0)
		Expect(walletLogs()).To(Equal(afterLogs))
	})

	It("实付 100 元异步原路退回，双方余额和流水正确，库存保持不变", func() {
		setBalance(buyer.ID, 120)
		setBalance(seller.ID, 50)
		o := createPaidOrder(1)
		expectBalances(20, 150)
		beforeLogs := walletLogs()
		var beforeRefund model.Product
		Expect(testDB.Take(&beforeRefund, "id = ?", product.ID).Error).To(Succeed())
		Expect(beforeRefund.Stock).To(Equal(product.Stock - 1))

		r := requestRefund(o.ID)
		Expect(r.Status).To(Equal(string(model.RefundStatusPending)))
		Expect(r.Amount).To(Equal(float64(100)))
		expectRefundState(o.ID, r.ID, model.RefundStatusPending, model.OrderStatusPaid)
		expectBalances(20, 150)
		Expect(walletLogs()).To(Equal(beforeLogs))

		_, err := testRefundWorker.ProcessRefundJobs(context.Background())
		Expect(err).NotTo(HaveOccurred())

		stored := expectRefundState(o.ID, r.ID, model.RefundStatusSucceeded, model.OrderStatusRefunded)
		Expect(stored.Amount.String()).To(Equal("100"))
		Expect(stored.RefundedAt).NotTo(BeNil())

		detail, _ := expectRefundDetail(o.ID, r.ID, model.RefundStatusSucceeded, model.OrderStatusRefunded, "refund_succeeded")
		Expect(detail.Amount).To(Equal(float64(100)))
		Expect(detail.RefundedAt).NotTo(BeNil())
		expectJobCount(r.ID, 0)
		expectBalances(120, 50)
		expectTransfer(r.ID, 100)
		Expect(walletLogs()).To(HaveLen(len(beforeLogs) + 2))
		var afterRefund model.Product
		Expect(testDB.Take(&afterRefund, "id = ?", product.ID).Error).To(Succeed())
		Expect(afterRefund.Stock).To(Equal(beforeRefund.Stock))
	})

	It("零元订单退款成功并退券，钱包余额及流水保持原值", func() {
		coupon := newRefundCoupon(100)
		o := createPaidOrderWithCoupon(1, coupon.ID)
		Expect(o.PayableAmount.IsZero()).To(BeTrue())
		var usedCoupon model.UserCoupon
		Expect(testDB.Take(&usedCoupon, "id = ?", coupon.ID).Error).To(Succeed())
		Expect(usedCoupon.Status).To(Equal(model.UserCouponStatusUsed))
		Expect(usedCoupon.UsedOrderID).NotTo(BeNil())
		Expect(*usedCoupon.UsedOrderID).To(Equal(o.ID))
		expectBalances(500, 0)
		beforeLogs := walletLogs()

		r := requestRefund(o.ID)
		Expect(r.Status).To(Equal(string(model.RefundStatusPending)))
		Expect(r.Amount).To(BeZero())
		_, err := testRefundWorker.ProcessRefundJobs(context.Background())
		Expect(err).NotTo(HaveOccurred())
		stored := expectRefundState(o.ID, r.ID, model.RefundStatusSucceeded, model.OrderStatusRefunded)
		Expect(stored.Amount.IsZero()).To(BeTrue())
		Expect(stored.RefundedAt).NotTo(BeNil())
		expectJobCount(r.ID, 0)
		expectBalances(500, 0)
		Expect(walletLogs()).To(Equal(beforeLogs))
		var returnedCoupon model.UserCoupon
		Expect(testDB.Take(&returnedCoupon, "id = ?", coupon.ID).Error).To(Succeed())
		Expect(returnedCoupon.Status).To(Equal(model.UserCouponStatusUnused))
		Expect(returnedCoupon.UsedOrderID).To(BeNil())
	})

	It("商家余额不足由后台失败，订单详情和时间线只显示公开信息", func() {
		o := createPaidOrder(1)
		setBalance(seller.ID, 30)
		beforeLogs := walletLogs()
		r := requestRefund(o.ID)
		Expect(r.Status).To(Equal(string(model.RefundStatusPending)))

		_, err := testRefundWorker.ProcessRefundJobs(context.Background())
		Expect(err).NotTo(HaveOccurred())
		stored := expectRefundState(o.ID, r.ID, model.RefundStatusFailed, model.OrderStatusPaid)
		Expect(stored.RefundedAt).To(BeNil())
		Expect(stored.FailureReason).NotTo(BeEmpty())
		expectJobCount(r.ID, 0)
		expectBalances(400, 30)
		Expect(walletLogs()).To(Equal(beforeLogs))

		detail, resp := expectRefundDetail(o.ID, r.ID, model.RefundStatusFailed, model.OrderStatusPaid, "refund_failed")
		Expect(detail.FailureReason).To(Equal("本次退款未完成"))
		publicResponse := string(resp.Data) + resp.UserMsg + resp.DevMsg
		Expect(publicResponse).NotTo(ContainSubstring(stored.FailureReason))
		Expect(publicResponse).NotTo(MatchRegexp("(?i)seller[_ ]+(wallet[_ ]+)?balance|商家余额|余额不足"))
		replayed := requestRefund(o.ID)
		Expect(replayed.ID).To(Equal(r.ID))
		Expect(replayed.Status).To(Equal(string(model.RefundStatusFailed)))
		Expect(replayed.FailureReason).To(Equal("本次退款未完成"))
		expectJobCount(r.ID, 0)
	})

	It("商家钱包并发扣款后，退款按获取行锁时的余额决定是否执行", func() {
		o := createPaidOrder(1)
		r := requestRefund(o.ID)
		beforeLogs := walletLogs()

		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		sellerTx := testDB.WithContext(ctx).Begin()
		Expect(sellerTx.Error).NotTo(HaveOccurred())
		defer sellerTx.Rollback()

		var blockerPID int
		Expect(sellerTx.Raw("SELECT pg_backend_pid()").Scan(&blockerPID).Error).To(Succeed())
		Expect(sellerTx.Model(&model.UserWallet{}).Where("user_id = ?", seller.ID).
			Update("balance", decimal.NewFromInt(30)).Error).To(Succeed())

		done := make(chan struct{})
		var workerErr error
		go func() {
			defer close(done)
			_, workerErr = testRefundWorker.ProcessRefundJobs(ctx)
		}()
		defer func() {
			cancel()
			<-done
		}()

		// Release the competing update only after PostgreSQL confirms the worker is blocked.
		Eventually(func() (bool, error) {
			var blocked bool
			err := testDB.WithContext(ctx).Raw(
				"SELECT EXISTS (SELECT 1 FROM pg_stat_activity WHERE ? = ANY(pg_blocking_pids(pid)))",
				blockerPID,
			).Scan(&blocked).Error
			return blocked, err
		}, 5*time.Second, 25*time.Millisecond).Should(BeTrue())
		Expect(sellerTx.Commit().Error).To(Succeed())
		Eventually(done, 5*time.Second).Should(BeClosed())
		Expect(workerErr).NotTo(HaveOccurred())

		expectRefundState(o.ID, r.ID, model.RefundStatusFailed, model.OrderStatusPaid)
		expectJobCount(r.ID, 0)
		expectBalances(400, 30)
		Expect(walletLogs()).To(Equal(beforeLogs))
	})

	It("买家退款流水写入失败时整体回滚，恢复后同一任务可重试", func() {
		o := createPaidOrder(1)
		beforeLogs := walletLogs()
		r := requestRefund(o.ID)
		var job model.RefundJob
		Expect(testDB.Take(&job, "refund_id = ?", r.ID).Error).To(Succeed())

		constraint := "reject_refund_credit_" + uuid.NewString()[:8]
		// Reject this refund's buyer credit so any preceding seller write must roll back.
		creditKey := "order:refund:" + job.RefundID.String() + ":credit"
		Expect(testDB.Exec(fmt.Sprintf(
			"ALTER TABLE wallet_logs ADD CONSTRAINT %s CHECK (idempotency_key <> '%s')", constraint, creditKey,
		)).Error).To(Succeed())
		removeFault := func() {
			Expect(testDB.Exec("ALTER TABLE wallet_logs DROP CONSTRAINT IF EXISTS " + constraint).Error).To(Succeed())
		}
		DeferCleanup(removeFault)

		processed, err := testRefundWorker.ProcessRefundJobs(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(processed).To(BeZero())
		stored := expectRefundState(o.ID, r.ID, model.RefundStatusPending, model.OrderStatusPaid)
		Expect(stored.RefundedAt).To(BeNil())
		var retained model.RefundJob
		Expect(testDB.Take(&retained, "refund_id = ?", r.ID).Error).To(Succeed())
		Expect(retained.ID).To(Equal(job.ID))
		expectBalances(400, 100)
		Expect(walletLogs()).To(Equal(beforeLogs))

		removeFault()
		_, err = testRefundWorker.ProcessRefundJobs(context.Background())
		Expect(err).NotTo(HaveOccurred())
		expectRefundState(o.ID, r.ID, model.RefundStatusSucceeded, model.OrderStatusRefunded)
		expectJobCount(r.ID, 0)
		expectBalances(500, 0)
		expectTransfer(r.ID, 100)
		Expect(walletLogs()).To(HaveLen(len(beforeLogs) + 2))
	})

	DescribeTable("未完成的旧退款任务到期后按钱包退款执行",
		func(jobType model.RefundJobType, status model.RefundStatus) {
			o := createPaidOrder(1)
			refund := model.Refund{OrderID: o.ID, BuyerID: o.UserID, Amount: o.PayableAmount, Status: status}
			Expect(testDB.Create(&refund).Error).To(Succeed())
			job := model.RefundJob{RefundID: refund.ID, JobType: jobType, NextRunAt: time.Now().Add(time.Hour)}
			Expect(testDB.Create(&job).Error).To(Succeed())
			refundID := refund.ID.String()
			beforeLogs := walletLogs()

			_, err := testRefundWorker.ProcessRefundJobs(context.Background())
			Expect(err).NotTo(HaveOccurred())
			expectRefundState(o.ID, refundID, status, model.OrderStatusPaid)
			expectJobCount(refundID, 1)
			expectBalances(400, 100)
			Expect(walletLogs()).To(Equal(beforeLogs))

			Expect(testDB.Model(&job).Update("next_run_at", time.Now().Add(-time.Minute)).Error).To(Succeed())
			_, err = testRefundWorker.ProcessRefundJobs(context.Background())
			Expect(err).NotTo(HaveOccurred())
			stored := expectRefundState(o.ID, refundID, model.RefundStatusSucceeded, model.OrderStatusRefunded)
			Expect(stored.RefundedAt).NotTo(BeNil())
			expectJobCount(refundID, 0)
			expectBalances(500, 0)
			expectTransfer(refundID, 100)
			Expect(walletLogs()).To(HaveLen(len(beforeLogs) + 2))
		},
		Entry("create_refund（processing）", model.RefundJobTypeCreate, model.RefundStatusProcessing),
		Entry("query_refund（wait_retry）", model.RefundJobTypeQuery, model.RefundStatusWaitRetry),
	)

	DescribeTable("旧退款已结束时不重新动账",
		func(jobType model.RefundJobType, status model.RefundStatus, orderStatus model.OrderStatus) {
			o := createPaidOrder(1)
			refund := model.Refund{OrderID: o.ID, BuyerID: o.UserID, Amount: o.PayableAmount, Status: status}
			if status == model.RefundStatusSucceeded {
				refundedAt := time.Now().Add(-time.Hour).Truncate(time.Second)
				refund.RefundedAt = &refundedAt
				Expect(testDB.Model(&o).Update("status", orderStatus).Error).To(Succeed())
			}
			Expect(testDB.Create(&refund).Error).To(Succeed())
			Expect(testDB.Create(&model.RefundJob{
				RefundID: refund.ID, JobType: jobType, NextRunAt: time.Now().Add(-time.Minute),
			}).Error).To(Succeed())
			refundID := refund.ID.String()
			before := expectRefundState(o.ID, refundID, status, orderStatus)
			beforeLogs := walletLogs()

			_, err := testRefundWorker.ProcessRefundJobs(context.Background())
			Expect(err).NotTo(HaveOccurred())

			Expect(expectRefundState(o.ID, refundID, status, orderStatus)).To(Equal(before))
			expectJobCount(refundID, 0)
			expectBalances(400, 100)
			Expect(walletLogs()).To(Equal(beforeLogs))
		},
		Entry("succeeded", model.RefundJobTypeCreate, model.RefundStatusSucceeded, model.OrderStatusRefunded),
		Entry("failed", model.RefundJobTypeCreate, model.RefundStatusFailed, model.OrderStatusPaid),
		Entry("manual", model.RefundJobTypeQuery, model.RefundStatusManual, model.OrderStatusPaid),
	)

	It("worker 退款成功退回未过期优惠券", func() {
		coupon := newRefundCoupon(10)
		o := createPaidOrderWithCoupon(1, coupon.ID)
		Expect(apply(o.ID, buyerToken).Code).To(Equal(errno.OK.FullCode()))
		_, err := testRefundWorker.ProcessRefundJobs(context.Background())
		Expect(err).NotTo(HaveOccurred())

		var stored model.UserCoupon
		Expect(testDB.First(&stored, "id = ?", coupon.ID).Error).To(Succeed())
		Expect(stored.Status).To(Equal(model.UserCouponStatusUnused))
		Expect(stored.UsedOrderID).To(BeNil())
	})

	It("worker 退款成功时已过期优惠券标记 expired", func() {
		coupon := newRefundCoupon(10)
		o := createPaidOrderWithCoupon(1, coupon.ID)
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
	It("退款金额取订单应付快照，商品改价不影响", func() {
		o := createPaidOrder(2) // 100 * 2 = 200
		Expect(testDB.Model(&model.Product{}).Where("id = ?", product.ID).
			Update("price", decimal.NewFromInt(999)).Error).To(Succeed())

		r := requestRefund(o.ID)
		Expect(r.Amount).To(Equal(float64(200)))
		_, err := testRefundWorker.ProcessRefundJobs(context.Background())
		Expect(err).NotTo(HaveOccurred())
		expectRefundState(o.ID, r.ID, model.RefundStatusSucceeded, model.OrderStatusRefunded)
		expectBalances(500, 0)
		expectTransfer(r.ID, 200)
	})
})
