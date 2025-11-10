package models

// SaldoResponse adalah struct yang kita kirimkan sebagai JSON untuk endpoint /saldo
type SaldoResponse struct {
	TotalSaldo      float64 `json:"total_saldo"`
	SaldoPendapatan float64 `json:"saldo_pendapatan"`
	SaldoRefund     float64 `json:"saldo_refund"`
	EvPoin          int     `json:"ev_poin"`
}

// KlaimRequest adalah struct untuk membaca body JSON sa// models/models.go
type KlaimRequest struct {
	UserID      string `json:"userId"      validate:"required"`
	KodeVoucher string `json:"kodeVoucher" validate:"required,min=6"`
}

// TarikSaldoRequest adalah struct untuk membaca body JSON saat tarik saldo
type TarikSaldoRequest struct {
	UserID    string  `json:"userId"    validate:"required"`
	Jumlah    float64 `json:"jumlah"    validate:"required,gt=0"`
	TipeSaldo string  `json:"tipeSaldo" validate:"required,oneof=Pendapatan Refund"`
}
