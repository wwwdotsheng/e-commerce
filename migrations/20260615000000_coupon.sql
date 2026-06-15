-- 优惠券模块
CREATE TABLE IF NOT EXISTS coupon_template (
    id             UUID PRIMARY KEY,
    name           VARCHAR(128)    NOT NULL,
    type           VARCHAR(16)     NOT NULL DEFAULT 'fixed_amount',
    discount_value DECIMAL(16,2)   NOT NULL DEFAULT 0,
    discount_rate  DECIMAL(5,2)    NOT NULL DEFAULT 0,
    max_deduction  DECIMAL(16,2)   NOT NULL DEFAULT 0,
    min_amount     DECIMAL(16,2)   NOT NULL DEFAULT 0,
    total_qty      INT             NOT NULL,
    remaining_qty  INT             NOT NULL,
    per_user_limit INT             NOT NULL DEFAULT 1,
    start_time     TIMESTAMPTZ     NOT NULL,
    end_time       TIMESTAMPTZ     NOT NULL,
    status         VARCHAR(16)     NOT NULL DEFAULT 'active',
    version        INT             NOT NULL DEFAULT 0,
    created_at     TIMESTAMPTZ     NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ     NOT NULL DEFAULT NOW(),
    deleted_at     TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS user_coupon (
    id            UUID PRIMARY KEY,
    user_id       UUID        NOT NULL,
    template_id   UUID        NOT NULL,
    status        VARCHAR(16) NOT NULL DEFAULT 'unused',
    used_order_id UUID,
    used_at       TIMESTAMPTZ,
    expire_time   TIMESTAMPTZ NOT NULL,
    version       INT         NOT NULL DEFAULT 0,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_user_coupon_user_id ON user_coupon(user_id);
CREATE INDEX IF NOT EXISTS idx_user_coupon_template_id ON user_coupon(template_id);
CREATE INDEX IF NOT EXISTS idx_coupon_template_status ON coupon_template(status);
