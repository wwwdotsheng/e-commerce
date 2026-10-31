-- Account identity is the merchant identity. Preserve historical shop data.
ALTER TABLE users RENAME COLUMN shop_name TO seller_name;
ALTER TABLE seller_application_reviews RENAME COLUMN shop_name TO seller_name;
ALTER TABLE products RENAME COLUMN publisher TO seller_id;

UPDATE users u SET seller_name = legacy.name
FROM (SELECT DISTINCT ON (owner_id) owner_id, name FROM shops ORDER BY owner_id, created_at, id) legacy
WHERE u.id = legacy.owner_id AND u.user_type = 'seller' AND btrim(u.seller_name) = '';
UPDATE users SET seller_name = user_name WHERE user_type = 'seller' AND btrim(seller_name) = '';
CREATE INDEX idx_products_seller_id ON products(seller_id);
-- shops and products.shop_id remain historical data only; application code no longer uses them.

ALTER TABLE orders ADD COLUMN seller_id uuid;
UPDATE orders o SET seller_id = p.seller_id FROM products p WHERE o.product_id = p.id;
ALTER TABLE orders ALTER COLUMN seller_id SET NOT NULL;
CREATE INDEX idx_orders_seller_id ON orders(seller_id);
