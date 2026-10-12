-- 修正数据库 schema 与代码模型的漂移
--
-- 背景：
--   bootstrap.go 里 AutoMigrate 只在非生产环境跑（if !config.IsProd()），
--   而 prod 的 schema 变更本应由 migrations/ 承担 —— 但这一列的迁移从没被写出来。
--   结果：代码用了 shop_id / user_coupon_id / discount_amount，生产库一个都没有。
--
-- 影响：
--   POST /api/v1/product/create   → 500（column "shop_id" does not exist）
--   GET  /api/v1/product/list     → 500
--   POST /api/v1/order/create     → 500
--   /api/v1/shop/*                → 500（shops 表不存在）
--
-- 本迁移只做增量变更（加列 / 建表），不改动、不删除任何现有数据。
-- 全部使用 IF NOT EXISTS 保证可重复执行。

-- ── products: 补 shop_id ──────────────────────────────────────────
ALTER TABLE "public"."products" ADD COLUMN IF NOT EXISTS "shop_id" uuid NULL;
CREATE INDEX IF NOT EXISTS "idx_products_shop_id" ON "public"."products" ("shop_id");

-- ── orders: 补优惠券相关两列 ───────────────────────────────────────
ALTER TABLE "public"."orders" ADD COLUMN IF NOT EXISTS "user_coupon_id" uuid NULL;
ALTER TABLE "public"."orders" ADD COLUMN IF NOT EXISTS "discount_amount" numeric(16,2) NOT NULL DEFAULT 0;

-- ── shops: 建表 ──────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS "public"."shops" (
  "id" uuid NOT NULL,
  "owner_id" uuid NOT NULL,
  "name" character varying(128) NOT NULL,
  "description" text NULL,
  "created_at" timestamptz NULL,
  "updated_at" timestamptz NULL,
  PRIMARY KEY ("id")
);
CREATE INDEX IF NOT EXISTS "idx_shops_owner_id" ON "public"."shops" ("owner_id");

-- ── stock_change_logs: 建表 ──────────────────────────────────────
CREATE TABLE IF NOT EXISTS "public"."stock_change_logs" (
  "id" uuid NOT NULL DEFAULT gen_random_uuid(),
  "product_id" uuid NOT NULL,
  "quantity" bigint NOT NULL,
  "before" bigint NOT NULL,
  "after" bigint NOT NULL,
  "reason" smallint NOT NULL,
  "created_at" timestamptz NOT NULL,
  PRIMARY KEY ("id")
);
CREATE INDEX IF NOT EXISTS "idx_stock_change_logs_product_id" ON "public"."stock_change_logs" ("product_id");
CREATE INDEX IF NOT EXISTS "idx_stock_change_logs_reason"     ON "public"."stock_change_logs" ("reason");
CREATE INDEX IF NOT EXISTS "idx_stock_change_logs_created_at" ON "public"."stock_change_logs" ("created_at");
