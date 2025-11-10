package repositories

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// =============== Errors ===============
type ErrNotFound struct{ Message string }

func (e ErrNotFound) Error() string { return e.Message }

// DBTX adalah interface minimal untuk *sql.DB dan *sql.Tx
type DBTX interface {
	ExecContext(ctx context.Context, q string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, q string, args ...any) *sql.Row
	QueryContext(ctx context.Context, q string, args ...any) (*sql.Rows, error)
}

// =============== Params struct dipakai Service ===============
type CreateTransactionParams struct {
	UserID        string
	TipeTransaksi string // contoh: "TOP_UP"
	Jumlah        float64
	Deskripsi     string
	ReferensiID   *string // boleh nil
	CreatedAt     time.Time
}

type AddSaldoParams struct {
	UserID      string
	DeltaTotal  float64
	DeltaTopup  float64
	DeltaRedeem float64
	UpdatedAt   time.Time
}

type TransactionRecord struct {
	ID          string
	UserID      string
	Type        string
	Amount      float64
	Description sql.NullString
	ReferenceID sql.NullString
	CreatedAt   time.Time
}

type PaymentOrderRecord struct {
	ID               string
	UserID           string
	OrderID          string
	GrossAmount      float64
	SnapToken        string
	RedirectURL      string
	Status           string
	MidtransTrxID    sql.NullString
	RawNotification  sql.NullString
	SettledAt        sql.NullTime
	BalanceApplied   bool
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type CreatePaymentOrderParams struct {
	UserID      string
	OrderID     string
	GrossAmount float64
	SnapToken   string
	RedirectURL string
}

type UpdatePaymentOrderStatusParams struct {
	OrderID            string
	Status             string
	MidtransTrxID      *string
	RawNotification    []byte
	SettledAt          *time.Time
	BalanceAlreadyUsed *bool
}

type PayoutRequestRecord struct {
	ID                 string
	UserID             string
	Amount             float64
	BankCode           string
	BankName           sql.NullString
	AccountNumber      string
	AccountHolderName  string
	Status             string
	MidtransPayoutID   sql.NullString
	RawResponse        sql.NullString
	RequestedAt        time.Time
	CompletedAt        sql.NullTime
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type CreatePayoutRequestParams struct {
	UserID            string
	Amount            float64
	BankCode          string
	BankName          string
	AccountNumber     string
	AccountHolderName string
	Status            string
	MidtransPayoutID  *string
	RawResponse       []byte
	RequestedAt       time.Time
	CompletedAt       *time.Time
}

type UpdatePayoutRequestStatusParams struct {
	ID               string
	Status           string
	MidtransPayoutID *string
	RawResponse      []byte
	CompletedAt      *time.Time
}

type UserProfile struct {
	ID    string
	Email string
	Name  string
	Phone string
}

type VoucherRecord struct {
	ID           string
	Code         string
	Amount       float64
	Description  sql.NullString
	ExpiresAt    sql.NullTime
	Active       bool
	Quota        sql.NullInt64
	PerUserLimit sql.NullInt64
	CreatedAt    time.Time
}

// =============== Interface ===============
type WalletRepo interface {
	BeginTx(ctx context.Context) (*sql.Tx, error)

	// Read saldo
	GetSaldo(ctx context.Context, userID string) (total, topup, redeem float64, evPoin int, err error)
	GetSaldoForUpdate(ctx context.Context, tx DBTX, userID string) (total, topup, redeem float64, evPoin int, err error)
	ListTransactions(ctx context.Context, userID string, limit int) ([]TransactionRecord, error)
	ListAvailableVouchers(ctx context.Context, userID string, limit int) ([]VoucherRecord, error)
	GetUserProfile(ctx context.Context, userID string) (*UserProfile, error)

	// Voucher
	GetVoucherByCode(ctx context.Context, kode string) (id string, nilai float64, aktif bool, exp sql.NullTime, err error)
	CreateVoucherClaim(ctx context.Context, tx DBTX, userID, voucherID string, now time.Time) (bool, error)

	// Transaksi & saldo
	CreateTransaction(ctx context.Context, tx DBTX, p CreateTransactionParams) error
	AddSaldo(ctx context.Context, tx DBTX, p AddSaldoParams) error

	// Payment orders
	CreatePaymentOrder(ctx context.Context, p CreatePaymentOrderParams) error
	GetPaymentOrder(ctx context.Context, orderID string) (PaymentOrderRecord, error)
	UpdatePaymentOrderStatus(ctx context.Context, p UpdatePaymentOrderStatusParams) error

	// Payout requests
	CreatePayoutRequest(ctx context.Context, p CreatePayoutRequestParams) (PayoutRequestRecord, error)
	UpdatePayoutRequestStatus(ctx context.Context, p UpdatePayoutRequestStatusParams) error
	GetPayoutRequestByID(ctx context.Context, id string) (PayoutRequestRecord, error)
	UpdatePaymentOrderStatusTx(ctx context.Context, tx DBTX, p UpdatePaymentOrderStatusParams) error
	UpdatePayoutRequestStatusTx(ctx context.Context, tx DBTX, p UpdatePayoutRequestStatusParams) error
}

// =============== Implementasi ===============
type walletRepo struct{ db *sql.DB }

func NewWalletRepo(db *sql.DB) WalletRepo { return &walletRepo{db: db} }

func (r *walletRepo) BeginTx(ctx context.Context) (*sql.Tx, error) {
	return r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
}

// --- Saldo ---
func (r *walletRepo) GetSaldo(ctx context.Context, userID string) (float64, float64, float64, int, error) {
	const q = `
  SELECT total_saldo, saldo_topup, saldo_redeem, ev_poin
  FROM wallet_summary
  WHERE user_id = $1
`
	var tot, topup, redeem float64
	var poin int
	err := r.db.QueryRowContext(ctx, q, userID).Scan(&tot, &topup, &redeem, &poin)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, 0, 0, 0, ErrNotFound{"wallet not found"}
	}
	return tot, topup, redeem, poin, err
}

func (r *walletRepo) GetSaldoForUpdate(ctx context.Context, tx DBTX, userID string) (float64, float64, float64, int, error) {
	const q = `
  SELECT total_saldo, saldo_topup, saldo_redeem, ev_poin
  FROM wallet_summary
  WHERE user_id = $1
  FOR UPDATE
`
	var tot, topup, redeem float64
	var poin int
	err := tx.QueryRowContext(ctx, q, userID).Scan(&tot, &topup, &redeem, &poin)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, 0, 0, 0, ErrNotFound{"wallet not found"}
	}
	return tot, topup, redeem, poin, err
}

