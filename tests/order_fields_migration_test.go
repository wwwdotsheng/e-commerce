package tests

import (
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gorm.io/gorm"
	"os"
	"time"
)

var _ = Describe("订单字段迁移", func() {
	It("回填历史应付金额和默认截止时间，不伪造支付及关闭时间", func() {
		sql, err := os.ReadFile("../migrations/20261031000300_order_payment_fields.sql")
		Expect(err).NotTo(HaveOccurred())
		Expect(testDB.Transaction(func(tx *gorm.DB) error {
			if err := tx.Exec(`CREATE TEMP TABLE orders (id uuid PRIMARY KEY, snapshot_price numeric NOT NULL, quantity bigint NOT NULL, discount_amount numeric(16,2) NOT NULL, status smallint NOT NULL, created_at timestamptz, updated_at timestamptz) ON COMMIT DROP`).Error; err != nil {
				return err
			}
			created := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
			for _, discount := range []float64{10, 200} {
				if err := tx.Exec(`INSERT INTO orders (id,snapshot_price,quantity,discount_amount,status,created_at,updated_at) VALUES (?,59.99,2,?,3,?,?)`, uuid.New(), discount, created, created.Add(time.Minute)).Error; err != nil {
					return err
				}
			}
			if err := tx.Exec(string(sql)).Error; err != nil {
				return err
			}
			var rows []struct {
				PayableAmount   float64
				PaymentDeadline time.Time
				PaidAt          *time.Time
				ClosedAt        *time.Time
			}
			if err := tx.Raw("SELECT payable_amount,payment_deadline,paid_at,closed_at FROM orders ORDER BY discount_amount").Scan(&rows).Error; err != nil {
				return err
			}
			Expect(rows).To(HaveLen(2))
			Expect(rows[0].PayableAmount).To(Equal(109.98))
			Expect(rows[1].PayableAmount).To(BeZero())
			for _, r := range rows {
				Expect(r.PaymentDeadline.Equal(created.Add(30 * time.Second))).To(BeTrue())
				Expect(r.PaidAt).To(BeNil())
				Expect(r.ClosedAt).To(BeNil())
			}
			return nil
		})).To(Succeed())
	})
})
