/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package contribution

import (
	"errors"
	"time"
)

var (
	ErrNotFound    = errors.New("iuran tidak ditemukan")
	ErrAlreadyPaid = errors.New("iuran sudah dibayar")
)

// Contribution adalah tagihan iuran seorang anggota pada satu putaran.
// Semua nominal dalam rupiah utuh.
type Contribution struct {
	ID          string     `json:"id"`
	RoundID     string     `json:"round_id"`
	RoundNo     int        `json:"round_no"`
	DueDate     string     `json:"due_date"` // YYYY-MM-DD
	MemberID    string     `json:"member_id"`
	UserName    string     `json:"user_name"`
	Amount      int64      `json:"amount"`
	PlatformFee int64      `json:"platform_fee"`
	LateFee     int64      `json:"late_fee"`
	Status      string     `json:"status"` // unpaid | paid
	PaidOnTime  *bool      `json:"paid_on_time"`
	PaidAt      *time.Time `json:"paid_at"`
	// Overdue: belum dibayar dan sudah lewat jatuh tempo (hari ini menurut WIB).
	Overdue bool `json:"overdue"`
}

// Mine adalah iuran milik pengguna, lengkap dengan grupnya (untuk dashboard).
type Mine struct {
	Contribution
	GroupID   string `json:"group_id"`
	GroupName string `json:"group_name"`
}

// Receipt adalah hasil MarkPaid. Semua nominal dalam rupiah utuh.
type Receipt struct {
	ID          string    `json:"contribution_id"`
	GroupID     string    `json:"group_id"`
	RoundID     string    `json:"round_id"`
	UserID      string    `json:"-"`
	Amount      int64     `json:"amount"`       // iuran (masuk ke dana arisan)
	PlatformFee int64     `json:"platform_fee"` // biaya platform
	LateFee     int64     `json:"late_fee"`     // denda keterlambatan
	Total       int64     `json:"total"`        // amount + platform_fee + late_fee
	PaidAt      time.Time `json:"paid_at"`
	PaidOnTime  bool      `json:"paid_on_time"`
}

// Quote adalah rincian yang harus dibayar untuk sebuah iuran pada saat ini.
type Quote struct {
	ContributionID string `json:"contribution_id"`
	GroupID        string `json:"group_id"`
	RoundID        string `json:"round_id"`
	DueDate        string `json:"due_date"`
	Status         string `json:"status"`       // unpaid | paid
	RoundStatus    string `json:"round_status"` // upcoming | collecting | paid_out
	Amount         int64  `json:"amount"`
	PlatformFee    int64  `json:"platform_fee"`
	LateFee        int64  `json:"late_fee"`
	Total          int64  `json:"total"`
	Late           bool   `json:"late"`
	Payable        bool   `json:"payable"`
	Reason         string `json:"reason,omitempty"` // mengapa belum bisa dibayar
}
