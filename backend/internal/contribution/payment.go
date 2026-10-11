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

// MarkPaid menandai iuran lunas di dalam transaksi pemanggil (modul wallet/payment
// memanggilnya bersama pencatatan ledger agar atomik). Baris dikunci; hanya iuran
// pada putaran yang sedang `collecting` dan belum dibayar yang bisa dilunasi.
// Denda keterlambatan grup dikenakan jika dibayar setelah jatuh tempo.
func MarkPaid(ctx context.Context, tx pgx.Tx, id string, paidAt time.Time) (*Receipt, error) {
	var status, roundStatus, due string
	var amount, groupLateFee int64
	err := tx.QueryRow(ctx,
		`SELECT c.status, r.status, to_char(r.due_date, 'YYYY-MM-DD'), c.amount, g.late_fee
		   FROM contributions c
		   JOIN rounds r ON r.id = c.round_id
		   JOIN groups g ON g.id = r.group_id
		  WHERE c.id = $1
		    FOR UPDATE OF c`, id,
	).Scan(&status, &roundStatus, &due, &amount, &groupLateFee)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if status == "paid" {
		return nil, ErrAlreadyPaid
	}
	if roundStatus != "collecting" {
		return nil, round.ErrRoundNotCollecting
	}

	onTime, fee := evaluate(due, paidAt, groupLateFee)
	if _, err := tx.Exec(ctx,
		`UPDATE contributions
		    SET status = 'paid', paid_at = $2, paid_on_time = $3, late_fee = $4
		  WHERE id = $1`, id, paidAt, onTime, fee); err != nil {
		return nil, err
	}
	return &Receipt{ID: id, PaidAt: paidAt, PaidOnTime: onTime, LateFee: fee, Total: amount + fee}, nil
}
