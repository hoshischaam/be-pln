-- contoh index kombinasi untuk query histori transaksi per user
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_transactions_user_created_at
ON transactions(user_id, created_at DESC);