func (r *walletRepo) GetUserProfile(ctx context.Context, userID string) (*UserProfile, error) {
	const q = `
		SELECT id, email, COALESCE(phone, '')
		FROM users
		WHERE id = $1
	`
	var (
		prof UserProfile
		phone string
	)
	err := r.db.QueryRowContext(ctx, q, userID).Scan(&prof.ID, &prof.Email, &phone)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound{Message: "user not found"}
	}
	if err != nil {
		return nil, err
	}
	prof.Phone = phone
	if idx := strings.Index(prof.Email, "@"); idx > 0 {
		prof.Name = prof.Email[:idx]
	}
	return &prof, nil
}

// --- Klaim voucher ---
func (r *walletRepo) WasVoucherClaimed(ctx context.Context, userID, kode string) (bool, error) {
	// sesuai skema kamu: user_voucher_claims(voucher_id) + vouchers(kode_voucher)
	const q = `
		SELECT EXISTS (
			SELECT 1
			FROM user_voucher_claims uvc
			JOIN vouchers v ON v.id = uvc.voucher_id
			WHERE uvc.user_id = $1 AND v.kode_voucher = $2
		)
	`
	var ok bool
	if err := r.db.QueryRowContext(ctx, q, userID, kode).Scan(&ok); err != nil {
		return false, err
	}
	return ok, nil
}

