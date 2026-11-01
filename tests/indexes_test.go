package tests

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// 列表分页依赖 (user_id, created_at, id) 的复合索引；
// 这条用例保证迁移确实被应用，而不是只在代码里改了 ORDER BY。
var _ = Describe("列表查询索引", func() {
	It("订单、用户券和商品列表的排序索引已创建", func() {
		for _, name := range []string{
			"idx_orders_user_created",
			"idx_user_coupon_user_created",
			"idx_products_created",
		} {
			var count int64
			Expect(testDB.Raw(
				"SELECT count(*) FROM pg_indexes WHERE schemaname = 'public' AND indexname = ?", name,
			).Scan(&count).Error).To(Succeed())
			Expect(count).To(Equal(int64(1)), "索引 %s 不存在", name)
		}
	})
})
