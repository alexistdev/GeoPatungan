/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

// Package topup mengelola pengisian saldo lewat transfer bank manual: pengguna meminta,
// mentransfer, dan mengonfirmasi; admin memverifikasi lalu saldo dikreditkan lewat wallet.Post.
package topup

import (
	"errors"
	"time"

	"github.com/alexistdev/geopatungan/internal/settings"
)

var (
	ErrNotFound      = errors.New("permintaan top-up tidak ditemukan")
	ErrInvalidState  = errors.New("status permintaan top-up tidak mengizinkan tindakan ini")
	ErrTooManyOpen   = errors.New("terlalu banyak permintaan top-up yang belum selesai")
	ErrSelfReview    = errors.New("admin tidak boleh memproses permintaan miliknya sendiri")
	ErrNoBankAccount = errors.New("rekening tujuan transfer belum diatur oleh admin")
	ErrBusy          = errors.New("sistem sedang sibuk, coba lagi")
)

// Status permintaan top-up.
const (
	StatusPending   = "pending"
	StatusSubmitted = "submitted"
	StatusApproved  = "approved"
	StatusRejected  = "rejected"
	StatusCancelled = "cancelled"
	StatusExpired   = "expired"
)

// Topup adalah satu permintaan pengisian saldo. Nominal dalam rupiah utuh.
type Topup struct {
	ID             string     `json:"id"`
	UserID         string     `json:"user_id"`
	Amount         int64      `json:"amount"`          // nominal yang diminta pengguna
	UniqueCode     int        `json:"unique_code"`     // 1..999
	TransferAmount int64      `json:"transfer_amount"` // Amount + UniqueCode: yang harus ditransfer
	Status         string     `json:"status"`
	SenderName     string     `json:"sender_name"`
	SenderBank     string     `json:"sender_bank"`
	Note           string     `json:"note"`
	SubmittedAt    *time.Time `json:"submitted_at"`
	ExpiresAt      time.Time  `json:"expires_at"`
	ReviewedAt     *time.Time `json:"reviewed_at"`
	ReviewNote     string     `json:"review_note"`
	ReceivedAmount *int64     `json:"received_amount"` // yang dikreditkan, terisi saat approved
	CreatedAt      time.Time  `json:"created_at"`
}

// AdminTopup menambahkan identitas pemohon dan peninjau untuk tampilan admin.
type AdminTopup struct {
	Topup
	UserName   string  `json:"user_name"`
	UserEmail  string  `json:"user_email"`
	ReviewedBy *string `json:"reviewed_by"`
}

// Detail adalah permintaan beserta rekening tujuan (hanya selama masih terbuka).
type Detail struct {
	Topup        *Topup                 `json:"topup"`
	BankAccounts []settings.BankAccount `json:"bank_accounts"`
}

// Open menyatakan apakah permintaan masih menunggu transfer atau verifikasi.
func (t *Topup) Open() bool {
	return t.Status == StatusPending || t.Status == StatusSubmitted
}
