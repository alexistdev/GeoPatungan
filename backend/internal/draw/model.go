/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package draw

import (
	"errors"
	"time"
)

var (
	ErrNoDraw           = errors.New("kocokan belum dibuat untuk grup ini")
	ErrAlreadyCommitted = errors.New("kocokan sudah dibuat untuk grup ini")
	ErrAlreadyExecuted  = errors.New("kocokan sudah dijalankan")
	ErrGroupNotFull     = errors.New("slot grup belum terisi penuh")
	ErrWrongMethod      = errors.New("metode kocokan grup tidak sesuai dengan endpoint ini")
	ErrMembersChanged   = errors.New("daftar anggota berubah, ulangi permintaan")
	ErrGroupStarted     = errors.New("grup sudah dimulai atau ditutup")
)

// Draw adalah satu kocokan untuk satu grup. Untuk algoritma weighted_es_v1:
// commit (hash seed) diumumkan lebih dulu, seed baru dibuka setelah dijalankan.
type Draw struct {
	ID          string     `json:"id"`
	GroupID     string     `json:"group_id"`
	Algorithm   string     `json:"algorithm"`
	SeedCommit  string     `json:"seed_commit,omitempty"`
	SeedReveal  string     `json:"seed_reveal,omitempty"` // terisi hanya setelah dijalankan
	CommittedAt time.Time  `json:"committed_at"`
	ExecutedAt  *time.Time `json:"executed_at"`
	Entries     []Entry    `json:"entries"`

	seed string // hex; tidak pernah diserialisasi ke JSON
}

type Entry struct {
	MemberID string   `json:"member_id"`
	UserName string   `json:"user_name"`
	Weight   float64  `json:"weight"`
	Score    *float64 `json:"score,omitempty"`
	Order    *int     `json:"turn_order"`
}

// WeightForRank memetakan level reputasi ke bobot kocokan. Rasio dibatasi 3:1
// supaya anggota baru tetap punya peluang yang berarti.
func WeightForRank(rank int) float64 {
	switch {
	case rank <= 1:
		return 1
	case rank == 2:
		return 2
	default:
		return 3
	}
}
