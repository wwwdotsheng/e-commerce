-- Buyer and seller accounts share one wallet each; do not duplicate existing balances.
INSERT INTO user_wallets(user_id,balance,created_at,updated_at)
SELECT id,0,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP FROM users
ON CONFLICT (user_id) DO NOTHING;
