ALTER TABLE vouchers
  ADD COLUMN IF NOT EXISTS kuota INT,
  ADD COLUMN IF NOT EXISTS per_user_limit INT;

CREATE INDEX IF NOT EXISTS idx_vouchers_active ON vouchers(aktif)
  WHERE aktif = TRUE;
