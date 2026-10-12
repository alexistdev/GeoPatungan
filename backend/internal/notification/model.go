/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

// Package notification menyimpan pemberitahuan dalam aplikasi untuk pengguna.
//
// Pemberitahuan ditulis di TRANSAKSI YANG SAMA dengan kejadian yang diberitahukannya (Send menerima
// pgx.Tx), sehingga tidak pernah ada pemberitahuan untuk kejadian yang dibatalkan, dan tidak ada
// kejadian penting yang lolos tanpa pemberitahuan. Isinya tidak boleh memuat rahasia atau data
// pribadi pihak lain.
package notification

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Jenis pemberitahuan.
const (
	TypeTopupApproved      = "topup.approved"
	TypeTopupRejected      = "topup.rejected"
	TypeWithdrawalDone     = "withdrawal.completed"
	TypeWithdrawalRejected = "withdrawal.rejected"
	TypeKYCVerified        = "kyc.verified"
	TypeKYCRejected        = "kyc.rejected"
	TypeMemberApproved     = "group.member_approved"
	TypeMemberRejected     = "group.member_rejected"
	TypeGroupStarted       = "group.started"
	TypePayoutReceived     = "round.payout_received"
	TypeReferralCommission = "referral.commission"
)

const (
	MaxTitle = 120
	MaxBody  = 500
)

var (
	ErrNotFound = errors.New("pemberitahuan tidak ditemukan")
	ErrInvalid  = errors.New("pemberitahuan tidak valid")
)

// Notice adalah pemberitahuan yang akan dikirim.
type Notice struct {
	UserID string
	Type   string
	Title  string
	Body   string
	Data   map[string]any // referensi untuk tautan di aplikasi (mis. group_id); bukan data pribadi
}

// Validate memastikan isi pemberitahuan layak simpan (lebih awal dan lebih jelas daripada CHECK DB).
func (n Notice) Validate() error {
	switch {
	case n.UserID == "":
		return fmt.Errorf("%w: penerima kosong", ErrInvalid)
	case strings.TrimSpace(n.Type) == "":
		return fmt.Errorf("%w: jenis kosong", ErrInvalid)
	case strings.TrimSpace(n.Title) == "" || len([]rune(n.Title)) > MaxTitle:
		return fmt.Errorf("%w: judul harus 1-%d karakter", ErrInvalid, MaxTitle)
	case strings.TrimSpace(n.Body) == "" || len([]rune(n.Body)) > MaxBody:
		return fmt.Errorf("%w: isi harus 1-%d karakter", ErrInvalid, MaxBody)
	}
	return nil
}

// Item adalah satu pemberitahuan yang dilihat pengguna.
type Item struct {
	ID        int64          `json:"id"`
	Type      string         `json:"type"`
	Title     string         `json:"title"`
	Body      string         `json:"body"`
	Data      map[string]any `json:"data"`
	Read      bool           `json:"read"`
	CreatedAt time.Time      `json:"created_at"`
}

// Page adalah satu halaman daftar; NextBefore terisi bila masih ada halaman berikutnya.
type Page struct {
	Notifications []Item `json:"notifications"`
	NextBefore    *int64 `json:"next_before"`
	UnreadCount   int    `json:"unread_count"`
}

// Rupiah memformat jumlah: 1234567 -> "Rp1.234.567".
func Rupiah(n int64) string {
	u := uint64(n)
	if n < 0 {
		u = -u // aritmetika dua-komplemen: benar juga untuk MinInt64
	}
	digits := strconv.FormatUint(u, 10)
	var b strings.Builder
	for i, r := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(r)
	}
	if n < 0 {
		return "-Rp" + b.String()
	}
	return "Rp" + b.String()
}

// WithNote menambahkan catatan admin ke kalimat dasar dan memotong hasilnya agar muat di MaxBody.
func WithNote(base, note string) string {
	note = strings.TrimSpace(note)
	if note != "" {
		base += " Catatan: " + note
	}
	r := []rune(base)
	if len(r) > MaxBody {
		return string(r[:MaxBody-1]) + "…"
	}
	return base
}
