CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TYPE transaction_type AS ENUM (
  'KLAIM_VOUCHER',
  'TARIK_SALDO_PENDAPATAN',
  'TARIK_SALDO_REFUND',
  'TOP_UP'
);


CREATE TABLE users (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  email         varchar(255) NOT NULL UNIQUE,
  password_hash varchar(255) NOT NULL,
  created_at    timestamptz NOT NULL DEFAULT now(),
  updated_at    timestamptz NOT NULL DEFAULT now()
);


CREATE TABLE wallet_summary (
  user_id         uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  total_saldo     numeric(19,4) NOT NULL DEFAULT 0.00,
  saldo_pendapatan numeric(19,4) NOT NULL DEFAULT 0.00,
  saldo_refund    numeric(19,4) NOT NULL DEFAULT 0.00,
  ev_poin         int NOT NULL DEFAULT 0,
  updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE vouchers (
  id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  kode_voucher        varchar(50) NOT NULL UNIQUE,
  nilai               numeric(19,4) NOT NULL,
  deskripsi           text,
  tanggal_kadaluarsa  timestamptz,
  aktif               boolean NOT NULL DEFAULT true,
  created_at          timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE user_voucher_claims (
  id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  voucher_id uuid NOT NULL REFERENCES vouchers(id) ON DELETE RESTRICT,
  claimed_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT user_voucher_claims_unique UNIQUE (user_id, voucher_id)
);

CREATE TABLE transactions (
  id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id        uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  tipe_transaksi transaction_type NOT NULL,
  jumlah         numeric(19,4) NOT NULL,
  deskripsi      text,
  referensi_id   varchar(255),
  created_at     timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE sessions (
  id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id            uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  refresh_token_hash text NOT NULL,
  user_agent         text,
  ip_address         text,
  expires_at         timestamptz NOT NULL,
  revoked_at         timestamptz,
  created_at         timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE password_reset_tokens (
  id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  token_hash text NOT NULL,
  expires_at timestamptz NOT NULL,
  used_at    timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);


CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS trigger AS $$
BEGIN
  NEW.updated_at = now();
  RETURN NEW;
END; $$ LANGUAGE plpgsql;

CREATE TRIGGER trg_users_updated_at BEFORE UPDATE ON users FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_wallet_summary_updated_at BEFORE UPDATE ON wallet_summary FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE INDEX IF NOT EXISTS idx_sessions_user_active ON sessions (user_id) WHERE revoked_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_password_reset_tokens_user_id ON password_reset_tokens(user_id);
CREATE INDEX IF NOT EXISTS idx_transactions_user_id ON transactions(user_id);
CREATE INDEX IF NOT EXISTS idx_user_voucher_claims_user_id ON user_voucher_claims(user_id);
CREATE INDEX IF NOT EXISTS idx_vouchers_kode_voucher ON vouchers(kode_voucher);