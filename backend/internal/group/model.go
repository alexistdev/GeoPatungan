/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package group

import (
	"errors"
	"time"

	"github.com/alexistdev/geopatungan/internal/httpx"
)

var (
	ErrNotFound      = errors.New("data tidak ditemukan")
	ErrDuplicate     = errors.New("data sudah ada")
	ErrAlreadyMember = errors.New("sudah terdaftar di grup ini")
	ErrGroupFull     = errors.New("slot grup sudah penuh")
	ErrGroupNotOpen  = errors.New("grup tidak lagi menerima anggota")
	ErrForbidden     = errors.New("tidak punya akses")
	ErrLevelTooLow   = errors.New("level reputasi belum memenuhi syarat grup")
)

// ValidationError adalah kesalahan input dari pengguna (dipetakan ke HTTP 400).
// Alias dari httpx.ValidationError agar semua modul berbagi satu tipe.
type ValidationError = httpx.ValidationError

// Semua nominal uang dalam rupiah utuh (BIGINT), tanpa pecahan.
type Group struct {
	ID                 string    `json:"id"`
	OwnerID            string    `json:"owner_id"`
	Name               string    `json:"name"`
	ContributionAmount int64     `json:"contribution_amount"`
	PeriodType         string    `json:"period_type"`
	SlotCount          int       `json:"slot_count"`
	StartDate          string    `json:"start_date"` // YYYY-MM-DD
	LateFee            int64     `json:"late_fee"`
	DrawMethod         string    `json:"draw_method"`
	MinLevelRank       int       `json:"min_level_rank"`
	Status             string    `json:"status"`
	InviteCode         string    `json:"invite_code,omitempty"` // hanya untuk pemilik grup
	CreatedAt          time.Time `json:"created_at"`

	ActiveMembers int    `json:"active_members"`
	MyStatus      string `json:"my_status,omitempty"` // status keanggotaan pemanggil
}

type Member struct {
	ID       string    `json:"id"`
	GroupID  string    `json:"group_id"`
	UserID   string    `json:"user_id"`
	UserName string    `json:"user_name"`
	Turn     *int      `json:"turn_order"`
	Status   string    `json:"status"` // pending | active | removed
	JoinedAt time.Time `json:"joined_at"`
}
