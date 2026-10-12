/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

// Package withdrawal mengelola penarikan saldo ke rekening bank. Saat diminta, dana langsung
// dipindahkan ke dompet "holding" sehingga tidak bisa dipakai dua kali; admin lalu mentransfer
// secara manual dan menandai selesai, atau menolak (dana kembali ke pengguna).
package withdrawal

import (
	"errors"
	"time"
)

var (
	ErrNotFound     = errors.New("permintaan penarikan tidak ditemukan")
	ErrInvalidState = errors.New("status permintaan penarikan tidak mengizinkan tindakan ini")
	ErrTooManyOpen  = errors.New("terlalu banyak permintaan penarikan yang belum selesai")
	ErrSelfReview   = errors.New("admin tidak boleh memproses permintaan miliknya sendiri")
)

// Status permintaan penarikan.
const (
	StatusPending   = "pending"
	StatusCompleted = "completed"
	StatusRejected  = "rejected"
	StatusCancelled = "cancelled"
)

// Withdrawal adalah satu permintaan penarikan. Nominal dalam rupiah utuh.
type Withdrawal struct {
	ID                string     `json:"id"`
	UserID            string     `json:"user_id"`
	Amount            int64      `json:"amount"`
	BankName          string     `json:"bank_name"`
	AccountNumber     string     `json:"account_number"`
	AccountHolder     string     `json:"account_holder"`
	Note              string     `json:"note"`
	Status            string     `json:"status"`
	TransferReference string     `json:"transfer_reference"`
	ReviewNote        string     `json:"review_note"`
	ReviewedAt        *time.Time `json:"reviewed_at"`
	CreatedAt         time.Time  `json:"created_at"`
}

// AdminWithdrawal menambahkan identitas pemohon dan peninjau untuk tampilan admin.
type AdminWithdrawal struct {
	Withdrawal
	UserName   string  `json:"user_name"`
	UserEmail  string  `json:"user_email"`
	ReviewedBy *string `json:"reviewed_by"`
}

// Input adalah data permintaan penarikan dari pengguna.
type Input struct {
	Amount        int64
	BankName      string
	AccountNumber string
	AccountHolder string
	Note          string
}
