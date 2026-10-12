/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

// Package referral membayarkan komisi referral multi-level (4 level) dari BIAYA PLATFORM.
//
// Prinsip yang membuatnya sehat:
//   - sumber komisi hanya biaya platform atas iuran yang benar-benar dibayar; tidak pernah dana arisan
//     anggota, tidak pernah biaya masuk, dan tidak ada komisi tanpa transaksi nyata;
//   - total bagian semua level tidak boleh melebihi 100% dari biaya (dijaga validasi dan trigger
//     database), jadi platform tidak bisa "defisit" karena komisi;
//   - komisi dibulatkan ke bawah per level; sisanya tetap di platform;
//   - hanya upline yang sudah terverifikasi (KYC) yang menerima; bagian upline lain tetap di platform.
package referral

import (
	"errors"
	"time"
)

var ErrNotFound = errors.New("pengguna tidak ditemukan")

// Rule adalah bagian dari biaya platform untuk upline pada satu level (basis poin: 4000 = 40%).
type Rule struct {
	Level    int `json:"level"`
	ShareBps int `json:"share_bps"`
}

// Beneficiary adalah upline yang berhak menerima komisi atas pembayaran seorang pengguna.
type Beneficiary struct {
	Level    int
	UserID   string
	ShareBps int
}

// Commission adalah komisi yang dibayarkan ke satu upline atas satu iuran.
type Commission struct {
	Level    int
	UserID   string
	ShareBps int
	Amount   int64
}

// Amounts menghitung komisi dari biaya platform sebesar fee. Dibulatkan ke bawah per level; bagian
// nol dibuang; total tidak pernah melebihi fee (pagar tambahan di atas validasi aturan).
func Amounts(plan []Beneficiary, fee int64) []Commission {
	if fee <= 0 {
		return nil
	}
	var out []Commission
	var total int64
	for _, b := range plan {
		amt := fee * int64(b.ShareBps) / 10000
		if amt > fee-total {
			amt = fee - total
		}
		if amt <= 0 {
			continue
		}
		total += amt
		out = append(out, Commission{Level: b.Level, UserID: b.UserID, ShareBps: b.ShareBps, Amount: amt})
	}
	return out
}

// Total menjumlahkan komisi.
func Total(cs []Commission) int64 {
	var t int64
	for _, c := range cs {
		t += c.Amount
	}
	return t
}

// Summary adalah ringkasan referral seorang pengguna.
type Summary struct {
	ReferralCode string        `json:"referral_code"`
	Eligible     bool          `json:"eligible"` // terverifikasi: hanya yang terverifikasi menerima komisi
	Rules        []Rule        `json:"rules"`
	Downline     DownlineStats `json:"downline"`
	Earned       EarnedStats   `json:"earned"`
}

type DownlineStats struct {
	Level1 int `json:"level_1"`
	Level2 int `json:"level_2"`
	Level3 int `json:"level_3"`
	Level4 int `json:"level_4"`
	Total  int `json:"total"`
}

type EarnedStats struct {
	Total   int64            `json:"total"`
	ByLevel map[string]int64 `json:"by_level"`
}

// CommissionEntry adalah satu baris riwayat komisi yang diterima. Nama sumber disamarkan.
type CommissionEntry struct {
	ID         int64     `json:"id"`
	Level      int       `json:"level"`
	ShareBps   int       `json:"share_bps"`
	Amount     int64     `json:"amount"`
	SourceName string    `json:"source_name"` // disamarkan, mis. "B*** S***"
	CreatedAt  time.Time `json:"created_at"`
}

// CommissionPage adalah satu halaman riwayat; NextBefore terisi jika masih ada halaman berikutnya.
type CommissionPage struct {
	Commissions []CommissionEntry `json:"commissions"`
	NextBefore  *int64            `json:"next_before"`
}
