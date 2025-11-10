package services

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"hash"
	"net/http"
	"sync"
	"time"

	"crypto/sha512"
)

// SnapClient dipakai untuk membuat transaksi Midtrans Snap.
type SnapClient struct {
	ServerKey string
	BaseURL   string
	Client    *http.Client
}

type SnapRequest struct {
	TransactionDetails SnapTransactionDetails `json:"transaction_details"`
	CustomerDetails    *SnapCustomerDetails   `json:"customer_details,omitempty"`
	ItemDetails        []SnapItemDetail       `json:"item_details,omitempty"`
	EnabledPayments    []string               `json:"enabled_payments,omitempty"`
}

type SnapTransactionDetails struct {
	OrderID     string  `json:"order_id"`
	GrossAmount float64 `json:"gross_amount"`
}

type SnapCustomerDetails struct {
	FirstName string `json:"first_name,omitempty"`
	Email     string `json:"email,omitempty"`
	Phone     string `json:"phone,omitempty"`
}

type SnapItemDetail struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Price    float64 `json:"price"`
	Quantity int     `json:"quantity"`
}

type SnapResponse struct {
	Token       string `json:"token"`
	RedirectURL string `json:"redirect_url"`
}

func NewSnapClient(serverKey, baseURL string) *SnapClient {
	if baseURL == "" {
		baseURL = "https://app.sandbox.midtrans.com/snap/v1/transactions"
	}
	return &SnapClient{
		ServerKey: serverKey,
		BaseURL:   baseURL,
		Client: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

func (c *SnapClient) CreateTransaction(ctx context.Context, req SnapRequest) (SnapResponse, error) {
	if c == nil {
		return SnapResponse{}, fmt.Errorf("snap client is nil")
	}
	body, err := json.Marshal(req)
	if err != nil {
		return SnapResponse{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL, bytes.NewReader(body))
	if err != nil {
		return SnapResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	httpReq.SetBasicAuth(c.ServerKey, "")

	resp, err := c.Client.Do(httpReq)
	if err != nil {
		return SnapResponse{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		var payload map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&payload)
		return SnapResponse{}, fmt.Errorf("midtrans snap error: status=%d response=%v", resp.StatusCode, payload)
	}
	var res SnapResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return SnapResponse{}, err
	}
	return res, nil
}

// NotificationPayload merepresentasikan payload notifikasi Midtrans.
type NotificationPayload struct {
	TransactionStatus string `json:"transaction_status"`
	FraudStatus       string `json:"fraud_status"`
	OrderID           string `json:"order_id"`
	StatusCode        string `json:"status_code"`
	GrossAmount       string `json:"gross_amount"`
	SignatureKey      string `json:"signature_key"`
	PaymentType       string `json:"payment_type"`
	TransactionID     string `json:"transaction_id"`
	SettlementTime    string `json:"settlement_time"`
}

func VerifyNotificationSignature(serverKey string, payload NotificationPayload) bool {
	raw := payload.OrderID + payload.StatusCode + payload.GrossAmount + serverKey
	expected := computeSHA512(raw)
	return expected == payload.SignatureKey
}

func computeSHA512(input string) string {
	h := sha512Pool.Get().(hash.Hash)
	defer func() {
		h.Reset()
		sha512Pool.Put(h)
	}()
	_, _ = h.Write([]byte(input))
	return fmt.Sprintf("%x", h.Sum(nil))
}

var sha512Pool = sync.Pool{
	New: func() any {
		return sha512.New()
	},
}

// Struktur untuk IRIS Payout
type IrisClient struct {
	BaseURL      string
	ClientKey    string
	ClientSecret string
	Client       *http.Client
}

type IrisPayoutRequest struct {
	Payouts []struct {
		Amount             string `json:"amount"`
		BeneficiaryName    string `json:"beneficiary_name"`
		BeneficiaryAccount string `json:"beneficiary_account"`
		BeneficiaryBank    string `json:"beneficiary_bank"`
		BeneficiaryEmail   string `json:"beneficiary_email,omitempty"`
		Notes              string `json:"notes,omitempty"`
		PartnerTrxID       string `json:"partner_trx_id"`
	} `json:"payouts"`
}

type IrisPayoutResponse struct {
	Result   string `json:"result"`
	PayoutID string `json:"payout_id"`
	Status   string `json:"status"`
}

func NewIrisClient(clientKey, clientSecret, baseURL string) *IrisClient {
	if baseURL == "" {
		baseURL = "https://app.sandbox.midtrans.com/iris/api/v1/payouts"
	}
	return &IrisClient{
		BaseURL:      baseURL,
		ClientKey:    clientKey,
		ClientSecret: clientSecret,
		Client: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

func (c *IrisClient) CreatePayout(ctx context.Context, req IrisPayoutRequest) (IrisPayoutResponse, error) {
	if c == nil {
		return IrisPayoutResponse{}, fmt.Errorf("iris client is nil")
	}
	body, err := json.Marshal(req)
	if err != nil {
		return IrisPayoutResponse{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL, bytes.NewReader(body))
	if err != nil {
		return IrisPayoutResponse{}, err
	}
	auth := base64.StdEncoding.EncodeToString([]byte(c.ClientKey + ":" + c.ClientSecret))
	httpReq.Header.Set("Authorization", "Basic "+auth)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	resp, err := c.Client.Do(httpReq)
	if err != nil {
		return IrisPayoutResponse{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		var payload map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&payload)
		return IrisPayoutResponse{}, fmt.Errorf("midtrans iris error: status=%d response=%v", resp.StatusCode, payload)
	}

	var res struct {
		Result   string `json:"result"`
		Payouts  []struct {
			ID     string `json:"payout_id"`
			Status string `json:"status"`
		} `json:"payouts"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return IrisPayoutResponse{}, err
	}
	out := IrisPayoutResponse{
		Result: res.Result,
	}
	if len(res.Payouts) > 0 {
		out.PayoutID = res.Payouts[0].ID
		out.Status = res.Payouts[0].Status
	}
	return out, nil
}
