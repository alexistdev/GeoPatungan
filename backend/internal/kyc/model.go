/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

// Package kyc mengelola verifikasi identitas pengguna yang diputuskan admin secara manual.
// Sengaja TIDAK menyimpan nomor identitas atau dokumen; hanya permintaan, keputusan, dan
// catatannya. Status akhir tercermin di users.kyc_status, yang dipakai modul reputasi.
package kyc

import (
	"errors"
	"time"
)

var (
	ErrNotFound        = errors.New("permintaan verifikasi tidak ditemukan")
	ErrAlreadyPending  = errors.New("permintaan verifikasi Anda sedang diproses")
	ErrAlreadyVerified = errors.New("akun Anda sudah terverifikasi")
	ErrInvalidState    = errors.New("permintaan verifikasi sudah diputuskan")
	ErrSelfReview      = errors.New("admin tidak boleh memverifikasi dirinya sendiri")
)

// Status permintaan verifikasi.
const (
	StatusPending  = "pending"
	StatusVerified = "verified"
	StatusRejected = "rejected"
)

type Request struct {
	ID          string     `json:"id"`
	UserID      string     `json:"user_id"`
	Status      string     `json:"status"`
	Note        string     `json:"note"`
	ReviewNote  string     `json:"review_note"`
	SubmittedAt time.Time  `json:"submitted_at"`
	ReviewedAt  *time.Time `json:"reviewed_at"`
}

// AdminRequest menambahkan identitas pemohon dan peninjau untuk tampilan admin.
type AdminRequest struct {
	Request
	UserName   string  `json:"user_name"`
	UserEmail  string  `json:"user_email"`
	ReviewedBy *string `json:"reviewed_by"`
}

// Status menggabungkan status akun dan permintaan terakhir untuk pengguna.
type Status struct {
	KYCStatus string   `json:"kyc_status"` // none | pending | verified | rejected
	Latest    *Request `json:"latest_request"`
}
