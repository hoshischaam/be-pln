package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/hoshichaam/pln_backend_go/internal/repositories"
)

type WalletService struct {
	repo     repositories.WalletRepo
	validate *validator.Validate
	now      func() time.Time
	snapClient        *SnapClient
	irisClient        *IrisClient
	midtransServerKey string
	callbackToken     string
}

func NewWalletService(r repositories.WalletRepo, v *validator.Validate, snap *SnapClient, iris *IrisClient, serverKey, callbackToken string) *WalletService {
	return &WalletService{
		repo:              r,
		validate:          v,
		now:               time.Now,
		snapClient:        snap,
		irisClient:        iris,
		midtransServerKey: serverKey,
		callbackToken:     callbackToken,
	}
}

func (s *WalletService) CallbackToken() string {
	return s.callbackToken
}

type SaldoDTO struct {
	Total  float64 `json:"total_saldo"`
	Topup  float64 `json:"saldo_topup"`
	Redeem float64 `json:"saldo_redeem"`
	EvPoin int     `json:"ev_poin"`
}

type TransactionDTO struct {
	ID          string    `json:"id"`
	Type        string    `json:"type"`
	Amount      float64   `json:"amount"`
	Description string    `json:"description"`
	ReferenceID *string   `json:"reference_id,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

type VoucherDTO struct {
	ID          string     `json:"id"`
	Code        string     `json:"kode_voucher"`
	Amount      float64    `json:"nilai"`
	Description string     `json:"deskripsi"`
	ExpiresAt   *time.Time `json:"tanggal_kadaluarsa,omitempty"`
}

type PaymentStatusDTO struct {
	OrderID     string     `json:"orderId"`
	Status      string     `json:"status"`
	Amount      float64    `json:"amount"`
	SnapToken   string     `json:"snapToken,omitempty"`
	RedirectURL string     `json:"redirectUrl,omitempty"`
	SettledAt   *time.Time `json:"settledAt,omitempty"`
}

func (s *WalletService) GetSaldo(ctx context.Context, userID string) (SaldoDTO, error) {
	tot, topup, redeem, poin, err := s.repo.GetSaldo(ctx, userID)
	if err != nil {
		return SaldoDTO{}, err
	}
	return SaldoDTO{Total: tot, Topup: topup, Redeem: redeem, EvPoin: poin}, nil
}

func (s *WalletService) GetTransactions(ctx context.Context, userID string, limit int) ([]TransactionDTO, error) {
	rows, err := s.repo.ListTransactions(ctx, userID, limit)
	if err != nil {
		return nil, err
	}
	result := make([]TransactionDTO, 0, len(rows))
	for _, row := range rows {
		var desc string
		if row.Description.Valid {
			desc = row.Description.String
		}
		var ref *string
		if row.ReferenceID.Valid {
			val := row.ReferenceID.String
			ref = &val
		}
		result = append(result, TransactionDTO{
			ID:          row.ID,
			Type:        row.Type,
			Amount:      row.Amount,
			Description: desc,
			ReferenceID: ref,
			CreatedAt:   row.CreatedAt,
		})
	}
	return result, nil
}

func (s *WalletService) ListAvailableVouchers(ctx context.Context, userID string, limit int) ([]VoucherDTO, error) {
	rows, err := s.repo.ListAvailableVouchers(ctx, userID, limit)
	if err != nil {
		return nil, err
	}
	result := make([]VoucherDTO, 0, len(rows))
	for _, row := range rows {
		var desc string
		if row.Description.Valid {
			desc = row.Description.String
		}
		var exp *time.Time
		if row.ExpiresAt.Valid {
			val := row.ExpiresAt.Time
			exp = &val
		}
		result = append(result, VoucherDTO{
			ID:          row.ID,
			Code:        row.Code,
			Amount:      row.Amount,
			Description: desc,
			ExpiresAt:   exp,
		})
	}
	return result, nil
}

func (s *WalletService) GetPaymentStatus(ctx context.Context, orderID string) (PaymentStatusDTO, error) {
	rec, err := s.repo.GetPaymentOrder(ctx, orderID)
	if err != nil {
		var notFound repositories.ErrNotFound
		if errors.As(err, &notFound) {
			return PaymentStatusDTO{}, ErrNotFoundResource{Msg: notFound.Message}
		}
		return PaymentStatusDTO{}, err
	}
	var settledAt *time.Time
	if rec.SettledAt.Valid {
		t := rec.SettledAt.Time
		settledAt = &t
	}
	return PaymentStatusDTO{
		OrderID:     rec.OrderID,
		Status:      rec.Status,
		Amount:      rec.GrossAmount,
		SnapToken:   rec.SnapToken,
		RedirectURL: rec.RedirectURL,
		SettledAt:   settledAt,
	}, nil
}

func (s *WalletService) HandleSnapNotification(ctx context.Context, payload NotificationPayload) error {
	if s.midtransServerKey == "" {
		return fmt.Errorf("midtrans server key belum dikonfigurasi")
	}
	if !VerifyNotificationSignature(s.midtransServerKey, payload) {
		return ErrBadRequest{Err: errors.New("signature midtrans tidak valid")}
	}

	order, err := s.repo.GetPaymentOrder(ctx, payload.OrderID)
	if err != nil {
		var notFound repositories.ErrNotFound
		if errors.As(err, &notFound) {
			return ErrNotFoundResource{Msg: notFound.Message}
		}
		return err
	}

	status := strings.ToLower(payload.TransactionStatus)
	fraud := strings.ToLower(payload.FraudStatus)
	newStatus := strings.ToUpper(payload.TransactionStatus)
	applyBalance := false

	switch status {
	case "capture":
		if fraud == "accept" {
			newStatus = "SETTLEMENT"
			applyBalance = true
		} else {
			newStatus = "PENDING"
		}
	case "settlement":
		newStatus = "SETTLEMENT"
		applyBalance = true
	case "cancel":
		newStatus = "CANCELLED"
	case "expire":
		newStatus = "EXPIRED"
	case "deny":
		newStatus = "DENY"
	}

	rawBytes, _ := json.Marshal(payload)
	var settledAt *time.Time
	if payload.SettlementTime != "" {
		if t, err := time.Parse(time.RFC3339, payload.SettlementTime); err == nil {
			settledAt = &t
		} else if t2, err2 := time.Parse("2006-01-02 15:04:05", payload.SettlementTime); err2 == nil {
			settledAt = &t2
		}
	}

	var midtransID *string
	if payload.TransactionID != "" {
		midtransID = &payload.TransactionID
	}

	if newStatus == "SETTLEMENT" && !order.BalanceApplied && applyBalance {
		tx, err := s.repo.BeginTx(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback()

		applied := true
		if err := s.repo.UpdatePaymentOrderStatusTx(ctx, tx, repositories.UpdatePaymentOrderStatusParams{
			OrderID:            order.OrderID,
			Status:             newStatus,
			MidtransTrxID:      midtransID,
			RawNotification:    rawBytes,
			SettledAt:          settledAt,
			BalanceAlreadyUsed: &applied,
		}); err != nil {
			return err
		}

		ref := payload.TransactionID
		if ref == "" {
			ref = order.OrderID
		}
		if err := s.repo.CreateTransaction(ctx, tx, repositories.CreateTransactionParams{
			UserID:        order.UserID,
			TipeTransaksi: "TOP_UP",
			Jumlah:        order.GrossAmount,
			Deskripsi:     "Top up via Midtrans",
			ReferensiID:   &ref,
			CreatedAt:     s.now(),
		}); err != nil {
			return err
		}

		if err := s.repo.AddSaldo(ctx, tx, repositories.AddSaldoParams{
			UserID:      order.UserID,
			DeltaTotal:  order.GrossAmount,
			DeltaTopup:  order.GrossAmount,
			DeltaRedeem: 0,
			UpdatedAt:   s.now(),
		}); err != nil {
			return err
		}

		return tx.Commit()
	}

	return s.repo.UpdatePaymentOrderStatus(ctx, repositories.UpdatePaymentOrderStatusParams{
		OrderID:         order.OrderID,
		Status:          newStatus,
		MidtransTrxID:   midtransID,
		RawNotification: rawBytes,
		SettledAt:       settledAt,
	})
}

// ===== Klaim Voucher =====

type KlaimVoucherInput struct {
	UserID      string `json:"userId"      validate:"required,uuid4"`
	KodeVoucher string `json:"kodeVoucher" validate:"required,min=6"`
}

type ErrBadRequest struct{ Err error }

func (e ErrBadRequest) Error() string { return e.Err.Error() }

type ErrConflict struct{ Msg string }

func (e ErrConflict) Error() string { return e.Msg }

type ErrInsufficientBalance struct{ Msg string }

func (e ErrInsufficientBalance) Error() string { return e.Msg }

type ErrNotFoundResource struct{ Msg string }

func (e ErrNotFoundResource) Error() string { return e.Msg }

func (s *WalletService) KlaimVoucher(ctx context.Context, in KlaimVoucherInput) error {
	if err := s.validate.Struct(in); err != nil {
		return ErrBadRequest{Err: err}
	}

	// 1) Ambil & validasi voucher
	vID, nilai, aktif, exp, err := s.repo.GetVoucherByCode(ctx, in.KodeVoucher)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrBadRequest{Err: errors.New("voucher tidak ditemukan")}
		}
		return err
	}
	now := s.now()
	if !aktif {
		return ErrConflict{Msg: "voucher non-aktif"}
	}
	if exp.Valid && now.After(exp.Time) {
		return ErrConflict{Msg: "voucher kadaluarsa"}
	}

	// 2) Transaksi (klaim + transaksi + update saldo)
	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	inserted, err := s.repo.CreateVoucherClaim(ctx, tx, in.UserID, vID, now)
	if err != nil {
		return err
	}
	if !inserted {
		return ErrConflict{Msg: "voucher sudah diklaim"}
	}

	ref := vID
	if err := s.repo.CreateTransaction(ctx, tx, repositories.CreateTransactionParams{
		UserID:        in.UserID,
		TipeTransaksi: "KLAIM_VOUCHER",
		Jumlah:        nilai,
		Deskripsi:     "Klaim voucher " + in.KodeVoucher,
		ReferensiID:   &ref,
		CreatedAt:     now,
	}); err != nil {
		return err
	}

	// Klaim voucher menambah saldo redeem dan total saldo.
	if err := s.repo.AddSaldo(ctx, tx, repositories.AddSaldoParams{
		UserID:      in.UserID,
		DeltaTotal:  nilai,
		DeltaTopup:  0,
		DeltaRedeem: nilai,
		UpdatedAt:   now,
	}); err != nil {
		return err
	}

	return tx.Commit()
}

// ===== Top Up =====

type TopUpInput struct {
	UserID string  `json:"userId" validate:"required,uuid4"`
	Jumlah float64 `json:"jumlah" validate:"required,gt=0"`
}

type TopUpResult struct {
	OrderID     string `json:"orderId"`
	SnapToken   string `json:"snapToken"`
	RedirectURL string `json:"redirectUrl"`
	Status      string `json:"status"`
}

func (s *WalletService) TopUp(ctx context.Context, in TopUpInput) (TopUpResult, error) {
	if err := s.validate.Struct(in); err != nil {
		return TopUpResult{}, ErrBadRequest{Err: err}
	}

	if s.snapClient == nil {
		return TopUpResult{}, fmt.Errorf("midtrans snap client belum dikonfigurasi")
	}

	user, err := s.repo.GetUserProfile(ctx, in.UserID)
	if err != nil {
		return TopUpResult{}, err
	}

	orderID := fmt.Sprintf("TOPUP-%s", strings.ReplaceAll(uuid.NewString(), "-", ""))

	req := SnapRequest{
		TransactionDetails: SnapTransactionDetails{
			OrderID:     orderID,
			GrossAmount: in.Jumlah,
		},
	}
	if user != nil {
		req.CustomerDetails = &SnapCustomerDetails{
			FirstName: user.Name,
			Email:     user.Email,
			Phone:     user.Phone,
		}
	}

	res, err := s.snapClient.CreateTransaction(ctx, req)
	if err != nil {
		return TopUpResult{}, err
	}

	if err := s.repo.CreatePaymentOrder(ctx, repositories.CreatePaymentOrderParams{
		UserID:      in.UserID,
		OrderID:     orderID,
		GrossAmount: in.Jumlah,
		SnapToken:   res.Token,
		RedirectURL: res.RedirectURL,
	}); err != nil {
		return TopUpResult{}, err
	}

	return TopUpResult{
		OrderID:     orderID,
		SnapToken:   res.Token,
		RedirectURL: res.RedirectURL,
		Status:      "PENDING",
	}, nil
}

// ===== Withdraw =====

type WithdrawInput struct {
	UserID         string  `json:"userId"`
	Jumlah         float64 `json:"jumlah"`
	BalanceType    string  `json:"balanceType"`
	BalanceTypeAlt string  `json:"balance_type"`
	Source         string  `json:"source"`
	BankCode       string  `json:"bankCode"`
	BankName       string  `json:"bankName"`
	AccountNumber  string  `json:"accountNumber"`
	AccountHolderName  string  `json:"accountHolderName"`
	Email          string  `json:"email"`
	Phone          string  `json:"phone"`
	Notes          string  `json:"notes"`
}

type WithdrawResult struct {
	OrderID string `json:"orderId"`
	Status  string `json:"status"`
}

func (s *WalletService) Withdraw(ctx context.Context, in WithdrawInput) (WithdrawResult, error) {
	if err := s.validate.Var(in.UserID, "required,uuid4"); err != nil {
		return WithdrawResult{}, ErrBadRequest{Err: err}
	}
	if err := s.validate.Var(in.Jumlah, "required,gt=0"); err != nil {
		return WithdrawResult{}, ErrBadRequest{Err: err}
	}
	if strings.TrimSpace(in.BankCode) == "" || strings.TrimSpace(in.AccountNumber) == "" || strings.TrimSpace(in.AccountHolderName) == "" {
		return WithdrawResult{}, ErrBadRequest{Err: errors.New("data bank wajib diisi")}
	}

	balanceType := firstNonEmpty(
		in.BalanceType,
		in.BalanceTypeAlt,
		in.Source,
	)
	balanceType = strings.ToLower(strings.TrimSpace(balanceType))
	if balanceType == "" {
		return WithdrawResult{}, ErrBadRequest{Err: errors.New("balanceType wajib diisi")}
	}

	var target string
	switch balanceType {
	case "topup", "saldo_topup", "pendapatan", "deposit", "saldo_deposit":
		target = "topup"
	case "redeem", "saldo_redeem", "refund", "saldo_refund", "ev", "ev_poin":
		target = "redeem"
	default:
		return WithdrawResult{}, ErrBadRequest{Err: errors.New("balanceType tidak valid")}
	}

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return WithdrawResult{}, err
	}
	defer tx.Rollback()

	total, topupBalance, redeemBalance, _, err := s.repo.GetSaldoForUpdate(ctx, tx, in.UserID)
	if err != nil {
		return WithdrawResult{}, err
	}

	amount := in.Jumlah

	switch target {
	case "topup":
		if topupBalance < amount {
			return WithdrawResult{}, ErrInsufficientBalance{Msg: "saldo top up tidak mencukupi"}
		}
	case "redeem":
		if redeemBalance < amount {
			return WithdrawResult{}, ErrInsufficientBalance{Msg: "saldo redeem tidak mencukupi"}
		}
	}

	if total < amount {
		return WithdrawResult{}, ErrInsufficientBalance{Msg: "total saldo tidak mencukupi"}
	}

	now := s.now()

	var (
		deltaTopup  float64
		deltaRedeem float64
		txnType     string
		txnDesc     string
	)
	if target == "topup" {
		deltaTopup = -amount
		txnType = "TARIK_SALDO_PENDAPATAN"
		txnDesc = "Tarik saldo top up"
	}
	if target == "redeem" {
		deltaRedeem = -amount
		txnType = "TARIK_SALDO_REFUND"
		txnDesc = "Tarik saldo redeem"
	}

	payoutID := fmt.Sprintf("WD-%s", strings.ReplaceAll(uuid.NewString(), "-", ""))

	rawReq := make(map[string]any)
	rawReq["bankCode"] = in.BankCode
	rawReq["bankName"] = in.BankName
	rawReq["accountNumber"] = in.AccountNumber
	rawReq["accountHolderName"] = in.AccountHolderName
	rawReq["notes"] = in.Notes
	rawBytes, _ := json.Marshal(rawReq)

	payout, err := s.repo.CreatePayoutRequest(ctx, repositories.CreatePayoutRequestParams{
		UserID:            in.UserID,
		Amount:            amount,
		BankCode:          in.BankCode,
		BankName:          in.BankName,
		AccountNumber:     in.AccountNumber,
		AccountHolderName: in.AccountHolderName,
		Status:            "PENDING",
		RawResponse:       rawBytes,
		RequestedAt:       now,
	})
	if err != nil {
		return WithdrawResult{}, err
	}

	if err := s.repo.CreateTransaction(ctx, tx, repositories.CreateTransactionParams{
		UserID:        in.UserID,
		TipeTransaksi: txnType,
		Jumlah:        amount,
		Deskripsi:     txnDesc,
		ReferensiID:   nil,
		CreatedAt:     now,
	}); err != nil {
		return WithdrawResult{}, err
	}

	if err := s.repo.AddSaldo(ctx, tx, repositories.AddSaldoParams{
		UserID:      in.UserID,
		DeltaTotal:  -amount,
		DeltaTopup:  deltaTopup,
		DeltaRedeem: deltaRedeem,
		UpdatedAt:   now,
	}); err != nil {
		return WithdrawResult{}, err
	}

	if err := tx.Commit(); err != nil {
		return WithdrawResult{}, err
	}

	var payoutStatus = "PENDING"
	if s.irisClient != nil && s.irisClient.ClientKey != "" && s.irisClient.ClientSecret != "" {
		irisReq := IrisPayoutRequest{
			Payouts: []struct {
				Amount             string `json:"amount"`
				BeneficiaryName    string `json:"beneficiary_name"`
				BeneficiaryAccount string `json:"beneficiary_account"`
				BeneficiaryBank    string `json:"beneficiary_bank"`
				BeneficiaryEmail   string `json:"beneficiary_email,omitempty"`
				Notes              string `json:"notes,omitempty"`
				PartnerTrxID       string `json:"partner_trx_id"`
			}{
				{
					Amount:             fmt.Sprintf("%.0f", amount),
					BeneficiaryName:    in.AccountHolderName,
					BeneficiaryAccount: in.AccountNumber,
					BeneficiaryBank:    in.BankCode,
					BeneficiaryEmail:   in.Email,
					Notes:              in.Notes,
					PartnerTrxID:       payoutID,
				},
			},
		}
		irisRes, err := s.irisClient.CreatePayout(ctx, irisReq)
		if err == nil {
			resBytes, _ := json.Marshal(irisRes)
			_ = s.repo.UpdatePayoutRequestStatus(ctx, repositories.UpdatePayoutRequestStatusParams{
				ID:               payout.ID,
				Status:           strings.ToUpper(irisRes.Status),
				MidtransPayoutID: &irisRes.PayoutID,
				RawResponse:      resBytes,
			})
			payoutStatus = strings.ToUpper(irisRes.Status)
		}
	}

	return WithdrawResult{
		OrderID: payout.ID,
		Status:  payoutStatus,
	}, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
