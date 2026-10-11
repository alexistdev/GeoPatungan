/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package round

import "errors"

var (
	ErrNotFound             = errors.New("putaran tidak ditemukan")
	ErrRoundNotCollecting   = errors.New("putaran tidak sedang menerima iuran")
	ErrContributionsPending = errors.New("masih ada iuran yang belum dibayar")
	ErrTurnOrderMissing     = errors.New("urutan giliran belum ditetapkan untuk semua anggota")
	ErrNoMembers            = errors.New("grup tidak punya anggota aktif")
)

// Round adalah satu putaran arisan: semua anggota membayar iuran, lalu satu
// anggota (penerima) menerima seluruh dana yang terkumpul.
type Round struct {
	ID                string `json:"id"`
	GroupID           string `json:"group_id"`
	RoundNo           int    `json:"round_no"`
	DueDate           string `json:"due_date"` // YYYY-MM-DD
	RecipientMemberID string `json:"recipient_member_id"`
	RecipientName     string `json:"recipient_name"`
	Status            string `json:"status"` // upcoming | collecting | paid_out
	PaidCount         int    `json:"paid_count"`
	MemberCount       int    `json:"member_count"`
	PotAmount         int64  `json:"pot_amount"` // total iuran putaran ini (rupiah)
}
