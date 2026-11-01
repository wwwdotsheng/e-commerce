-- 订单退款记录：一期仅支持全额退款，一个订单最多一条退款记录（唯一索引兜底重复申请）。
CREATE TABLE IF NOT EXISTS refunds (
    id uuid PRIMARY KEY,
    order_id uuid NOT NULL,
    buyer_id uuid NOT NULL,
    amount numeric(16,2) NOT NULL CHECK (amount >= 0),
    status varchar(16) NOT NULL,
    failure_reason varchar(255) NOT NULL DEFAULT '',
    idempotency_key varchar(64) NOT NULL,
    attempts integer NOT NULL DEFAULT 0,
    refunded_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS uni_refunds_order_id ON refunds (order_id);
CREATE INDEX IF NOT EXISTS idx_refunds_buyer_created ON refunds (buyer_id, created_at DESC, id DESC);
