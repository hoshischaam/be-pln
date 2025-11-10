CREATE TYPE payment_order_status AS ENUM (
  'PENDING',
  'SETTLEMENT',
  'FAILED',
  'EXPIRED',
  'CANCELLED',
  'DENY'
);

CREATE TYPE payout_request_status AS ENUM (
  'PENDING',
  'REQUESTED',
  'FAILED',
  'COMPLETED'
);

CREATE TABLE payment_orders (
  id                     uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id                uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  order_id               varchar(64) NOT NULL UNIQUE,
  gross_amount           numeric(19,4) NOT NULL,
  snap_token             text NOT NULL,
  redirect_url           text NOT NULL,
  status                 payment_order_status NOT NULL DEFAULT 'PENDING',
  midtrans_transaction_id text,
  raw_notification       jsonb,
  settled_at             timestamptz,
  balance_applied        boolean NOT NULL DEFAULT false,
  created_at             timestamptz NOT NULL DEFAULT now(),
  updated_at             timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE payout_requests (
  id                       uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id                  uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  amount                   numeric(19,4) NOT NULL,
  bank_code                varchar(32) NOT NULL,
  bank_name                varchar(128),
  account_number           varchar(64) NOT NULL,
  account_holder_name      varchar(128) NOT NULL,
  status                   payout_request_status NOT NULL DEFAULT 'PENDING',
  midtrans_payout_id       text,
  raw_response             jsonb,
  requested_at             timestamptz NOT NULL DEFAULT now(),
  completed_at             timestamptz,
  created_at               timestamptz NOT NULL DEFAULT now(),
  updated_at               timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_payment_orders_user ON payment_orders(user_id);
CREATE INDEX idx_payment_orders_status ON payment_orders(status);
CREATE INDEX idx_payout_requests_user ON payout_requests(user_id);
CREATE INDEX idx_payout_requests_status ON payout_requests(status);

CREATE TRIGGER trg_payment_orders_updated_at
BEFORE UPDATE ON payment_orders
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_payout_requests_updated_at
BEFORE UPDATE ON payout_requests
FOR EACH ROW EXECUTE FUNCTION set_updated_at();
