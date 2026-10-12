/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package contribution

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/alexistdev/geopatungan/internal/round"
)

// wib adalah zona waktu bisnis. Indonesia (WIB) tidak mengenal DST, jadi offset tetap
// aman dan tidak butuh database zona waktu.
var wib = time.FixedZone("WIB", 7*60*60)

// evaluate menentukan apakah pembayaran tepat waktu. Tenggat berlaku sampai akhir
// hari jatuh tempo menurut WIB; setelah itu terkena denda keterlambatan.
func evaluate(dueDate string, paidAt time.Time, lateFee int64) (onTime bool, fee int64) {
	if paidAt.In(wib).Format("2006-01-02") <= dueDate {
		return true, 0
	}
	return false, lateFee
}

// PlatformFee menghitung biaya platform atas satu iuran: amount x bps / 10.000, dibulatkan
// ke rupiah terdekat (setengah ke atas). bps 100 = 1%. bps negatif dianggap 0.
func PlatformFee(amount int64, bps int) int64 {
	if bps <= 0 || amount <= 0 {
		return 0
	}
	return (amount*int64(bps) + 5000) / 10000
}

// Querier dipenuhi *pgxpool.Pool maupun pgx.Tx.
type Querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// payable adalah keadaan sebuah iuran yang relevan untuk pembayaran.
type payable struct {
	status, roundStatus, due     string
	amount, platformFee, lateFee int64
	groupLateFee                 int64
	paidOnTime                   *bool
	userID, memberStatus         string
	roundID, groupID             string
}

// load membaca iuran beserta putaran, grup, dan pemiliknya. lock=true mengunci baris iuran.
func load(ctx context.Context, q Querier, id string, lock bool) (*payable, error) {
	sql := `SELECT c.status, r.status, to_char(r.due_date, 'YYYY-MM-DD'), c.amount, c.platform_fee, c.late_fee,
	               g.late_fee, c.paid_on_time, m.user_id::text, m.status, r.id::text, g.id::text
	          FROM contributions c
	          JOIN rounds r ON r.id = c.round_id
	          JOIN groups g ON g.id = r.group_id
	          JOIN group_members m ON m.id = c.member_id
	         WHERE c.id = $1`
	if lock {
		sql += ` FOR UPDATE OF c`
	}
	var p payable
	err := q.QueryRow(ctx, sql, id).Scan(&p.status, &p.roundStatus, &p.due, &p.amount, &p.platformFee, &p.lateFee,
		&p.groupLateFee, &p.paidOnTime, &p.userID, &p.memberStatus, &p.roundID, &p.groupID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// owns memastikan iuran milik anggota aktif userID. Selain itu dianggap tidak ada
// (keberadaan iuran orang lain tidak boleh bocor).
func (p *payable) owns(userID string) bool {
	return p.userID == userID && p.memberStatus == "active"
}

// MarkPaid menandai iuran lunas di dalam transaksi pemanggil (modul payment memanggilnya
// bersama pencatatan ledger agar atomik). Baris iuran dikunci. Syarat:
//
//   - iuran milik anggota aktif payerUserID (selain itu ErrNotFound);
//   - belum dibayar (ErrAlreadyPaid) dan putaran sedang `collecting`.
//
// Denda keterlambatan grup dikenakan jika dibayar setelah jatuh tempo (akhir hari WIB), dan
// biaya platform (feeBps, basis poin) dihitung dari nominal iuran. Keduanya disimpan pada
// iuran. Total yang harus dibayar = amount + biaya platform + denda.
func MarkPaid(ctx context.Context, tx pgx.Tx, id, payerUserID string, paidAt time.Time, feeBps int) (*Receipt, error) {
	p, err := load(ctx, tx, id, true)
	if err != nil {
		return nil, err
	}
	if !p.owns(payerUserID) {
		return nil, ErrNotFound
	}
	if p.status == "paid" {
		return nil, ErrAlreadyPaid
	}
	if p.roundStatus != "collecting" {
		return nil, round.ErrRoundNotCollecting
	}

	onTime, late := evaluate(p.due, paidAt, p.groupLateFee)
	fee := PlatformFee(p.amount, feeBps)
	if _, err := tx.Exec(ctx,
		`UPDATE contributions
		    SET status = 'paid', paid_at = $2, paid_on_time = $3, late_fee = $4, platform_fee = $5
		  WHERE id = $1`, id, paidAt, onTime, late, fee); err != nil {
		return nil, err
	}
	return &Receipt{
		ID: id, GroupID: p.groupID, RoundID: p.roundID, UserID: p.userID,
		Amount: p.amount, PlatformFee: fee, LateFee: late, Total: p.amount + fee + late,
		PaidAt: paidAt, PaidOnTime: onTime,
	}, nil
}

// QuoteFor menghitung rincian yang harus dibayar SEKARANG tanpa mengubah apa pun, supaya
// pengguna melihat total (termasuk biaya dan denda) sebelum membayar. Untuk iuran yang sudah
// lunas, yang ditampilkan adalah nominal yang tercatat saat dibayar.
func QuoteFor(ctx context.Context, q Querier, id, userID string, now time.Time, feeBps int) (*Quote, error) {
	p, err := load(ctx, q, id, false)
	if err != nil {
		return nil, err
	}
	if !p.owns(userID) {
		return nil, ErrNotFound
	}

	qt := &Quote{ContributionID: id, GroupID: p.groupID, RoundID: p.roundID, DueDate: p.due,
		Status: p.status, RoundStatus: p.roundStatus, Amount: p.amount}
	if p.status == "paid" {
		qt.PlatformFee, qt.LateFee = p.platformFee, p.lateFee
		qt.Late = p.paidOnTime != nil && !*p.paidOnTime
		qt.Reason = "sudah dibayar"
	} else {
		onTime, late := evaluate(p.due, now, p.groupLateFee)
		qt.PlatformFee, qt.LateFee, qt.Late = PlatformFee(p.amount, feeBps), late, !onTime
		switch p.roundStatus {
		case "upcoming":
			qt.Reason = "putaran belum dibuka"
		case "paid_out":
			qt.Reason = "putaran sudah ditutup"
		default:
			qt.Payable = true
		}
	}
	qt.Total = qt.Amount + qt.PlatformFee + qt.LateFee
	return qt, nil
}
