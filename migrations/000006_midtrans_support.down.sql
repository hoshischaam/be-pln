DROP TRIGGER IF EXISTS trg_payout_requests_updated_at ON payout_requests;
DROP TRIGGER IF EXISTS trg_payment_orders_updated_at ON payment_orders;

DROP TABLE IF EXISTS payout_requests;
DROP TABLE IF EXISTS payment_orders;

DROP TYPE IF EXISTS payout_request_status;
DROP TYPE IF EXISTS payment_order_status;
