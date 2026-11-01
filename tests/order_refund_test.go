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

var _ = Describe("订单退款", func() {
	var buyer, seller model.User
	var product model.Product
	var buyerToken, sellerToken string

	newUser := func(kind model.UserType, name string) model.User {
		GinkgoHelper()
		hash, err := bcrypt.GenerateFromPassword([]byte("refund-test-password"), bcrypt.MinCost)
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

	login := func(email string) string {
		GinkgoHelper()
		resp := invariantRequest(http.MethodPost, "/api/v1/auth/login", "", map[string]string{
			"email": email, "password": "refund-test-password",
		})
		Expect(resp.Code).To(Equal(errno.OK.FullCode()))
		var data loginData
		Expect(json.Unmarshal(resp.Data, &data)).To(Succeed())
		Expect(data.AccessToken).NotTo(BeEmpty())
		return data.AccessToken
	}

	setPrice := func(price decimal.Decimal) {
		GinkgoHelper()
		Expect(testDB.Model(&model.Product{}).Where("id = ?", product.ID).Update("price", price).Error).To(Succeed())
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

	createPaidOrder := func(qty int) model.Order {
		GinkgoHelper()
		o := createOrder(qty)
		Expect(invariantRequest(http.MethodPost, "/api/v1/order/"+o.ID.String()+"/pay", buyerToken, nil).Code).
			To(Equal(errno.OK.FullCode()))
		Expect(testDB.First(&o, "id = ?", o.ID).Error).To(Succeed())
		Expect(o.Status).To(Equal(model.OrderStatusPaid))
		return o
	}

	apply := func(orderID uuid.UUID, key string) Response {
		GinkgoHelper()
		return invariantRequest(http.MethodPost, "/api/v1/order/"+orderID.String()+"/refund", buyerToken,
			map[string]any{"idempotency_key": key})
	}

	get := func(orderID uuid.UUID, token string) Response {
		GinkgoHelper()
		return invariantRequest(http.MethodGet, "/api/v1/order/"+orderID.String()+"/refund", token, nil)
	}

	type refundData struct {
		ID            string  `json:"id"`
		Status        string  `json:"status"`
		Amount        float64 `json:"amount"`
		FailureReason string  `json:"failure_reason"`
		UpdatedAt     string  `json:"updated_at"`
	}
	decodeRefund := func(resp Response) refundData {
		GinkgoHelper()
		var data refundData
		Expect(json.Unmarshal(resp.Data, &data)).To(Succeed())
		return data
	}
	loadRefund := func(orderID uuid.UUID) model.Refund {
		GinkgoHelper()
		var r model.Refund
		Expect(testDB.First(&r, "order_id = ?", orderID).Error).To(Succeed())
		return r
	}
	loadOrder := func(orderID uuid.UUID) model.Order {
		GinkgoHelper()
		var o model.Order
		Expect(testDB.First(&o, "id = ?", orderID).Error).To(Succeed())
		return o
	}

	BeforeEach(func() {
		buyer = newUser(model.UserTypeBuyer, "refund-buyer")
		seller = newUser(model.UserTypeSeller, "refund-seller")
		Expect(testDB.Create(&model.UserWallet{UserID: buyer.ID, Balance: decimal.NewFromInt(500)}).Error).To(Succeed())
		Expect(testDB.Create(&model.UserWallet{UserID: seller.ID, Balance: decimal.Zero}).Error).To(Succeed())
		product = model.Product{
			ID: uuid.New(), SellerID: seller.ID, Name: "退款测试商品", Description: "fixture",
			Price: decimal.NewFromInt(99), Stock: 10, Status: model.ProductStatusActive, Version: 1,
		}
		Expect(testDB.Create(&product).Error).To(Succeed())
		buyerToken = login(buyer.Email)
		sellerToken = login(seller.Email)
	})

	AfterEach(func() {
		Expect(testDB.Exec("DELETE FROM refunds WHERE buyer_id = ?", buyer.ID).Error).To(Succeed())
		Expect(testDB.Where("user_id = ? OR seller_id = ?", buyer.ID, seller.ID).Delete(&model.Order{}).Error).To(Succeed())
		Expect(testDB.Where("product_id = ?", product.ID).Delete(&model.StockChangeLog{}).Error).To(Succeed())
		Expect(testDB.Where("id = ?", product.ID).Delete(&model.Product{}).Error).To(Succeed())
		Expect(testDB.Where("user_id IN ?", []uuid.UUID{buyer.ID, seller.ID}).Delete(&model.WalletLog{}).Error).To(Succeed())
		Expect(testDB.Where("user_id IN ?", []uuid.UUID{buyer.ID, seller.ID}).Delete(&model.UserWallet{}).Error).To(Succeed())
		Expect(testDB.Unscoped().Where("id IN ?", []uuid.UUID{buyer.ID, seller.ID}).Delete(&model.User{}).Error).To(Succeed())
	})

	It("全额退款成功：退款记录成功、订单变为已退款、详情可查询", func() {
		// 99 * 2 = 198.00，金额（分）%3 == 0，模拟渠道返回成功。
		o := createPaidOrder(2)
		refund := decodeRefund(apply(o.ID, uuid.NewString()))
		Expect(refund.Status).To(Equal(string(model.RefundStatusSucceeded)))
		Expect(refund.Amount).To(Equal(float64(198)))

		stored := loadRefund(o.ID)
		Expect(stored.RefundedAt).NotTo(BeNil())
		Expect(loadOrder(o.ID).Status).To(Equal(model.OrderStatusRefunded))

		detail := decodeRefund(get(o.ID, buyerToken))
		Expect(detail.ID).To(Equal(refund.ID))
		Expect(detail.Status).To(Equal(string(model.RefundStatusSucceeded)))

		var orderDetail struct {
			Status int         `json:"status"`
			Refund *refundData `json:"refund"`
		}
		Expect(json.Unmarshal(invariantRequest(http.MethodGet, "/api/v1/order/"+o.ID.String(), buyerToken, nil).Data, &orderDetail)).To(Succeed())
		Expect(orderDetail.Status).To(Equal(int(model.OrderStatusRefunded)))
		Expect(orderDetail.Refund).NotTo(BeNil())
		Expect(orderDetail.Refund.Status).To(Equal(string(model.RefundStatusSucceeded)))
	})

	It("明确失败：记录原因，同幂等键不重试，新幂等键允许重试", func() {
		// 100.00 的金额（分）%3 == 1，模拟渠道明确失败。
		setPrice(decimal.NewFromInt(100))
		o := createPaidOrder(1)
		key := uuid.NewString()

		first := decodeRefund(apply(o.ID, key))
		Expect(first.Status).To(Equal(string(model.RefundStatusFailed)))
		Expect(first.FailureReason).NotTo(BeEmpty())
		Expect(loadOrder(o.ID).Status).To(Equal(model.OrderStatusPaid))

		same := decodeRefund(apply(o.ID, key))
		Expect(same.ID).To(Equal(first.ID))
		Expect(loadRefund(o.ID).Attempts).To(Equal(1))

		retried := decodeRefund(apply(o.ID, uuid.NewString()))
		Expect(retried.ID).To(Equal(first.ID))
		Expect(loadRefund(o.ID).Attempts).To(Equal(2))
		Expect(loadOrder(o.ID).Status).To(Equal(model.OrderStatusPaid))
	})

	It("结果不明：先记 confirming，查询确认后成功并更新订单", func() {
		// 98.00 的金额（分）%3 == 2，模拟渠道超时返回结果不明。
		setPrice(decimal.NewFromInt(98))
		o := createPaidOrder(1)

		applied := decodeRefund(apply(o.ID, uuid.NewString()))
		Expect(applied.Status).To(Equal(string(model.RefundStatusConfirming)))
		Expect(loadOrder(o.ID).Status).To(Equal(model.OrderStatusPaid))

		confirmed := decodeRefund(get(o.ID, buyerToken))
		Expect(confirmed.Status).To(Equal(string(model.RefundStatusSucceeded)))
		Expect(loadOrder(o.ID).Status).To(Equal(model.OrderStatusRefunded))
		Expect(loadRefund(o.ID).RefundedAt).NotTo(BeNil())
	})

	It("重复申请只产生一条退款记录", func() {
		o := createPaidOrder(2)
		key := uuid.NewString()
		first := decodeRefund(apply(o.ID, key))
		second := decodeRefund(apply(o.ID, key))
		Expect(second.ID).To(Equal(first.ID))

		var count int64
		Expect(testDB.Model(&model.Refund{}).Where("order_id = ?", o.ID).Count(&count).Error).To(Succeed())
		Expect(count).To(Equal(int64(1)))
		Expect(loadRefund(o.ID).Attempts).To(Equal(1))
	})

	It("未支付订单不可退款，非买家角色被拒绝，无退款记录查询返回不存在", func() {
		unpaid := createOrder(1)
		Expect(apply(unpaid.ID, uuid.NewString()).Code).To(Equal(errno.ErrOrderRefundStatusInvalid.FullCode()))

		o := createPaidOrder(2)
		Expect(apply(o.ID, uuid.NewString()).Code).To(Equal(errno.OK.FullCode()))
		Expect(get(o.ID, buyerToken).Code).To(Equal(errno.OK.FullCode()))

		paidNoRefund := createPaidOrder(2)
		Expect(get(paidNoRefund.ID, buyerToken).Code).To(Equal(errno.ErrNotFoundRecord.FullCode()))

		// 卖家既不是买家也不能查询/申请退款。
		Expect(apply(o.ID, uuid.NewString()).Code).To(Equal(errno.OK.FullCode()))
		Expect(invariantRequest(http.MethodPost, "/api/v1/order/"+o.ID.String()+"/refund", sellerToken,
			map[string]any{"idempotency_key": uuid.NewString()}).Code).To(Equal(errno.ErrAuthNotPermission.FullCode()))
		Expect(get(o.ID, sellerToken).Code).To(Equal(errno.ErrAuthNotPermission.FullCode()))
	})
})
