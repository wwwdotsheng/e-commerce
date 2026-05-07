-- Modify "user_wallets" table
ALTER TABLE "public"."user_wallets" ADD COLUMN "created_at" timestamptz NULL;
-- Modify "wallet_logs" table
ALTER TABLE "public"."wallet_logs" ADD COLUMN "created_at" timestamptz NULL;
