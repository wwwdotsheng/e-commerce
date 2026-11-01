package tests

import (
	"e-commerce/internal/model"
	"e-commerce/pkg/errno"
	"encoding/json"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gorm.io/gorm"
	"net/http"
	"os"
)

var _ = Describe("买家和商家独立余额", func() {
	It("两类账号注册即有零余额，充值及查询按登录账号隔离", func() {
		type account struct {
			ID    uuid.UUID
			Token string
		}
		register := func(kind string) account {
			name := "wallet_" + uuid.NewString()[:8]
			email := uuid.NewString() + "@example.com"
			r := invariantRequest(http.MethodPost, "/api/v1/user/register/"+kind, "", map[string]string{"user_name": name, "email": email, "password": "wallet-test-password", "seller_name": "余额测试商家", "application_note": "余额测试申请"})
			Expect(r.Code).To(Equal(errno.OK.FullCode()))
			var user model.User
			Expect(testDB.Where("email = ?", email).First(&user).Error).To(Succeed())
			var wallet model.UserWallet
			Expect(testDB.First(&wallet, "user_id = ?", user.ID).Error).To(Succeed())
			Expect(wallet.Balance.IsZero()).To(BeTrue())
			r = invariantRequest(http.MethodPost, "/api/v1/auth/login", "", map[string]string{"email": email, "password": "wallet-test-password"})
			Expect(r.Code).To(Equal(errno.OK.FullCode()))
			var pair struct {
				Token string `json:"access_token"`
			}
			Expect(json.Unmarshal(r.Data, &pair)).To(Succeed())
			profile := invariantRequest(http.MethodGet, "/api/v1/user/me", pair.Token, nil)
			Expect(profile.Code).To(Equal(errno.OK.FullCode()))
			var fields map[string]interface{}
			Expect(json.Unmarshal(profile.Data, &fields)).To(Succeed())
			Expect(fields["balance"]).To(Equal(float64(0)))
			return account{user.ID, pair.Token}
		}
		buyer, seller := register("buyer"), register("seller")
		Expect(invariantRequest(http.MethodGet, "/api/v1/wallet/balance", seller.Token, nil).Code).To(Equal(errno.ErrAuthNotPermission.FullCode()))
		// Approval fixtures are tested separately through the real admin review API.
		Expect(testDB.Model(&model.User{}).Where("id = ?", seller.ID).Update("status", model.UserStatusActive).Error).To(Succeed())
		read := func(a account, want float64) {
			r := invariantRequest(http.MethodGet, "/api/v1/wallet/balance?user_id="+uuid.NewString(), a.Token, nil)
			Expect(r.Code).To(Equal(errno.OK.FullCode()))
			var b struct {
				Balance float64 `json:"balance"`
			}
			Expect(json.Unmarshal(r.Data, &b)).To(Succeed())
			Expect(b.Balance).To(Equal(want))
			r = invariantRequest(http.MethodGet, "/api/v1/user/me", a.Token, nil)
			Expect(r.Code).To(Equal(errno.OK.FullCode()))
			Expect(json.Unmarshal(r.Data, &b)).To(Succeed())
			Expect(b.Balance).To(Equal(want))
		}
		read(buyer, 0)
		read(seller, 0)
		deposit := map[string]interface{}{"amount": 12.34, "idempotency_key": uuid.NewString()}
		Expect(invariantRequest(http.MethodPost, "/api/v1/wallet/deposit", buyer.Token, deposit).Code).To(Equal(errno.OK.FullCode()))
		Expect(invariantRequest(http.MethodPost, "/api/v1/wallet/deposit", buyer.Token, deposit).Code).To(Equal(errno.OK.FullCode()))
		read(buyer, 12.34)
		read(seller, 0)
		Expect(invariantRequest(http.MethodPost, "/api/v1/wallet/deposit", seller.Token, map[string]interface{}{"amount": 56.78, "idempotency_key": uuid.NewString()}).Code).To(Equal(errno.OK.FullCode()))
		read(buyer, 12.34)
		read(seller, 56.78)
		Expect(invariantRequest(http.MethodGet, "/api/v1/wallet/balance", "", nil).Code).To(Equal(errno.ErrAuthInvalidToken.FullCode()))
	})
	It("历史钱包补齐迁移不会覆盖已有余额且支持重复执行", func() {
		sql, err := os.ReadFile("../migrations/20261101000200_backfill_account_wallets.sql")
		Expect(err).NotTo(HaveOccurred())
		Expect(testDB.Transaction(func(tx *gorm.DB) error {
			if err := tx.Exec(`CREATE TEMP TABLE users (id uuid PRIMARY KEY) ON COMMIT DROP; CREATE TEMP TABLE user_wallets (user_id uuid PRIMARY KEY,balance numeric(16,2) NOT NULL,created_at timestamptz,updated_at timestamptz) ON COMMIT DROP`).Error; err != nil {
				return err
			}
			a, b := uuid.New(), uuid.New()
			if err := tx.Exec("INSERT INTO users(id) VALUES (?),(?)", a, b).Error; err != nil {
				return err
			}
			if err := tx.Exec("INSERT INTO user_wallets(user_id,balance) VALUES (?,99.99)", a).Error; err != nil {
				return err
			}
			for i := 0; i < 2; i++ {
				if err := tx.Exec(string(sql)).Error; err != nil {
					return err
				}
			}
			var rows []struct {
				UserID  uuid.UUID
				Balance float64
			}
			if err := tx.Raw("SELECT user_id,balance FROM user_wallets").Scan(&rows).Error; err != nil {
				return err
			}
			Expect(rows).To(HaveLen(2))
			for _, r := range rows {
				if r.UserID == a {
					Expect(r.Balance).To(Equal(99.99))
				} else {
					Expect(r.UserID).To(Equal(b))
					Expect(r.Balance).To(BeZero())
				}
			}
			return nil
		})).To(Succeed())
	})
})
