/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package settings

import (
	"errors"
	"time"
)

var (
	ErrNotFound  = errors.New("data tidak ditemukan")
	ErrDuplicate = errors.New("rekening dengan bank dan nomor yang sama sudah ada")
)

// Settings adalah pengaturan platform yang diubah admin. Nominal dalam rupiah utuh.
type Settings struct {
	// PlatformFeeBps adalah biaya platform per iuran dalam basis poin (100 = 1%).
	PlatformFeeBps    int   `json:"platform_fee_bps"`
	TopupMinAmount    int64 `json:"topup_min_amount"`
	TopupMaxAmount    int64 `json:"topup_max_amount"`
	WithdrawMinAmount int64 `json:"withdraw_min_amount"`
	// TopupExpiryHours adalah lama permintaan top-up menunggu transfer sebelum kedaluwarsa.
	TopupExpiryHours int `json:"topup_expiry_hours"`
}

// Defaults adalah nilai yang dipakai selama admin belum mengubah pengaturan.
func Defaults() Settings {
	return Settings{
		PlatformFeeBps:    0,
		TopupMinAmount:    10_000,
		TopupMaxAmount:    100_000_000,
		WithdrawMinAmount: 50_000,
		TopupExpiryHours:  24,
	}
}

// BankAccount adalah rekening tujuan transfer top-up.
type BankAccount struct {
	ID            string    `json:"id"`
	BankName      string    `json:"bank_name"`
	AccountNumber string    `json:"account_number"`
	AccountHolder string    `json:"account_holder"`
	Notes         string    `json:"notes"`
	IsActive      bool      `json:"is_active"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// PaymentInfo adalah informasi yang dilihat pengguna untuk melakukan top-up.
type PaymentInfo struct {
	BankAccounts      []BankAccount `json:"bank_accounts"`
	TopupMinAmount    int64         `json:"topup_min_amount"`
	TopupMaxAmount    int64         `json:"topup_max_amount"`
	WithdrawMinAmount int64         `json:"withdraw_min_amount"`
	PlatformFeeBps    int           `json:"platform_fee_bps"`
	TopupExpiryHours  int           `json:"topup_expiry_hours"`
}
