ALTER TABLE users
 ADD COLUMN user_type varchar(16) NOT NULL DEFAULT 'buyer',
 ADD COLUMN status varchar(16) NOT NULL DEFAULT 'active',
 ADD COLUMN shop_name varchar(128) NOT NULL DEFAULT '',
 ADD COLUMN application_note varchar(1000) NOT NULL DEFAULT '',
 ADD COLUMN review_reason varchar(1000) NOT NULL DEFAULT '',
 ADD COLUMN reviewed_by uuid REFERENCES users(id),
 ADD COLUMN reviewed_at timestamptz;
-- Preserve existing shop owners and product publishers as active sellers.
UPDATE users SET user_type = 'seller' WHERE id IN (SELECT owner_id FROM shops UNION SELECT publisher FROM products);
ALTER TABLE users ADD CONSTRAINT users_user_type_check CHECK (user_type IN ('buyer','seller','admin'));
ALTER TABLE users ADD CONSTRAINT users_status_check CHECK (status IN ('pending','active','rejected','disabled'));
ALTER TABLE users ADD CONSTRAINT users_review_status_check CHECK (status NOT IN ('pending','rejected') OR user_type = 'seller');
CREATE INDEX idx_users_seller_review ON users(user_type,status,created_at);
CREATE TABLE seller_application_reviews (
 id bigserial PRIMARY KEY,
 user_id uuid NOT NULL REFERENCES users(id),
 reviewer_id uuid NOT NULL REFERENCES users(id),
 decision varchar(16) NOT NULL CHECK (decision IN ('approve','reject')),
 reason varchar(1000) NOT NULL DEFAULT '',
 shop_name varchar(128) NOT NULL,
 application_note varchar(1000) NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_seller_application_reviews_user ON seller_application_reviews(user_id,created_at);
