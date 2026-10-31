package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"time"

	"e-commerce/internal/coupon"
	"e-commerce/internal/model"
	"e-commerce/internal/order"
	"e-commerce/internal/product"
	"e-commerce/internal/wallet"
	"e-commerce/pkg/errno"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel"
	"golang.org/x/crypto/bcrypt"
)

func invariantRequest(method, path, token string, body any) Response {
	GinkgoHelper()
	raw, err := json.Marshal(body)
	Expect(err).NotTo(HaveOccurred())
	req := httptest.NewRequest(method, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	// Login rate limits must not couple independently generated fixtures.
	req.RemoteAddr = fmt.Sprintf("192.0.2.%d:1234", int(uuid.New()[0])+1)
	w := httptest.NewRecorder()
	testRouter.ServeHTTP(w, req)
	Expect(w.Code).To(Equal(http.StatusOK))
	var resp Response
	Expect(json.Unmarshal(w.Body.Bytes(), &resp)).To(Succeed())
	return resp
}

var _ = Describe("Order invariants", func() {
	var buyer, other model.User
	var p model.Product
	var token, otherToken string
	var template model.CouponTemplate
	var uc model.UserCoupon

	BeforeEach(func() {
		hash, err := bcrypt.GenerateFromPassword([]byte("test123456"), bcrypt.MinCost)
		Expect(err).NotTo(HaveOccurred())
		buyer = model.User{ID: uuid.New(), UserName: "inv-buyer-" + uuid.NewString()[:8], Email: uuid.NewString() + "@example.test", Password: string(hash)}
		other = model.User{ID: uuid.New(), UserName: "inv-other-" + uuid.NewString()[:8], Email: uuid.NewString() + "@example.test", Password: string(hash)}
		Expect(testDB.Create(&buyer).Error).To(Succeed())
		Expect(testDB.Create(&other).Error).To(Succeed())
		p = model.Product{ID: uuid.New(), SellerID: other.ID, Name: "Invariant product", Description: "fixture", Price: 100, Stock: 5, Status: model.ProductStatusActive, Version: 1}
		Expect(testDB.Create(&p).Error).To(Succeed())
		template = model.CouponTemplate{ID: uuid.New(), Name: "Invariant coupon", Type: model.CouponTypeFixed, DiscountValue: 10, TotalQty: 10, RemainingQty: 10, PerUserLimit: 1, StartTime: time.Now().Add(-time.Hour), EndTime: time.Now().Add(time.Hour), Status: model.CouponStatusActive}
		Expect(testDB.Create(&template).Error).To(Succeed())
		uc = model.UserCoupon{ID: uuid.New(), UserID: buyer.ID, TemplateID: template.ID, Status: model.UserCouponStatusUnused, ExpireTime: time.Now().Add(time.Hour)}
		Expect(testDB.Create(&uc).Error).To(Succeed())
		login := func(email string) string {
			r := invariantRequest(http.MethodPost, "/api/v1/auth/login", "", map[string]string{"email": email, "password": "test123456"})
			Expect(r.Code).To(Equal(errno.OK.FullCode()))
			var data loginData
			Expect(json.Unmarshal(r.Data, &data)).To(Succeed())
			Expect(data.AccessToken).NotTo(BeEmpty())
			return data.AccessToken
		}
		token, otherToken = login(buyer.Email), login(other.Email)
	})

	AfterEach(func() {
		// Only delete IDs created by this spec, never shared fixtures.
		Expect(testDB.Where("user_id IN ?", []uuid.UUID{buyer.ID, other.ID}).Delete(&model.Order{}).Error).To(Succeed())
		Expect(testDB.Where("product_id = ?", p.ID).Delete(&model.StockChangeLog{}).Error).To(Succeed())
		Expect(testDB.Where("id = ?", p.ID).Delete(&model.Product{}).Error).To(Succeed())
		Expect(testDB.Where("id = ?", uc.ID).Delete(&model.UserCoupon{}).Error).To(Succeed())
		Expect(testDB.Unscoped().Where("id = ?", template.ID).Delete(&model.CouponTemplate{}).Error).To(Succeed())
		Expect(testDB.Where("user_id IN ?", []uuid.UUID{buyer.ID, other.ID}).Delete(&model.WalletLog{}).Error).To(Succeed())
		Expect(testDB.Where("user_id IN ?", []uuid.UUID{buyer.ID, other.ID}).Delete(&model.UserWallet{}).Error).To(Succeed())
		Expect(testDB.Unscoped().Where("id IN ?", []uuid.UUID{buyer.ID, other.ID}).Delete(&model.User{}).Error).To(Succeed())
	})

	create := func(key string, qty int, couponID string) Response {
		return invariantRequest(http.MethodPost, "/api/v1/order/create", token, map[string]any{"product_id": p.ID.String(), "quantity": qty, "idempotency_key": key, "coupon_id": couponID})
	}
	stock := func() int {
		GinkgoHelper()
		var current model.Product
		Expect(testDB.First(&current, "id = ?", p.ID).Error).To(Succeed())
		return current.Stock
	}
	findOrder := func(key string) model.Order {
		GinkgoHelper()
		var o model.Order
		Expect(testDB.First(&o, "idempotency_key = ?", key).Error).To(Succeed())
		return o
	}

	It("returns order details only to the owning user", func() {
		key := uuid.NewString()
		Expect(create(key, 2, "").Code).To(Equal(errno.OK.FullCode()))
		o := findOrder(key)

		resp := invariantRequest(http.MethodGet, "/api/v1/order/"+o.ID.String(), token, nil)
		Expect(resp.Code).To(Equal(errno.OK.FullCode()))
		var detail struct {
			ID             string  `json:"id"`
			ProductID      string  `json:"product_id"`
			Quantity       int     `json:"quantity"`
			SnapshotTitle  string  `json:"snapshot_title"`
			SnapshotPrice  float64 `json:"snapshot_price"`
			DiscountAmount float64 `json:"discount_amount"`
			TotalAmount    float64 `json:"total_amount"`
			Status         int     `json:"status"`
			CreatedAt      string  `json:"created_at"`
			UpdatedAt      string  `json:"updated_at"`
			Refund         *any    `json:"refund"`
		}
		Expect(json.Unmarshal(resp.Data, &detail)).To(Succeed())
		Expect(detail.ID).To(Equal(o.ID.String()))
		Expect(detail.ProductID).To(Equal(p.ID.String()))
		Expect(detail.Quantity).To(Equal(2))
		Expect(detail.SnapshotTitle).To(Equal(p.Name))
		Expect(detail.SnapshotPrice).To(Equal(p.Price))
		Expect(detail.DiscountAmount).To(BeZero())
		Expect(detail.TotalAmount).To(Equal(p.Price * 2))
		Expect(detail.Status).To(Equal(int(o.Status)))
		Expect(detail.CreatedAt).NotTo(BeEmpty())
		Expect(detail.UpdatedAt).NotTo(BeEmpty())
		Expect(detail.Refund).To(BeNil())

		otherUserResp := invariantRequest(http.MethodGet, "/api/v1/order/"+o.ID.String(), otherToken, nil)
		Expect(otherUserResp.Code).To(Equal(errno.ErrNotFoundRecord.FullCode()))
		missingOrderResp := invariantRequest(http.MethodGet, "/api/v1/order/"+uuid.NewString(), token, nil)
		Expect(missingOrderResp.Code).To(Equal(errno.ErrNotFoundRecord.FullCode()))
	})

	It("replays an exhausted-stock order without deducting twice", func() {
		key := uuid.NewString()
		Expect(create(key, 5, "").Code).To(Equal(errno.OK.FullCode()))
		Expect(create(key, 5, "").Code).To(Equal(errno.OK.FullCode()))
		Expect(stock()).To(Equal(0))
		var count int64
		Expect(testDB.Model(&model.Order{}).Where("idempotency_key = ?", key).Count(&count).Error).To(Succeed())
		Expect(count).To(Equal(int64(1)))
	})

	It("does not oversell under simultaneous requests", func() {
		const attempts = 16
		results := make(chan error, attempts)
		start := make(chan struct{})
		var wg sync.WaitGroup
		for i := 0; i < attempts; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				results <- testOrderService.CreateOrder(context.Background(), buyer.ID, order.CreateOrderParam{ProductID: p.ID, Quantity: 1, IdempotencyKey: uuid.NewString()})
			}()
		}
		close(start)
		wg.Wait()
		close(results)
		succeeded := 0
		for err := range results {
			if err == nil {
				succeeded++
			} else {
				Expect(errors.Is(err, errno.ErrProductStockInsufficient)).To(BeTrue(), "unexpected error: %v", err)
			}
		}
		Expect(succeeded).To(Equal(5))
		Expect(stock()).To(Equal(0))
		var count int64
		Expect(testDB.Model(&model.Order{}).Where("product_id = ?", p.ID).Count(&count).Error).To(Succeed())
		Expect(count).To(Equal(int64(5)))
	})

	It("rolls inventory back when the coupon belongs to another user", func() {
		Expect(testDB.Model(&uc).Update("user_id", other.ID).Error).To(Succeed())
		Expect(create(uuid.NewString(), 2, uc.ID.String()).Code).NotTo(Equal(errno.OK.FullCode()))
		Expect(stock()).To(Equal(5))
		var count int64
		Expect(testDB.Model(&model.Order{}).Where("product_id = ?", p.ID).Count(&count).Error).To(Succeed())
		Expect(count).To(BeZero())
	})

	It("links the used coupon to the real order and replays coupon orders", func() {
		key := uuid.NewString()
		Expect(create(key, 2, uc.ID.String()).Code).To(Equal(errno.OK.FullCode()))
		o := findOrder(key)
		var used model.UserCoupon
		Expect(testDB.First(&used, "id = ?", uc.ID).Error).To(Succeed())
		Expect(used.UsedOrderID).NotTo(BeNil())
		Expect(*used.UsedOrderID).To(Equal(o.ID))
		Expect(o.DiscountAmount).To(Equal(float64(10)))
		Expect(create(key, 2, uc.ID.String()).Code).To(Equal(errno.OK.FullCode()))
		Expect(stock()).To(Equal(3))
	})

	It("returns stock and coupon exactly once on repeated timeout", func() {
		key := uuid.NewString()
		Expect(create(key, 2, uc.ID.String()).Code).To(Equal(errno.OK.FullCode()))
		o := findOrder(key)
		for i := 0; i < 3; i++ {
			Expect(testOrderService.HandleOrderTimeout(context.Background(), o.ID)).To(Succeed())
		}
		Expect(stock()).To(Equal(5))
		Expect(findOrder(key).Status).To(Equal(model.OrderStatusTimeout))
		var returned model.UserCoupon
		Expect(testDB.First(&returned, "id = ?", uc.ID).Error).To(Succeed())
		Expect(returned.Status).To(Equal(model.UserCouponStatusUnused))
		Expect(returned.UsedOrderID).To(BeNil())
		var count int64
		Expect(testDB.Model(&model.StockChangeLog{}).Where("product_id = ? AND reason = ?", p.ID, model.StockChangeTimeout).Count(&count).Error).To(Succeed())
		Expect(count).To(Equal(int64(1)))
	})

	It("does not return a completed order's coupon or inventory", func() {
		key := uuid.NewString()
		Expect(create(key, 2, uc.ID.String()).Code).To(Equal(errno.OK.FullCode()))
		o := findOrder(key)
		Expect(testDB.Model(&model.Order{}).Where("id = ?", o.ID).Update("status", model.OrderStatusCompleted).Error).To(Succeed())
		Expect(testOrderService.HandleOrderTimeout(context.Background(), o.ID)).To(Succeed())
		Expect(stock()).To(Equal(3))
		var used model.UserCoupon
		Expect(testDB.First(&used, "id = ?", uc.ID).Error).To(Succeed())
		Expect(used.Status).To(Equal(model.UserCouponStatusUsed))
	})

	It("does not expose another user's orders", func() {
		Expect(create(uuid.NewString(), 1, "").Code).To(Equal(errno.OK.FullCode()))
		r := invariantRequest(http.MethodGet, "/api/v1/order/list?page_num=1&page_size=20", otherToken, nil)
		Expect(r.Code).To(Equal(errno.OK.FullCode()))
		var list listOrdersResponse
		Expect(json.Unmarshal(r.Data, &list)).To(Succeed())
		Expect(list.Total).To(BeZero())
		Expect(list.Orders).To(BeEmpty())
	})

	DescribeTable("rejects unauthenticated requests",
		func(tokenValue string) {
			r := invariantRequest(http.MethodPost, "/api/v1/order/create", tokenValue, map[string]any{"product_id": p.ID.String(), "quantity": 1, "idempotency_key": uuid.NewString()})
			Expect(r.Code).To(Equal(errno.ErrAuthInvalidToken.FullCode()))
			Expect(stock()).To(Equal(5))
		}, Entry("missing token", ""), Entry("malformed token", "invalid-token"),
	)

	It("rejects an expired opaque access token", func() {
		ctx := context.Background()
		sessions, err := testRedis.SMembers(ctx, fmt.Sprintf("ident:u_sess:%s", buyer.ID)).Result()
		Expect(err).NotTo(HaveOccurred())
		Expect(sessions).To(HaveLen(1))
		Expect(testRedis.HSet(ctx, "ident:sess:"+sessions[0], "at_exp", time.Now().Add(-time.Minute).Unix()).Err()).To(Succeed())
		r := invariantRequest(http.MethodGet, "/api/v1/order/list?page_num=1&page_size=20", token, nil)
		Expect(r.Code).To(Equal(errno.ErrAuthTokenExpired.FullCode()))
	})
	It("revokes order access when the session is logged out", func() {
		Expect(invariantRequest(http.MethodPost, "/api/v1/auth/logout", token, nil).Code).To(Equal(errno.OK.FullCode()))
		Expect(invariantRequest(http.MethodGet, "/api/v1/order/list?page_num=1&page_size=20", token, nil).Code).NotTo(Equal(errno.OK.FullCode()))
	})

	It("credits a repeated concurrent deposit only once", func() {
		metrics, err := wallet.NewMetrics(otel.Meter("wallet-invariants"))
		Expect(err).NotTo(HaveOccurred())
		svc := wallet.NewService(wallet.NewRepository(testDB, testRedis), metrics)
		key := uuid.NewString()
		results := make(chan error, 12)
		var wg sync.WaitGroup
		for i := 0; i < 12; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				results <- svc.Deposit(context.Background(), buyer.ID, "test-session", &wallet.DepositInput{Amount: 100, IdempotencyKey: key})
			}()
		}
		wg.Wait()
		close(results)
		for err := range results {
			Expect(err).NotTo(HaveOccurred())
		}
		var balance model.UserWallet
		Expect(testDB.First(&balance, "user_id = ?", buyer.ID).Error).To(Succeed())
		Expect(balance.Balance).To(Equal(float64(100)))
		var count int64
		Expect(testDB.Model(&model.WalletLog{}).Where("user_id = ?", buyer.ID).Count(&count).Error).To(Succeed())
		Expect(count).To(Equal(int64(1)))
	})

	It("rolls the entire timeout back when returning the coupon fails", func() {
		key := uuid.NewString()
		Expect(create(key, 2, uc.ID.String()).Code).To(Equal(errno.OK.FullCode()))
		o := findOrder(key)
		name := "fail_coupon_" + uuid.NewString()[:8]
		Expect(testDB.Exec(fmt.Sprintf("CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected coupon failure'; END $$", name)).Error).To(Succeed())
		Expect(testDB.Exec(fmt.Sprintf("CREATE TRIGGER %s BEFORE UPDATE ON user_coupon FOR EACH ROW WHEN (OLD.id = '%s'::uuid) EXECUTE FUNCTION %s()", name, uc.ID, name)).Error).To(Succeed())
		DeferCleanup(func() {
			Expect(testDB.Exec("DROP TRIGGER " + name + " ON user_coupon").Error).To(Succeed())
			Expect(testDB.Exec("DROP FUNCTION " + name + "()").Error).To(Succeed())
		})
		Expect(testOrderService.HandleOrderTimeout(context.Background(), o.ID)).NotTo(Succeed())
		Expect(findOrder(key).Status).To(Equal(model.OrderStatusProcessing))
		Expect(stock()).To(Equal(3))
	})

	It("releases inventory once under concurrent timeout deliveries", func() {
		key := uuid.NewString()
		Expect(create(key, 2, "").Code).To(Equal(errno.OK.FullCode()))
		o := findOrder(key)
		results := make(chan error, 10)
		var wg sync.WaitGroup
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); results <- testOrderService.HandleOrderTimeout(context.Background(), o.ID) }()
		}
		wg.Wait()
		close(results)
		for err := range results {
			Expect(err).NotTo(HaveOccurred())
		}
		Expect(stock()).To(Equal(5))
		var count int64
		Expect(testDB.Model(&model.StockChangeLog{}).Where("product_id = ? AND reason = ?", p.ID, model.StockChangeTimeout).Count(&count).Error).To(Succeed())
		Expect(count).To(Equal(int64(1)))
	})

	It("does not return a reused coupon when an old timeout is delivered again", func() {
		firstKey := uuid.NewString()
		Expect(create(firstKey, 2, uc.ID.String()).Code).To(Equal(errno.OK.FullCode()))
		first := findOrder(firstKey)
		Expect(testOrderService.HandleOrderTimeout(context.Background(), first.ID)).To(Succeed())
		secondKey := uuid.NewString()
		Expect(create(secondKey, 1, uc.ID.String()).Code).To(Equal(errno.OK.FullCode()))
		second := findOrder(secondKey)
		Expect(testOrderService.HandleOrderTimeout(context.Background(), first.ID)).To(Succeed())
		var current model.UserCoupon
		Expect(testDB.First(&current, "id = ?", uc.ID).Error).To(Succeed())
		Expect(current.Status).To(Equal(model.UserCouponStatusUsed))
		Expect(current.UsedOrderID).NotTo(BeNil())
		Expect(*current.UsedOrderID).To(Equal(second.ID))
		Expect(stock()).To(Equal(4))
	})

	It("rejects a key reused by another account or with another quantity", func() {
		key := uuid.NewString()
		Expect(create(key, 1, "").Code).To(Equal(errno.OK.FullCode()))
		Expect(create(key, 2, "").Code).To(Equal(errno.ErrInvalidParam.FullCode()))
		r := invariantRequest(http.MethodPost, "/api/v1/order/create", otherToken, map[string]any{"product_id": p.ID.String(), "quantity": 1, "idempotency_key": key})
		Expect(r.Code).To(Equal(errno.ErrInvalidParam.FullCode()))
		Expect(stock()).To(Equal(4))
	})

	It("processes the real TTL dead-letter queue and releases stock", func() {
		conn, err := amqp.Dial(testRabbitMQURL)
		Expect(err).NotTo(HaveOccurred())
		ch, err := conn.Channel()
		Expect(err).NotTo(HaveOccurred())
		ctx, cancel := context.WithCancel(context.Background())
		cfg := testOrderMQConfig
		prefix := "inv-" + uuid.NewString()
		cfg.Exchange = prefix + "-exchange"
		cfg.ConsumerQueue = prefix + "-consumer"
		cfg.DelayQueue = prefix + "-delay"
		cfg.TTLMs = 100
		DeferCleanup(func() {
			cancel()
			_, _ = ch.QueueDelete(cfg.ConsumerQueue, false, false, false)
			_, _ = ch.QueueDelete(cfg.DelayQueue, false, false, false)
			_ = ch.ExchangeDelete(cfg.Exchange, false, false)
			_ = conn.Close()
		})
		repo := order.NewRepository(testDB, ch, &cfg)
		Expect(repo.SetupMQ(&cfg)).To(Succeed())
		metrics, err := order.NewMetrics(otel.Meter("mq-invariants"))
		Expect(err).NotTo(HaveOccurred())
		svc := order.NewService(testDB, repo, product.NewRepository(testDB), coupon.NewRepository(testDB), wallet.NewRepository(testDB, testRedis), metrics)
		Expect(order.NewMqHandler(svc).ListenTimeout(ctx, ch, cfg.ConsumerQueue)).To(Succeed())
		key := uuid.NewString()
		Expect(svc.CreateOrder(ctx, buyer.ID, order.CreateOrderParam{ProductID: p.ID, Quantity: 2, IdempotencyKey: key})).To(Succeed())
		Eventually(func() (model.OrderStatus, error) {
			var o model.Order
			err := testDB.First(&o, "idempotency_key = ?", key).Error
			return o.Status, err
		}, 5*time.Second, 25*time.Millisecond).Should(Equal(model.OrderStatusTimeout))
		Expect(stock()).To(Equal(5))
	})
})