func (r *walletRepo) CreateVoucherClaim(ctx context.Context, tx DBTX, userID, voucherID string, now time.Time) (bool, error) {
	const q = `
		INSERT INTO user_voucher_claims (id, user_id, voucher_id, claimed_at)
		VALUES (gen_random_uuid(), $1, $2, $3)
		ON CONFLICT (user_id, voucher_id) DO NOTHING`
	res, err := tx.ExecContext(ctx, q, userID, voucherID, now)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

func (r *walletRepo) GetVoucherByCode(ctx context.Context, kode string) (id string, nilai float64, aktif bool, exp sql.NullTime, err error) {
	const q = `SELECT id, nilai, aktif, tanggal_kadaluarsa FROM vouchers WHERE kode_voucher=$1`
	err = r.db.QueryRowContext(ctx, q, kode).Scan(&id, &nilai, &aktif, &exp)
	return
}

func (r *walletRepo) ListTransactions(ctx context.Context, userID string, limit int) ([]TransactionRecord, error) {
	if limit <= 0 {
		limit = 50
	}
	const q = `
		SELECT id, user_id, tipe_transaksi, jumlah, deskripsi, referensi_id, created_at
		FROM transactions
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`
	rows, err := r.db.QueryContext(ctx, q, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var res []TransactionRecord
	for rows.Next() {
		var rec TransactionRecord
		if err := rows.Scan(&rec.ID, &rec.UserID, &rec.Type, &rec.Amount, &rec.Description, &rec.ReferenceID, &rec.CreatedAt); err != nil {
			return nil, err
		}
		res = append(res, rec)
	}
	return res, rows.Err()
}

func (r *walletRepo) ListAvailableVouchers(ctx context.Context, userID string, limit int) ([]VoucherRecord, error) {
	if limit <= 0 {
		limit = 50
	}
	const q = `
		SELECT v.id,
		       v.kode_voucher,
		       v.nilai,
		       v.deskripsi,
		       v.tanggal_kadaluarsa,
		       v.aktif,
		       v.kuota,
		       v.per_user_limit,
		       v.created_at
		FROM vouchers v
		WHERE v.aktif = TRUE
		  AND (v.tanggal_kadaluarsa IS NULL OR v.tanggal_kadaluarsa > NOW())
		  AND (
		    v.kuota IS NULL OR v.kuota > (
		      SELECT COUNT(*) FROM user_voucher_claims WHERE voucher_id = v.id
		    )
		  )
		  AND NOT EXISTS (
		    SELECT 1 FROM user_voucher_claims uvc WHERE uvc.user_id = $1 AND uvc.voucher_id = v.id
		  )
		ORDER BY v.created_at DESC
		LIMIT $2
	`
	rows, err := r.db.QueryContext(ctx, q, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var res []VoucherRecord
	for rows.Next() {
		var vr VoucherRecord
		if err := rows.Scan(&vr.ID, &vr.Code, &vr.Amount, &vr.Description, &vr.ExpiresAt, &vr.Active, &vr.Quota, &vr.PerUserLimit, &vr.CreatedAt); err != nil {
			return nil, err
		}
		res = append(res, vr)
	}
	return res, rows.Err()
}

// --- Top up ---
func (r *walletRepo) CreateTransaction(ctx context.Context, tx DBTX, p CreateTransactionParams) error {
	const q = `
		INSERT INTO transactions (id, user_id, tipe_transaksi, jumlah, deskripsi, referensi_id, created_at)
		VALUES (gen_random_uuid(), $1, $2::transaction_type, $3, $4, $5, $6)
	`
	_, err := tx.ExecContext(ctx, q,
		p.UserID, p.TipeTransaksi, p.Jumlah, p.Deskripsi, p.ReferensiID, p.CreatedAt,
	)
	return err
}

func (r *walletRepo) AddSaldo(ctx context.Context, tx DBTX, p AddSaldoParams) error {
	// UPSERT: kalau baris belum ada, insert; kalau ada, tambah delta
	const q = `
		INSERT INTO wallet_summary (user_id, total_saldo, saldo_topup, saldo_redeem, ev_poin, updated_at)
  VALUES ($1, $2, $3, $4, 0, $5)
  ON CONFLICT (user_id) DO UPDATE
  SET total_saldo   = wallet_summary.total_saldo   + EXCLUDED.total_saldo,
      saldo_topup   = wallet_summary.saldo_topup   + EXCLUDED.saldo_topup,
      saldo_redeem  = wallet_summary.saldo_redeem  + EXCLUDED.saldo_redeem,
      updated_at    = EXCLUDED.updated_at
	`
	_, err := tx.ExecContext(ctx, q, p.UserID, p.DeltaTotal, p.DeltaTopup, p.DeltaRedeem, p.UpdatedAt)
	return err
}

func (r *walletRepo) CreatePaymentOrder(ctx context.Context, p CreatePaymentOrderParams) error {
	const q = `
		INSERT INTO payment_orders (user_id, order_id, gross_amount, snap_token, redirect_url)
		VALUES ($1, $2, $3, $4, $5)
	`
	_, err := r.db.ExecContext(ctx, q, p.UserID, p.OrderID, p.GrossAmount, p.SnapToken, p.RedirectURL)
	return err
}

func (r *walletRepo) GetPaymentOrder(ctx context.Context, orderID string) (PaymentOrderRecord, error) {
	const q = `
		SELECT id, user_id, order_id, gross_amount, snap_token, redirect_url, status,
		       midtrans_transaction_id, raw_notification, settled_at, balance_applied,
		       created_at, updated_at
		FROM payment_orders
		WHERE order_id = $1
	`
	var rec PaymentOrderRecord
	err := r.db.QueryRowContext(ctx, q, orderID).Scan(
		&rec.ID,
		&rec.UserID,
		&rec.OrderID,
		&rec.GrossAmount,
		&rec.SnapToken,
		&rec.RedirectURL,
		&rec.Status,
		&rec.MidtransTrxID,
		&rec.RawNotification,
		&rec.SettledAt,
		&rec.BalanceApplied,
		&rec.CreatedAt,
		&rec.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return rec, ErrNotFound{Message: "payment order not found"}
	}
	return rec, err
}

func (r *walletRepo) updatePaymentOrderStatus(ctx context.Context, exec DBTX, p UpdatePaymentOrderStatusParams) error {
const q = `
	UPDATE payment_orders
	SET status = $2::payment_order_status,
	    midtrans_transaction_id = COALESCE($3, midtrans_transaction_id),
	    raw_notification = COALESCE($4, raw_notification),
	    settled_at = COALESCE($5, settled_at),
	    balance_applied = COALESCE($6, balance_applied),
	    updated_at = now()
	WHERE order_id = $1
`
	_, err := exec.ExecContext(ctx, q, p.OrderID, p.Status, p.MidtransTrxID, p.RawNotification, p.SettledAt, p.BalanceAlreadyUsed)
	return err
}

func (r *walletRepo) UpdatePaymentOrderStatus(ctx context.Context, p UpdatePaymentOrderStatusParams) error {
	return r.updatePaymentOrderStatus(ctx, r.db, p)
}

func (r *walletRepo) UpdatePaymentOrderStatusTx(ctx context.Context, tx DBTX, p UpdatePaymentOrderStatusParams) error {
	return r.updatePaymentOrderStatus(ctx, tx, p)
}

func (r *walletRepo) CreatePayoutRequest(ctx context.Context, p CreatePayoutRequestParams) (PayoutRequestRecord, error) {
	const q = `
		INSERT INTO payout_requests (
			user_id, amount, bank_code, bank_name, account_number, account_holder_name,
			status, midtrans_payout_id, raw_response, requested_at, completed_at
		)
	VALUES ($1, $2, $3, $4, $5, $6, $7::payout_request_status, $8, $9, $10, $11)
		RETURNING id, user_id, amount, bank_code, bank_name, account_number, account_holder_name,
		          status, midtrans_payout_id, raw_response, requested_at, completed_at, created_at, updated_at
	`
	var rec PayoutRequestRecord
	err := r.db.QueryRowContext(ctx, q,
		p.UserID,
		p.Amount,
		p.BankCode,
		p.BankName,
		p.AccountNumber,
		p.AccountHolderName,
		p.Status,
		p.MidtransPayoutID,
		p.RawResponse,
		p.RequestedAt,
		p.CompletedAt,
	).Scan(
		&rec.ID,
		&rec.UserID,
		&rec.Amount,
		&rec.BankCode,
		&rec.BankName,
		&rec.AccountNumber,
		&rec.AccountHolderName,
		&rec.Status,
		&rec.MidtransPayoutID,
		&rec.RawResponse,
		&rec.RequestedAt,
		&rec.CompletedAt,
		&rec.CreatedAt,
		&rec.UpdatedAt,
	)
	return rec, err
}

func (r *walletRepo) updatePayoutRequestStatus(ctx context.Context, exec DBTX, p UpdatePayoutRequestStatusParams) error {
	const q = `
		UPDATE payout_requests
		SET status = $2,
		    midtrans_payout_id = COALESCE($3, midtrans_payout_id),
		    raw_response = COALESCE($4, raw_response),
		    completed_at = COALESCE($5, completed_at),
		    updated_at = now()
		WHERE id = $1
	`
	_, err := exec.ExecContext(ctx, q, p.ID, p.Status, p.MidtransPayoutID, p.RawResponse, p.CompletedAt)
	return err
}

func (r *walletRepo) UpdatePayoutRequestStatus(ctx context.Context, p UpdatePayoutRequestStatusParams) error {
	return r.updatePayoutRequestStatus(ctx, r.db, p)
}

func (r *walletRepo) UpdatePayoutRequestStatusTx(ctx context.Context, tx DBTX, p UpdatePayoutRequestStatusParams) error {
	return r.updatePayoutRequestStatus(ctx, tx, p)
}

func (r *walletRepo) GetPayoutRequestByID(ctx context.Context, id string) (PayoutRequestRecord, error) {
	const q = `
		SELECT id, user_id, amount, bank_code, bank_name, account_number, account_holder_name,
		       status, midtrans_payout_id, raw_response, requested_at, completed_at,
		       created_at, updated_at
		FROM payout_requests
		WHERE id = $1
	`
	var rec PayoutRequestRecord
	err := r.db.QueryRowContext(ctx, q, id).Scan(
		&rec.ID,
		&rec.UserID,
		&rec.Amount,
		&rec.BankCode,
		&rec.BankName,
		&rec.AccountNumber,
		&rec.AccountHolderName,
		&rec.Status,
		&rec.MidtransPayoutID,
		&rec.RawResponse,
		&rec.RequestedAt,
		&rec.CompletedAt,
		&rec.CreatedAt,
		&rec.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return rec, ErrNotFound{Message: "payout request not found"}
	}
	return rec, err
}
