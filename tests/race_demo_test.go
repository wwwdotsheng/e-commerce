package tests

import (
	"context"
	"e-commerce/internal/model"
	"sync"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gorm.io/gorm"
)

// ============================================================
// 教学演示：如何测试出业务代码中的竞态条件
// ============================================================
// 场景：钱包充值
//
// 错误写法（NaiveDeposit）：先查询余额 → 计算新余额 → 写回
// 正确写法（AtomicDeposit）：SET balance = balance + ? 原子操作
//
// 步骤：
//   RED:   跑 NaiveDeposit，看到余额丢失更新
//   GREEN: 用 AtomicDeposit 修复，验证余额正确
// ============================================================

// NaiveDeposit 模拟"经典竞态写法"：先查询余额，再计算，再更新
// 这是最常见的 read-modify-write 竞态模式
func NaiveDeposit(db *gorm.DB, userID uuid.UUID, amount float64) error {
	var wallet model.UserWallet
	err := db.Where("user_id = ?", userID).First(&wallet).Error
	if err != nil {
		return err
	}

	// 竞态窗口在这里！
	// 两个并发 goroutine 可能同时读到 balance=0
	// 各自计算 newBalance=100
	// 各自写回 balance=100，丢失了一次更新
	newBalance := wallet.Balance + amount

	return db.Model(&model.UserWallet{}).
		Where("user_id = ?", userID).
		Update("balance", newBalance).Error
}

// AtomicDeposit 修复版：用数据库原子表达式，没有 read-modify-write 窗口
func AtomicDeposit(db *gorm.DB, userID uuid.UUID, amount float64) error {
	return db.Model(&model.UserWallet{}).
		Where("user_id = ?", userID).
		Update("balance", gorm.Expr("balance + ?", amount)).Error
}

// ConcurrentDeposit 并发充值 helper：启动 n 个 goroutine，每个充值 amount 元
func ConcurrentDeposit(ctx context.Context, db *gorm.DB, userID uuid.UUID, n int, amount float64, fn func(*gorm.DB, uuid.UUID, float64) error) []error {
	var wg sync.WaitGroup
	errs := make([]error, n)

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			errs[idx] = fn(db, userID, amount)
		}(i)
	}

	wg.Wait()
	return errs
}

var _ = Describe("Race Condition 教学演示", func() {
	var userID uuid.UUID

	BeforeEach(func() {
		userID = uuid.New()

		// 创建一个初始余额为 0 的钱包
		err := testDB.Create(&model.UserWallet{
			UserID:  userID,
			Balance: 0,
		}).Error
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		// 清理测试数据
		testDB.Where("user_id = ?", userID).Delete(&model.UserWallet{})
	})

	It("RED: 非原子充值会丢失更新（并发竞态复现）", func() {
		const goroutines = 10
		const depositAmount = 100.0

		// 10 个 goroutine 并发充值 100 元
		errs := ConcurrentDeposit(context.Background(), testDB, userID, goroutines, depositAmount, NaiveDeposit)

		// 验证：没有报错
		for i, err := range errs {
			Expect(err).NotTo(HaveOccurred(), "goroutine %d 报错", i)
		}

		// 读取最终余额
		var wallet model.UserWallet
		err := testDB.Where("user_id = ?", userID).First(&wallet).Error
		Expect(err).NotTo(HaveOccurred())

		// 预期：10 * 100 = 1000
		// 实际：因为竞态，小于 1000
		GinkgoWriter.Printf("非原子充值结果：期望 %.0f，实际 %.2f\n", goroutines*depositAmount, wallet.Balance)
		Expect(wallet.Balance).To(BeNumerically("<", goroutines*depositAmount),
			"并发 10 次非原子充值后余额应该小于 1000（丢失更新了）")
	})

	It("GREEN: 原子充值不会丢失更新（竞态修复验证）", func() {
		const goroutines = 10
		const depositAmount = 100.0

		// 10 个 goroutine 并发原子充值 100 元
		errs := ConcurrentDeposit(context.Background(), testDB, userID, goroutines, depositAmount, AtomicDeposit)

		// 验证：没有报错
		for i, err := range errs {
			Expect(err).NotTo(HaveOccurred(), "goroutine %d 报错", i)
		}

		// 读取最终余额
		var wallet model.UserWallet
		err := testDB.Where("user_id = ?", userID).First(&wallet).Error
		Expect(err).NotTo(HaveOccurred())

		// 预期：10 * 100 = 1000
		// 实际：原子操作，一定是 1000
		GinkgoWriter.Printf("原子充值结果：期望 %.0f，实际 %.2f\n", goroutines*depositAmount, wallet.Balance)
		Expect(wallet.Balance).To(Equal(goroutines*depositAmount),
			"并发 10 次原子充值后余额应该是准确的 1000")
	})
})
