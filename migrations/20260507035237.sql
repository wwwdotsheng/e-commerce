-- Create all tables (基础 schema，由 AutoMigrate 演化后纳入 Atlas 管理)
CREATE TABLE "public"."users" (
  "id" uuid NOT NULL,
  "user_name" character varying(30) NOT NULL,
  "email" character varying(254) NOT NULL,
  "password" character varying(255) NOT NULL,
  "created_at" timestamptz NULL,
  "updated_at" timestamptz NULL,
  "deleted_at" timestamptz NULL,
  PRIMARY KEY ("id")
);
CREATE INDEX "idx_users_deleted_at" ON "public"."users" ("deleted_at");
CREATE UNIQUE INDEX "uni_user_user_email" ON "public"."users" ("email");
CREATE UNIQUE INDEX "uni_user_user_name" ON "public"."users" ("user_name");

CREATE TABLE "public"."products" (
  "id" uuid NOT NULL,
  "publisher" uuid NOT NULL,
  "name" character varying(255) NOT NULL,
  "description" text NOT NULL,
  "price" numeric(16,2) NOT NULL,
  "stock" bigint NOT NULL DEFAULT 0,
  "frozen_stock" bigint NOT NULL DEFAULT 0,
  "status" text NOT NULL DEFAULT 'active',
  "version" bigint NOT NULL DEFAULT 0,
  "created_at" timestamptz NULL,
  "updated_at" timestamptz NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "chk_products_frozen_stock" CHECK (frozen_stock >= 0),
  CONSTRAINT "chk_products_stock" CHECK (stock >= 0)
);

CREATE TABLE "public"."orders" (
  "id" uuid NOT NULL,
  "user_id" uuid NULL,
  "product_id" uuid NULL,
  "quantity" bigint NOT NULL,
  "snapshot_title" text NOT NULL,
  "snapshot_price" numeric NOT NULL,
  "status" smallint NOT NULL,
  "idempotency_key" character varying(64) NOT NULL,
  "created_at" timestamptz NULL,
  "updated_at" timestamptz NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "chk_orders_quantity" CHECK (quantity >= 0)
);
CREATE UNIQUE INDEX "uni_order_idempotency_key" ON "public"."orders" ("idempotency_key");

CREATE TABLE "public"."user_wallets" (
  "user_id" uuid NOT NULL,
  "balance" numeric(16,2) NOT NULL DEFAULT 0,
  "updated_at" timestamptz NULL,
  PRIMARY KEY ("user_id")
);

CREATE TABLE "public"."wallet_logs" (
  "id" uuid NOT NULL,
  "user_id" uuid NOT NULL,
  "session_id" text NOT NULL,
  "amount" numeric(16,2) NOT NULL,
  "type" character varying(20) NOT NULL,
  "idempotency_key" character varying(64) NOT NULL,
  PRIMARY KEY ("id")
);
CREATE UNIQUE INDEX "uni_wallet_log_idempotency_key" ON "public"."wallet_logs" ("idempotency_key");
