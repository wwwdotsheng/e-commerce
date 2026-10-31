-- Amount is an order snapshot, not recomputed from mutable product data.
ALTER TABLE orders
 ADD COLUMN payable_amount numeric(16,2),
 ADD COLUMN payment_deadline timestamptz,
 ADD COLUMN paid_at timestamptz NULL,
 ADD COLUMN closed_at timestamptz NULL;
UPDATE orders SET
 payable_amount = GREATEST(ROUND(snapshot_price * quantity - discount_amount, 2), 0),
 -- Existing rows did not store their TTL; use the historical 30-second default.
 payment_deadline = COALESCE(created_at, updated_at, CURRENT_TIMESTAMP) + INTERVAL '30 seconds';
-- Historical paid_at and closed_at remain NULL because their actual event times are unknown.
ALTER TABLE orders
 ALTER COLUMN payable_amount SET NOT NULL,
 ALTER COLUMN payment_deadline SET NOT NULL,
 ADD CONSTRAINT chk_orders_payable_amount CHECK (payable_amount >= 0);
CREATE INDEX idx_orders_payment_deadline ON orders(payment_deadline);
