/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package reputation

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/alexistdev/geopatungan/internal/audit"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

// Levels mengembalikan definisi level terurut menurut rank naik.
func (r *Repository) Levels(ctx context.Context) ([]Level, error) {
	rows, err := r.db.Query(ctx,
		`SELECT rank, code, name, require_verified, min_completed_cycles, max_late_count,
		        draw_weight, can_create_group, max_group_value
		   FROM reputation_levels ORDER BY rank`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var levels []Level
	for rows.Next() {
		var l Level
		if err := rows.Scan(&l.Rank, &l.Code, &l.Name, &l.RequireVerified, &l.MinCompletedCycles,
			&l.MaxLateCount, &l.DrawWeight, &l.CanCreateGroup, &l.MaxGroupValue); err != nil {
			return nil, err
		}
		levels = append(levels, l)
	}
	return levels, rows.Err()
}

// Stats mengumpulkan fakta reputasi seorang pengguna dari tabel sumber.
//
// "Menunggak" = iuran belum dibayar pada putaran yang sedang menerima iuran dan sudah lewat
// jatuh tempo (hari ini menurut WIB). Iuran pada putaran yang belum dibuka tidak dihitung.
func (r *Repository) Stats(ctx context.Context, userID string) (Stats, error) {
	var st Stats
	err := r.db.QueryRow(ctx,
		`SELECT u.kyc_status,
		        (SELECT count(*) FROM group_members m JOIN groups g ON g.id = m.group_id
		          WHERE m.user_id = u.id AND m.status = 'active' AND g.status = 'done'),
		        COALESCE(c.ontime, 0), COALESCE(c.late, 0), COALESCE(c.overdue, 0)
		   FROM users u
		   LEFT JOIN LATERAL (
		        SELECT count(*) FILTER (WHERE c.status = 'paid' AND c.paid_on_time)     AS ontime,
		               count(*) FILTER (WHERE c.status = 'paid' AND NOT c.paid_on_time) AS late,
		               count(*) FILTER (WHERE c.status = 'unpaid' AND r.status = 'collecting'
		                                  AND r.due_date < (now() AT TIME ZONE 'Asia/Jakarta')::date) AS overdue
		          FROM contributions c
		          JOIN group_members m ON m.id = c.member_id
		          JOIN rounds r ON r.id = c.round_id
		         WHERE m.user_id = u.id) c ON true
		  WHERE u.id = $1`, userID,
	).Scan(&st.KYCStatus, &st.CompletedCycles, &st.OnTimePayments, &st.LatePayments, &st.OverdueUnpaid)
	if errors.Is(err, pgx.ErrNoRows) {
		return Stats{}, ErrNotFound
	}
	if err != nil {
		return Stats{}, err
	}
	st.Verified = st.KYCStatus == "verified"
	st.LateTotal = st.LatePayments + st.OverdueUnpaid
	return st, nil
}

// UpdateLevels mengganti seluruh definisi level dan mencatat audit dalam satu transaksi.
// Level seorang pengguna dihitung ulang dari definisi baru saat berikutnya dibutuhkan.
func (r *Repository) UpdateLevels(ctx context.Context, adminID string, levels []Level) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := saveLevels(ctx, tx, adminID, levels); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// saveLevels dipisah dari UpdateLevels supaya tes bisa menjalankannya di transaksi yang
// di-rollback tanpa menyentuh definisi level sungguhan.
func saveLevels(ctx context.Context, tx pgx.Tx, adminID string, levels []Level) error {
	for _, l := range levels {
		if _, err := tx.Exec(ctx,
			`UPDATE reputation_levels
			    SET code = $2, name = $3, require_verified = $4, min_completed_cycles = $5, max_late_count = $6,
			        draw_weight = $7, can_create_group = $8, max_group_value = $9, updated_at = now()
			  WHERE rank = $1`,
			l.Rank, l.Code, l.Name, l.RequireVerified, l.MinCompletedCycles, l.MaxLateCount,
			l.DrawWeight, l.CanCreateGroup, l.MaxGroupValue); err != nil {
			return err
		}
	}
	return audit.Record(ctx, tx, adminID, "reputation.levels.update", "reputation_levels", "",
		map[string]any{"levels": levels})
}
