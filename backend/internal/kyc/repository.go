/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package kyc

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/alexistdev/geopatungan/internal/audit"
)

const columns = `r.id, r.user_id, r.status, r.note, r.review_note, r.submitted_at, r.reviewed_at`

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func scan(row pgx.Row, r *Request, extra ...any) error {
	dest := []any{&r.ID, &r.UserID, &r.Status, &r.Note, &r.ReviewNote, &r.SubmittedAt, &r.ReviewedAt}
	return row.Scan(append(dest, extra...)...)
}

// Submit membuat permintaan verifikasi. Baris pengguna dikunci supaya dua permintaan serentak
// tidak lolos berdua; yang sedang pending atau sudah terverifikasi ditolak.
func (r *Repository) Submit(ctx context.Context, userID, note string) (*Request, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var status string
	if err := tx.QueryRow(ctx, `SELECT kyc_status FROM users WHERE id = $1 FOR UPDATE`, userID).Scan(&status); err != nil {
		return nil, err
	}
	switch status {
	case StatusPending:
		return nil, ErrAlreadyPending
	case StatusVerified:
		return nil, ErrAlreadyVerified
	}

	var req Request
	if err := scan(tx.QueryRow(ctx,
		`INSERT INTO kyc_requests (user_id, note) VALUES ($1, $2)
		 RETURNING id, user_id, status, note, review_note, submitted_at, reviewed_at`, userID, note), &req); err != nil {
		// Lapis kedua: indeks unik uq_kyc_pending menolak permintaan pending ganda walau kunci di atas lolos.
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, ErrAlreadyPending
		}
		return nil, err
	}
	if _, err := tx.Exec(ctx, `UPDATE users SET kyc_status = 'pending' WHERE id = $1`, userID); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &req, nil
}

// StatusOf mengembalikan status verifikasi pengguna beserta permintaan terakhirnya.
func (r *Repository) StatusOf(ctx context.Context, userID string) (*Status, error) {
	st := &Status{}
	if err := r.db.QueryRow(ctx, `SELECT kyc_status FROM users WHERE id = $1`, userID).Scan(&st.KYCStatus); err != nil {
		return nil, err
	}
	var req Request
	err := scan(r.db.QueryRow(ctx,
		`SELECT `+columns+` FROM kyc_requests r WHERE r.user_id = $1 ORDER BY r.submitted_at DESC, r.id LIMIT 1`, userID), &req)
	if err == nil {
		st.Latest = &req
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	return st, nil
}

// ListAdmin untuk admin. status: "pending" (terlama dulu), "all", atau satu status.
func (r *Repository) ListAdmin(ctx context.Context, status string, limit int) ([]AdminRequest, error) {
	order := "r.submitted_at DESC, r.id"
	if status == StatusPending {
		order = "r.submitted_at ASC, r.id"
	}
	rows, err := r.db.Query(ctx,
		`SELECT `+columns+`, u.name, u.email, r.reviewed_by::text
		   FROM kyc_requests r JOIN users u ON u.id = r.user_id
		  WHERE ($1 = 'all' OR r.status = $1)
		  ORDER BY `+order+` LIMIT $2`, status, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []AdminRequest{}
	for rows.Next() {
		var a AdminRequest
		if err := scan(rows, &a.Request, &a.UserName, &a.UserEmail, &a.ReviewedBy); err != nil {
			return nil, err
		}
		list = append(list, a)
	}
	return list, rows.Err()
}

// Review memutuskan permintaan pending: baris permintaan dikunci, status akun pengguna ikut
// diperbarui, dan audit dicatat dalam satu transaksi. Admin tidak boleh memutuskan permintaannya
// sendiri (tanpa itu, seorang admin bisa menaikkan reputasinya sendiri).
func (r *Repository) Review(ctx context.Context, adminID, id, decision, note string) (*Request, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var userID, status string
	err = tx.QueryRow(ctx, `SELECT user_id::text, status FROM kyc_requests WHERE id = $1 FOR UPDATE`, id).Scan(&userID, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if userID == adminID {
		return nil, ErrSelfReview
	}
	if status != StatusPending {
		return nil, ErrInvalidState
	}

	var req Request
	if err := scan(tx.QueryRow(ctx,
		`UPDATE kyc_requests
		    SET status = $2, review_note = $3, reviewed_by = $4, reviewed_at = now()
		  WHERE id = $1
		 RETURNING id, user_id, status, note, review_note, submitted_at, reviewed_at`,
		id, decision, note, adminID), &req); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `UPDATE users SET kyc_status = $2 WHERE id = $1`, userID, decision); err != nil {
		return nil, err
	}
	if err := audit.Record(ctx, tx, adminID, "kyc."+decision, "kyc_request", id,
		map[string]any{"user_id": userID}); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &req, nil
}
