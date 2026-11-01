package tests

import (
	"context"
	"e-commerce/internal/model"
	"e-commerce/pkg/errno"
	"encoding/json"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/shopspring/decimal"
	"golang.org/x/crypto/bcrypt"
	"net/http"
	"time"
)

var _ = Describe("订单金额与状态时间快照", func() {
	var buyer, seller model.User
	var product model.Product
	var token string
	BeforeEach(func() {
		hash, err := bcrypt.GenerateFromPassword([]byte("order-field-test"), bcrypt.MinCost)
		Expect(err).NotTo(HaveOccurred())
		buyer = model.User{ID: uuid.New(), UserName: "buyer_" + uuid.NewString()[:8], Email: uuid.NewString() + "@example.com", Password: string(hash), UserType: model.UserTypeBuyer, Status: model.UserStatusActive}
		seller = model.User{ID: uuid.New(), UserName: "seller_" + uuid.NewString()[:8], Email: uuid.NewString() + "@example.com", Password: string(hash), UserType: model.UserTypeSeller, Status: model.UserStatusActive, SellerName: "订单测试商家"}
		Expect(testDB.Create(&buyer).Error).To(Succeed())
		Expect(testDB.Create(&seller).Error).To(Succeed())
		product = model.Product{ID: uuid.New(), SellerID: seller.ID, Name: "下单时商品名称", Description: "测试商品", Price: decimal.RequireFromString("59.99"), Stock: 10, Status: model.ProductStatusActive}
		Expect(testDB.Create(&product).Error).To(Succeed())
		r := invariantRequest(http.MethodPost, "/api/v1/auth/login", "", map[string]string{"email": buyer.Email, "password": "order-field-test"})
		Expect(r.Code).To(Equal(errno.OK.FullCode()))
		var pair struct {
			Token string `json:"access_token"`
		}
		Expect(json.Unmarshal(r.Data, &pair)).To(Succeed())
		token = pair.Token
	})
	create := func(discount decimal.Decimal) (model.Order, map[string]interface{}) {
		template := model.CouponTemplate{ID: uuid.New(), Name: "订单优惠", Type: model.CouponTypeFixed, DiscountValue: discount, TotalQty: 10, RemainingQty: 10, PerUserLimit: 1, StartTime: time.Now().Add(-time.Hour), EndTime: time.Now().Add(time.Hour), Status: model.CouponStatusActive}
		Expect(testDB.Create(&template).Error).To(Succeed())
		coupon := model.UserCoupon{ID: uuid.New(), UserID: buyer.ID, TemplateID: template.ID, Status: model.UserCouponStatusUnused, ExpireTime: time.Now().Add(time.Hour)}
		Expect(testDB.Create(&coupon).Error).To(Succeed())
		body := map[string]interface{}{"product_id": product.ID.String(), "quantity": 2, "coupon_id": coupon.ID.String(), "idempotency_key": uuid.NewString()}
		Expect(invariantRequest(http.MethodPost, "/api/v1/order/create", token, body).Code).To(Equal(errno.OK.FullCode()))
		var order model.Order
		Expect(testDB.Where("idempotency_key = ?", body["idempotency_key"]).First(&order).Error).To(Succeed())
		return order, body
	}
	It("下单保存金额及截止时间，修改商品和幂等重试均不改变订单", func() {
		order, body := create(decimal.NewFromInt(10))
		Expect(order.PayableAmount.Equal(decimal.RequireFromString("109.98"))).To(BeTrue())
		Expect(order.DiscountAmount.Equal(decimal.NewFromInt(10))).To(BeTrue())
		Expect(order.PaymentDeadline.Sub(order.CreatedAt)).To(Equal(time.Duration(testOrderMQConfig.TTLMs) * time.Millisecond))
		Expect(order.PaidAt).To(BeNil())
		Expect(order.ClosedAt).To(BeNil())
		Expect(order.CloseReason).To(BeNil())
		Expect(testDB.Model(&product).Updates(map[string]interface{}{"name": "商品改名", "price": 888}).Error).To(Succeed())
		Expect(invariantRequest(http.MethodPost, "/api/v1/order/create", token, body).Code).To(Equal(errno.OK.FullCode()))
		var again model.Order
		Expect(testDB.First(&again, "id = ?", order.ID).Error).To(Succeed())
		Expect(again.SnapshotTitle).To(Equal("下单时商品名称"))
		Expect(again.SnapshotPrice.Equal(decimal.RequireFromString("59.99"))).To(BeTrue())
		Expect(again.PayableAmount.Equal(decimal.RequireFromString("109.98"))).To(BeTrue())
		Expect(again.PaymentDeadline).To(Equal(order.PaymentDeadline))
		result := invariantRequest(http.MethodGet, "/api/v1/order/list?page_num=1&page_size=10", token, nil)
		Expect(result.Code).To(Equal(errno.OK.FullCode()))
		var list struct {
			Orders []struct {
				Payable float64 `json:"payable_amount"`
				Total   float64 `json:"total_amount"`
			} `json:"orders"`
		}
		Expect(json.Unmarshal(result.Data, &list)).To(Succeed())
		Expect(list.Orders).To(HaveLen(1))
		Expect(list.Orders[0].Payable).To(Equal(109.98))
		Expect(list.Orders[0].Total).To(Equal(109.98))
	})
	It("优惠超过总额时仍保存零应付金额，不限制原优惠金额", func() {
		order, _ := create(decimal.NewFromInt(200))
		Expect(order.DiscountAmount.Equal(decimal.NewFromInt(200))).To(BeTrue())
		Expect(order.PayableAmount.IsZero()).To(BeTrue())
	})
	It("超时关闭保存原因和时间，重复消费不覆盖", func() {
		order, _ := create(decimal.NewFromInt(10))
		Expect(testDB.Model(&order).Update("payment_deadline", time.Now().Add(-time.Second)).Error).To(Succeed())
		Expect(testOrderService.HandleOrderTimeout(context.Background(), order.ID)).To(Succeed())
		var closed model.Order
		Expect(testDB.First(&closed, "id = ?", order.ID).Error).To(Succeed())
		Expect(closed.Status).To(Equal(model.OrderStatusTimeout))
		Expect(closed.ClosedAt).NotTo(BeNil())
		Expect(closed.CloseReason).NotTo(BeNil())
		Expect(*closed.CloseReason).To(Equal(model.OrderCloseReasonPaymentTimeout))
		Expect(closed.PaidAt).To(BeNil())
		Expect(testOrderService.HandleOrderTimeout(context.Background(), order.ID)).To(Succeed())
		var replay model.Order
		Expect(testDB.First(&replay, "id = ?", order.ID).Error).To(Succeed())
		Expect(replay.ClosedAt).To(Equal(closed.ClosedAt))
		Expect(replay.CloseReason).To(Equal(closed.CloseReason))
		Expect(replay.PayableAmount.Equal(order.PayableAmount)).To(BeTrue())
	})
	It("已完成订单不会被超时消息补写关闭时间", func() {
		order, _ := create(decimal.NewFromInt(10))
		Expect(testDB.Model(&order).Update("status", model.OrderStatusCompleted).Error).To(Succeed())
		Expect(testOrderService.HandleOrderTimeout(context.Background(), order.ID)).To(Succeed())
		var saved model.Order
		Expect(testDB.First(&saved, "id = ?", order.ID).Error).To(Succeed())
		Expect(saved.ClosedAt).To(BeNil())
		Expect(saved.CloseReason).To(BeNil())
		Expect(saved.PaidAt).To(BeNil())
	})
	It("退券失败时关闭原因和关闭时间随事务一起回滚", func() {
		order, _ := create(decimal.NewFromInt(10))
		Expect(testDB.Model(&model.UserCoupon{}).Where("id = ?", order.UserCouponID).Update("status", model.UserCouponStatusUnused).Error).To(Succeed())
		Expect(testDB.Model(&model.Order{}).Where("id = ?", order.ID).Update("payment_deadline", time.Now().Add(-time.Second)).Error).To(Succeed())
		Expect(testOrderService.HandleOrderTimeout(context.Background(), order.ID)).NotTo(Succeed())
		var saved model.Order
		Expect(testDB.First(&saved, "id = ?", order.ID).Error).To(Succeed())
		Expect(saved.Status).To(Equal(model.OrderStatusProcessing))
		Expect(saved.ClosedAt).To(BeNil())
		Expect(saved.CloseReason).To(BeNil())
		var savedProduct model.Product
		Expect(testDB.First(&savedProduct, "id = ?", product.ID).Error).To(Succeed())
		Expect(savedProduct.Stock).To(Equal(8))
	})

})
