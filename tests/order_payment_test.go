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

var _ = Describe("订单余额支付", func() {
	var buyer, seller model.User
	var product model.Product
	var buyerToken string

	newUser := func(kind model.UserType, name string) model.User {
		GinkgoHelper()
		hash, err := bcrypt.GenerateFromPassword([]byte("pay-test-password"), bcrypt.MinCost)
		Expect(err).NotTo(HaveOccurred())
		u := model.User{
			ID:       uuid.New(),
			UserName: name + "-" + uuid.NewString()[:8],
			Email:    uuid.NewString() + "@example.test",
			Password: string(hash),
			UserType: kind,
			Status:   model.UserStatusActive,
		}
		Expect(testDB.Create(&u).Error).To(Succeed())
		return u
	}

	createWallet := func(userID uuid.UUID, balance float64) {
		GinkgoHelper()
		Expect(testDB.Create(&model.UserWallet{UserID: userID, Balance: decimal.NewFromFloat(balance)}).Error).To(Succeed())
	}

	balanceOf := func(userID uuid.UUID) float64 {
		GinkgoHelper()
		var wallet model.UserWallet
		Expect(testDB.First(&wallet, "user_id = ?", userID).Error).To(Succeed())
		return wallet.Balance.InexactFloat64()
	}

	createOrder := func(qty int) model.Order {
		GinkgoHelper()
		key := uuid.NewString()
		Expect(invariantRequest(http.MethodPost, "/api/v1/order/create", buyerToken, map[string]any{
			"product_id": product.ID.String(), "quantity": qty, "idempotency_key": key,
		}).Code).To(Equal(errno.OK.FullCode()))
		var o model.Order
		Expect(testDB.First(&o, "idempotency_key = ?", key).Error).To(Succeed())
		return o
	}

	pay := func(token string, orderID uuid.UUID) Response {
		GinkgoHelper()
		return invariantRequest(http.MethodPost, "/api/v1/order/"+orderID.String()+"/pay", token, nil)
	}

	paymentLogs := func() []model.WalletLog {
		GinkgoHelper()
		var logs []model.WalletLog
		Expect(testDB.Where("user_id IN ?", []uuid.UUID{buyer.ID, seller.ID}).Order("created_at").Find(&logs).Error).To(Succeed())
		return logs
	}

	BeforeEach(func() {
		buyer = newUser(model.UserTypeBuyer, "pay-buyer")
		seller = newUser(model.UserTypeSeller, "pay-seller")
		createWallet(buyer.ID, 500)
		createWallet(seller.ID, 0)

		product = model.Product{
			ID: uuid.New(), SellerID: seller.ID, Name: "支付测试商品", Description: "fixture",
			Price: decimal.NewFromInt(100), Stock: 10, Status: model.ProductStatusActive, Version: 1,
		}
		Expect(testDB.Create(&product).Error).To(Succeed())

		login := invariantRequest(http.MethodPost, "/api/v1/auth/login", "", map[string]string{
			"email": buyer.Email, "password": "pay-test-password",
		})
		Expect(login.Code).To(Equal(errno.OK.FullCode()))
		var data loginData
		Expect(json.Unmarshal(login.Data, &data)).To(Succeed())
		Expect(data.AccessToken).NotTo(BeEmpty())
		buyerToken = data.AccessToken
	})

	AfterEach(func() {
		Expect(testDB.Where("user_id IN ? OR seller_id IN ?", []uuid.UUID{buyer.ID}, []uuid.UUID{seller.ID}).Delete(&model.Order{}).Error).To(Succeed())
		Expect(testDB.Where("product_id = ?", product.ID).Delete(&model.StockChangeLog{}).Error).To(Succeed())
		Expect(testDB.Where("id = ?", product.ID).Delete(&model.Product{}).Error).To(Succeed())
		Expect(testDB.Where("user_id IN ?", []uuid.UUID{buyer.ID, seller.ID}).Delete(&model.WalletLog{}).Error).To(Succeed())
		Expect(testDB.Where("user_id IN ?", []uuid.UUID{buyer.ID, seller.ID}).Delete(&model.UserWallet{}).Error).To(Succeed())
		Expect(testDB.Unscoped().Where("id IN ?", []uuid.UUID{buyer.ID, seller.ID}).Delete(&model.User{}).Error).To(Succeed())
	})

	It("支付成功后买家扣款、商家入账并写入支付时间", func() {
		o := createOrder(2)
		Expect(o.PayableAmount.InexactFloat64()).To(Equal(float64(200)))

		resp := pay(buyerToken, o.ID)
		Expect(resp.Code).To(Equal(errno.OK.FullCode()))
		var detail struct {
			Status        int        `json:"status"`
			PayableAmount float64    `json:"payable_amount"`
			PaidAt        *time.Time `json:"paid_at"`
			ClosedAt      *time.Time `json:"closed_at"`
		}
		Expect(json.Unmarshal(resp.Data, &detail)).To(Succeed())
		Expect(detail.Status).To(Equal(int(model.OrderStatusPaid)))
		Expect(detail.PayableAmount).To(Equal(float64(200)))
		Expect(detail.PaidAt).NotTo(BeNil())
		Expect(detail.ClosedAt).To(BeNil())

		Expect(balanceOf(buyer.ID)).To(Equal(float64(300)))
		Expect(balanceOf(seller.ID)).To(Equal(float64(200)))
		var paid model.Order
		Expect(testDB.First(&paid, "id = ?", o.ID).Error).To(Succeed())
		Expect(paid.Status).To(Equal(model.OrderStatusPaid))
		Expect(paid.PaidAt).NotTo(BeNil())

		logs := paymentLogs()
		Expect(logs).To(HaveLen(2))
		var buyerLog, sellerLog model.WalletLog
		Expect(testDB.First(&buyerLog, "user_id = ?", buyer.ID).Error).To(Succeed())
		Expect(buyerLog.Type).To(Equal(model.WalletLogTypeOrderPayment))
		Expect(buyerLog.Amount.InexactFloat64()).To(Equal(float64(-200)))
		Expect(testDB.First(&sellerLog, "user_id = ?", seller.ID).Error).To(Succeed())
		Expect(sellerLog.Type).To(Equal(model.WalletLogTypeOrderIncome))
		Expect(sellerLog.Amount.InexactFloat64()).To(Equal(float64(200)))
	})

	It("重复支付返回已有结果且不重复扣款", func() {
		o := createOrder(1)
		Expect(pay(buyerToken, o.ID).Code).To(Equal(errno.OK.FullCode()))

		first := paymentLogs()
		second := pay(buyerToken, o.ID)
		Expect(second.Code).To(Equal(errno.OK.FullCode()))
		var detail struct {
			Status int        `json:"status"`
			PaidAt *time.Time `json:"paid_at"`
		}
		Expect(json.Unmarshal(second.Data, &detail)).To(Succeed())
		Expect(detail.Status).To(Equal(int(model.OrderStatusPaid)))

		Expect(balanceOf(buyer.ID)).To(Equal(float64(400)))
		Expect(balanceOf(seller.ID)).To(Equal(float64(100)))
		Expect(paymentLogs()).To(HaveLen(len(first)))
	})

	It("余额不足时拒绝支付并保持待支付", func() {
		o := createOrder(1)
		Expect(testDB.Model(&model.UserWallet{}).Where("user_id = ?", buyer.ID).Update("balance", 50).Error).To(Succeed())

		Expect(pay(buyerToken, o.ID).Code).To(Equal(errno.ErrWalletBalanceInsufficient.FullCode()))
		Expect(balanceOf(buyer.ID)).To(Equal(float64(50)))
		Expect(balanceOf(seller.ID)).To(BeZero())
		Expect(paymentLogs()).To(BeEmpty())

		var current model.Order
		Expect(testDB.First(&current, "id = ?", o.ID).Error).To(Succeed())
		Expect(current.Status).To(Equal(model.OrderStatusProcessing))
		Expect(current.PaidAt).To(BeNil())
	})

	It("超过支付截止时间后拒绝支付", func() {
		o := createOrder(1)
		Expect(testDB.Model(&model.Order{}).Where("id = ?", o.ID).
			Update("payment_deadline", time.Now().Add(-time.Minute)).Error).To(Succeed())

		Expect(pay(buyerToken, o.ID).Code).To(Equal(errno.ErrOrderPaymentExpired.FullCode()))
		Expect(balanceOf(buyer.ID)).To(Equal(float64(500)))
		Expect(paymentLogs()).To(BeEmpty())

		var current model.Order
		Expect(testDB.First(&current, "id = ?", o.ID).Error).To(Succeed())
		Expect(current.Status).To(Equal(model.OrderStatusProcessing))
	})

	It("已超时关闭的订单不能再支付", func() {
		o := createOrder(1)
		Expect(testDB.Model(&model.Order{}).Where("id = ?", o.ID).
			Update("payment_deadline", time.Now().Add(-time.Second)).Error).To(Succeed())
		Expect(testOrderService.HandleOrderTimeout(context.Background(), o.ID)).To(Succeed())

		Expect(pay(buyerToken, o.ID).Code).To(Equal(errno.ErrOrderPaymentStatusInvalid.FullCode()))
		Expect(balanceOf(buyer.ID)).To(Equal(float64(500)))
		Expect(paymentLogs()).To(BeEmpty())
	})

	It("不能支付其他买家的订单", func() {
		o := createOrder(1)
		other := newUser(model.UserTypeBuyer, "pay-other")
		createWallet(other.ID, 500)
		login := invariantRequest(http.MethodPost, "/api/v1/auth/login", "", map[string]string{
			"email": other.Email, "password": "pay-test-password",
		})
		Expect(login.Code).To(Equal(errno.OK.FullCode()))
		var data loginData
		Expect(json.Unmarshal(login.Data, &data)).To(Succeed())
		defer func() {
			Expect(testDB.Where("user_id = ?", other.ID).Delete(&model.WalletLog{}).Error).To(Succeed())
			Expect(testDB.Where("user_id = ?", other.ID).Delete(&model.UserWallet{}).Error).To(Succeed())
			Expect(testDB.Unscoped().Where("id = ?", other.ID).Delete(&model.User{}).Error).To(Succeed())
		}()

		Expect(pay(data.AccessToken, o.ID).Code).To(Equal(errno.ErrNotFoundRecord.FullCode()))
		Expect(balanceOf(other.ID)).To(Equal(float64(500)))
		Expect(balanceOf(buyer.ID)).To(Equal(float64(500)))
	})

	It("并发支付同一订单只扣款一次", func() {
		o := createOrder(2)
		const attempts = 12
		results := make(chan error, attempts)
		start := make(chan struct{})
		var wg sync.WaitGroup
		for i := 0; i < attempts; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				_, err := testOrderService.PayOrder(context.Background(), buyer.ID, "concurrent-session", o.ID)
				results <- err
			}()
		}
		close(start)
		wg.Wait()
		close(results)
		for err := range results {
			Expect(err).NotTo(HaveOccurred())
		}

		Expect(balanceOf(buyer.ID)).To(Equal(float64(300)))
		Expect(balanceOf(seller.ID)).To(Equal(float64(200)))
		Expect(paymentLogs()).To(HaveLen(2))
	})

	It("已支付订单不会被超时消息关闭", func() {
		o := createOrder(1)
		Expect(pay(buyerToken, o.ID).Code).To(Equal(errno.OK.FullCode()))

		Expect(testOrderService.HandleOrderTimeout(context.Background(), o.ID)).To(Succeed())
		var current model.Order
		Expect(testDB.First(&current, "id = ?", o.ID).Error).To(Succeed())
		Expect(current.Status).To(Equal(model.OrderStatusPaid))
		Expect(current.ClosedAt).To(BeNil())
		Expect(current.CloseReason).To(BeNil())
	})

	It("零元订单直接支付成功且不产生资金流水", func() {
		free := model.Product{
			ID: uuid.New(), SellerID: seller.ID, Name: "免费测试商品", Description: "fixture",
			Price: decimal.Zero, Stock: 1, Status: model.ProductStatusActive, Version: 1,
		}
		Expect(testDB.Create(&free).Error).To(Succeed())
		defer func() {
			Expect(testDB.Where("product_id = ?", free.ID).Delete(&model.StockChangeLog{}).Error).To(Succeed())
			Expect(testDB.Where("id = ?", free.ID).Delete(&model.Product{}).Error).To(Succeed())
		}()

		key := uuid.NewString()
		Expect(invariantRequest(http.MethodPost, "/api/v1/order/create", buyerToken, map[string]any{
			"product_id": free.ID.String(), "quantity": 1, "idempotency_key": key,
		}).Code).To(Equal(errno.OK.FullCode()))
		var o model.Order
		Expect(testDB.First(&o, "idempotency_key = ?", key).Error).To(Succeed())
		Expect(o.PayableAmount.IsZero()).To(BeTrue())

		resp := pay(buyerToken, o.ID)
		Expect(resp.Code).To(Equal(errno.OK.FullCode()))
		var detail struct {
			Status int `json:"status"`
		}
		Expect(json.Unmarshal(resp.Data, &detail)).To(Succeed())
		Expect(detail.Status).To(Equal(int(model.OrderStatusPaid)))
		Expect(balanceOf(buyer.ID)).To(Equal(float64(500)))
		Expect(paymentLogs()).To(BeEmpty())
	})

	It("支付响应中的金额与下单快照一致", func() {
		o := createOrder(3)
		Expect(pay(buyerToken, o.ID).Code).To(Equal(errno.OK.FullCode()))
		detail := invariantRequest(http.MethodGet, "/api/v1/order/"+o.ID.String(), buyerToken, nil)
		Expect(detail.Code).To(Equal(errno.OK.FullCode()))
		var fields struct {
			PayableAmount float64 `json:"payable_amount"`
			TotalAmount   float64 `json:"total_amount"`
			PaidAt        string  `json:"paid_at"`
		}
		Expect(json.Unmarshal(detail.Data, &fields)).To(Succeed())
		Expect(fields.PayableAmount).To(Equal(float64(300)))
		Expect(fields.TotalAmount).To(Equal(float64(300)))
		Expect(fields.PaidAt).NotTo(BeEmpty())
	})

	It("历史充值键占用了订单流水键时返回 A05104", func() {
		o := createOrder(1)

		// 模拟修复前被用户充值预占的系统键：唯一索引会拒绝本次支付流水。
		legacy := model.WalletLog{
			UserID:         buyer.ID,
			SessionID:      "legacy-deposit",
			Amount:         decimal.NewFromInt(1),
			Type:           model.WalletLogTypeDeposit,
			IdempotencyKey: "order:" + o.ID.String() + ":payment",
		}
		Expect(testDB.Create(&legacy).Error).To(Succeed())

		resp := pay(buyerToken, o.ID)
		Expect(resp.Code).To(Equal(errno.ErrOrderPaymentConflict.FullCode()))

		// 整个支付事务回滚：余额、订单状态和流水都不应变化。
		Expect(balanceOf(buyer.ID)).To(Equal(float64(500)))
		Expect(balanceOf(seller.ID)).To(BeZero())
		Expect(paymentLogs()).To(HaveLen(1))

		var current model.Order
		Expect(testDB.First(&current, "id = ?", o.ID).Error).To(Succeed())
		Expect(current.Status).To(Equal(model.OrderStatusProcessing))
		Expect(current.PaidAt).To(BeNil())
	})
})
