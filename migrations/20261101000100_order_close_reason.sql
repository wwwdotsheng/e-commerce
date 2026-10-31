ALTER TABLE orders ADD COLUMN close_reason varchar(32) NULL;
UPDATE orders SET close_reason = 'payment_timeout' WHERE status = 3;
ALTER TABLE orders ADD CONSTRAINT chk_orders_close_reason
 CHECK (close_reason IS NULL OR (close_reason = 'payment_timeout' AND status = 3));
