-- 列表查询的排序索引。
-- 列顺序和方向必须与查询一致（WHERE user_id = ? ORDER BY created_at DESC, id DESC），
-- 才能同时走索引过滤和有序扫描，避免全表扫描 + 额外排序。
-- 当前数据量小，用普通 CREATE INDEX（事务内可回滚）；大表上线前应改为 CREATE INDEX CONCURRENTLY。
CREATE INDEX IF NOT EXISTS idx_orders_user_created
    ON orders (user_id, created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_user_coupon_user_created
    ON user_coupon (user_id, created_at DESC, id DESC);

-- 商品列表没有等值过滤，只按创建时间倒序分页。
CREATE INDEX IF NOT EXISTS idx_products_created
    ON products (created_at DESC, id DESC);
