/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package contribution

import (
	"testing"
	"time"
)

func TestEvaluate(t *testing.T) {
	const due = "2026-12-01"
	const fee = 5_000

	tests := []struct {
		name       string
		paidAt     time.Time
		wantOnTime bool
	}{
		{"seminggu sebelum", time.Date(2026, 11, 24, 10, 0, 0, 0, wib), true},
		{"pagi hari jatuh tempo", time.Date(2026, 12, 1, 0, 0, 0, 0, wib), true},
		{"detik terakhir hari jatuh tempo (WIB)", time.Date(2026, 12, 1, 23, 59, 59, 0, wib), true},
		{"detik pertama hari berikutnya (WIB)", time.Date(2026, 12, 2, 0, 0, 0, 0, wib), false},
		{"seminggu terlambat", time.Date(2026, 12, 8, 12, 0, 0, 0, wib), false},
		// 17:30 UTC tanggal 1 = 00:30 WIB tanggal 2 -> terlambat menurut WIB
		{"UTC masih tanggal 1 tetapi WIB sudah tanggal 2", time.Date(2026, 12, 1, 17, 30, 0, 0, time.UTC), false},
		// 16:59 UTC tanggal 1 = 23:59 WIB tanggal 1 -> masih tepat waktu
		{"UTC 16:59 = WIB 23:59", time.Date(2026, 12, 1, 16, 59, 0, 0, time.UTC), true},
		// zona waktu lain tetap dinilai menurut WIB
		{"zona UTC-8 sore tanggal 1 (= tanggal 2 WIB)", time.Date(2026, 12, 1, 10, 0, 0, 0, time.FixedZone("PST", -8*3600)), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			onTime, got := evaluate(due, tc.paidAt, fee)
			if onTime != tc.wantOnTime {
				t.Errorf("onTime = %v, want %v", onTime, tc.wantOnTime)
			}
			wantFee := int64(0)
			if !tc.wantOnTime {
				wantFee = fee
			}
			if got != wantFee {
				t.Errorf("denda = %d, want %d", got, wantFee)
			}
		})
	}
}

func TestEvaluateWithoutLateFee(t *testing.T) {
	onTime, fee := evaluate("2026-12-01", time.Date(2027, 1, 1, 0, 0, 0, 0, wib), 0)
	if onTime || fee != 0 {
		t.Errorf("terlambat tanpa denda grup: onTime=%v fee=%d, want false dan 0", onTime, fee)
	}
}

func TestWIBOffset(t *testing.T) {
	_, off := time.Date(2026, 1, 1, 0, 0, 0, 0, wib).Zone()
	if off != 7*3600 {
		t.Errorf("offset WIB = %d detik, want 25200", off)
	}
}
