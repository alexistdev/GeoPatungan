/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

// Package reputation menghitung level reputasi pengguna dari data nyata dan menyediakan
// kebijakan (hak dan batas) tiap level untuk modul grup dan kocokan.
//
// Level TIDAK disimpan; ia dihitung setiap kali dibutuhkan dari tiga sumber:
//   - KYC: users.kyc_status = 'verified';
//   - arisan tuntas: grup berstatus done yang diikuti sebagai anggota aktif;
//   - keterlambatan: iuran yang dibayar terlambat ditambah iuran yang kini menunggak.
//
// Dengan begitu level tidak mungkin melenceng dari kenyataan, dan perubahan ambang oleh admin
// berlaku seketika.
package reputation

import (
	"errors"
	"fmt"
)

var ErrNotFound = errors.New("pengguna tidak ditemukan")

// Level adalah definisi satu level reputasi: syarat mencapainya dan hak yang diberikannya.
type Level struct {
	Rank int    `json:"rank"`
	Code string `json:"code"`
	Name string `json:"name"`

	// Syarat.
	RequireVerified    bool `json:"require_verified"`
	MinCompletedCycles int  `json:"min_completed_cycles"`
	MaxLateCount       *int `json:"max_late_count"` // nil = tanpa batas

	// Hak.
	DrawWeight     float64 `json:"draw_weight"`
	CanCreateGroup bool    `json:"can_create_group"`
	MaxGroupValue  *int64  `json:"max_group_value"` // nil = tanpa batas
}

// Stats adalah fakta tentang seorang pengguna yang menentukan levelnya.
type Stats struct {
	KYCStatus       string `json:"kyc_status"`
	Verified        bool   `json:"verified"`
	CompletedCycles int    `json:"completed_cycles"` // arisan yang tuntas diikuti
	OnTimePayments  int    `json:"on_time_payments"`
	LatePayments    int    `json:"late_payments"`  // dibayar setelah jatuh tempo
	OverdueUnpaid   int    `json:"overdue_unpaid"` // saat ini menunggak
	LateTotal       int    `json:"late_total"`     // LatePayments + OverdueUnpaid
}

// Requirement adalah satu syarat menuju level berikutnya beserta kemajuannya.
type Requirement struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Met      bool   `json:"met"`
	Current  int    `json:"current"`
	Required int    `json:"required"` // untuk "late": batas maksimal yang diizinkan
}

// NextLevel adalah level di atas level saat ini beserta syarat yang belum/sudah dipenuhi.
type NextLevel struct {
	Level        Level         `json:"level"`
	Requirements []Requirement `json:"requirements"`
}

// Profile adalah gambaran reputasi seorang pengguna.
type Profile struct {
	Level Level      `json:"level"`
	Stats Stats      `json:"stats"`
	Next  *NextLevel `json:"next_level"` // nil jika sudah di level tertinggi
}

// Satisfied menyatakan apakah stats memenuhi syarat level ini.
func (l Level) Satisfied(st Stats) bool {
	if l.RequireVerified && !st.Verified {
		return false
	}
	if st.CompletedCycles < l.MinCompletedCycles {
		return false
	}
	if l.MaxLateCount != nil && st.LateTotal > *l.MaxLateCount {
		return false
	}
	return true
}

// Evaluate mengembalikan level tertinggi yang syaratnya dipenuhi. levels harus terurut menurut
// rank naik dan tidak kosong; level pertama (dasar) selalu menjadi cadangan.
func Evaluate(levels []Level, st Stats) Level {
	best := levels[0]
	for _, l := range levels[1:] {
		if l.Satisfied(st) {
			best = l
		}
	}
	return best
}

// Progress menjabarkan syarat sebuah level dan seberapa jauh stats memenuhinya.
func Progress(l Level, st Stats) []Requirement {
	var reqs []Requirement
	if l.RequireVerified {
		cur := 0
		if st.Verified {
			cur = 1
		}
		reqs = append(reqs, Requirement{Key: "verified", Label: "Akun terverifikasi (KYC)", Met: st.Verified, Current: cur, Required: 1})
	}
	if l.MinCompletedCycles > 0 {
		reqs = append(reqs, Requirement{
			Key: "completed_cycles", Label: fmt.Sprintf("Menuntaskan %d arisan", l.MinCompletedCycles),
			Met: st.CompletedCycles >= l.MinCompletedCycles, Current: st.CompletedCycles, Required: l.MinCompletedCycles,
		})
	}
	if l.MaxLateCount != nil {
		label := "Tanpa keterlambatan"
		if *l.MaxLateCount > 0 {
			label = fmt.Sprintf("Maksimal %d keterlambatan", *l.MaxLateCount)
		}
		reqs = append(reqs, Requirement{Key: "late", Label: label, Met: st.LateTotal <= *l.MaxLateCount, Current: st.LateTotal, Required: *l.MaxLateCount})
	}
	if reqs == nil {
		reqs = []Requirement{}
	}
	return reqs
}

// BuildProfile menyusun profil: level saat ini, stats, dan syarat menuju level berikutnya.
func BuildProfile(levels []Level, st Stats) *Profile {
	cur := Evaluate(levels, st)
	p := &Profile{Level: cur, Stats: st}
	for _, l := range levels {
		if l.Rank == cur.Rank+1 {
			next := l
			p.Next = &NextLevel{Level: next, Requirements: Progress(next, st)}
			break
		}
	}
	return p
}
