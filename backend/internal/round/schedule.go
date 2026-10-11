/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package round

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// Tanggal jatuh tempo putaran k = start_date + (k-1) periode. Penjumlahan bulan
// dihitung Postgres dari start_date asli, jadi akhir bulan aman:
// 31 Jan + 1 bulan = 28 Feb, tetapi + 2 bulan = 31 Mar (bukan 28 Mar).
const insertRounds = `
INSERT INTO rounds (group_id, round_no, due_date, recipient_member_id, status)
SELECT g.id, m.turn_order,
       CASE g.period_type
            WHEN 'weekly' THEN g.start_date + (m.turn_order - 1) * 7
            ELSE (g.start_date + (m.turn_order - 1) * INTERVAL '1 month')::date
       END,
       m.id,
       CASE WHEN m.turn_order = 1 THEN 'collecting' ELSE 'upcoming' END
  FROM groups g
  JOIN group_members m ON m.group_id = g.id AND m.status = 'active'
 WHERE g.id = $1`

const insertContributions = `
INSERT INTO contributions (round_id, member_id, amount)
SELECT r.id, m.id, g.contribution_amount
  FROM rounds r
  JOIN groups g ON g.id = r.group_id
  JOIN group_members m ON m.group_id = g.id AND m.status = 'active'
 WHERE r.group_id = $1`

// CreateSchedule membuat seluruh putaran dan tagihan iuran sebuah grup berdasarkan
// turn_order anggota aktif. Dipanggil di dalam transaksi pemanggil (saat kocokan
// dijalankan), sehingga jadwal selalu terbentuk atomik bersama penetapan urutan.
// Dipanggil dua kali untuk grup yang sama akan gagal (UNIQUE group_id, round_no).
func CreateSchedule(ctx context.Context, tx pgx.Tx, groupID string) error {
	var missing int
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM group_members
		  WHERE group_id = $1 AND status = 'active' AND turn_order IS NULL`, groupID,
	).Scan(&missing); err != nil {
		return err
	}
	if missing > 0 {
		return ErrTurnOrderMissing
	}

	tag, err := tx.Exec(ctx, insertRounds, groupID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNoMembers
	}
	_, err = tx.Exec(ctx, insertContributions, groupID)
	return err
}

// Complete menutup putaran yang sudah lunas: statusnya menjadi paid_out, putaran
// berikutnya mulai menerima iuran, dan jika ini putaran terakhir grup menjadi done.
// Dipanggil di dalam transaksi pencairan dana (modul payout). Mengembalikan true
// jika grup selesai.
func Complete(ctx context.Context, tx pgx.Tx, roundID string) (groupDone bool, err error) {
	var groupID, status string
	var no int
	err = tx.QueryRow(ctx,
		`SELECT group_id, round_no, status FROM rounds WHERE id = $1 FOR UPDATE`, roundID,
	).Scan(&groupID, &no, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrNotFound
	}
	if err != nil {
		return false, err
	}
	if status != "collecting" {
		return false, ErrRoundNotCollecting
	}

	var unpaid int
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM contributions WHERE round_id = $1 AND status <> 'paid'`, roundID,
	).Scan(&unpaid); err != nil {
		return false, err
	}
	if unpaid > 0 {
		return false, ErrContributionsPending
	}

	if _, err := tx.Exec(ctx, `UPDATE rounds SET status = 'paid_out' WHERE id = $1`, roundID); err != nil {
		return false, err
	}
	next, err := tx.Exec(ctx,
		`UPDATE rounds SET status = 'collecting'
		  WHERE group_id = $1 AND round_no = $2 AND status = 'upcoming'`, groupID, no+1)
	if err != nil {
		return false, err
	}
	if next.RowsAffected() == 0 { // tidak ada putaran berikutnya
		if _, err := tx.Exec(ctx, `UPDATE groups SET status = 'done' WHERE id = $1`, groupID); err != nil {
			return false, err
		}
		return true, nil
	}
	return false, nil
}
