/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package topup

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/alexistdev/geopatungan/internal/audit"
	"github.com/alexistdev/geopatungan/internal/wallet"
)

const columns = `t.id, t.user_id, t.amount, t.unique_code, t.transfer_amount, t.status, t.sender_name,
	t.sender_bank, t.note, t.submitted_at, t.expires_at, t.reviewed_at, t.review_note,
	t.received_amount, t.created_at`

// returning menjadikan baris hasil INSERT/UPDATE berbentuk sama dengan SELECT (alias t).
const returning = `RETURNING id, user_id, amount, unique_code, transfer_amount, status, sender_name,
	sender_bank, note, submitted_at, expires_at, reviewed_at, review_note, received_amount, created_at`

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func scan(row pgx.Row, t *Topup, extra ...any) error {
	dest := []any{&t.ID, &t.UserID, &t.Amount, &t.UniqueCode, &t.TransferAmount, &t.Status, &t.SenderName,
		&t.SenderBank, &t.Note, &t.SubmittedAt, &t.ExpiresAt, &t.ReviewedAt, &t.ReviewNote,
		&t.ReceivedAmount, &t.CreatedAt}
	return row.Scan(append(dest, extra...)...)
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// Create membuat permintaan top-up. codes adalah kandidat kode unik (urut prioritas);
// yang pertama menghasilkan nominal transfer yang belum dipakai permintaan terbuka lain
// akan dipakai. Dalam satu transaksi:
//
//   - kunci advisory per pengguna, supaya pembuatan serentak tidak melewati batas maxOpen;
//   - permintaan pending yang sudah lewat batas waktu dikedaluwarsakan (membebaskan kodenya);
//   - batas maxOpen permintaan terbuka per pengguna dicek.
func (r *Repository) Create(ctx context.Context, userID string, amount int64, codes []int, expiryHours, maxOpen int) (*Topup, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('topup:' || $1::text))`, userID); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE topups SET status = 'expired' WHERE status = 'pending' AND expires_at < now()`); err != nil {
		return nil, err
	}

	var open int
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM topups WHERE user_id = $1 AND status IN ('pending', 'submitted')`, userID,
	).Scan(&open); err != nil {
		return nil, err
	}
	if open >= maxOpen {
		return nil, ErrTooManyOpen
	}

	for _, code := range codes {
		// SAVEPOINT per percobaan: pelanggaran unique membatalkan transaksi, kecuali dibungkus savepoint.
		sp, err := tx.Begin(ctx)
		if err != nil {
			return nil, err
		}
		var t Topup
		err = scan(sp.QueryRow(ctx,
			`INSERT INTO topups (user_id, amount, unique_code, transfer_amount, expires_at)
			 VALUES ($1, $2::bigint, $3::int, $2::bigint + $3::int, now() + make_interval(hours => $4::int)) `+returning,
			userID, amount, code, expiryHours), &t)
		if err == nil {
			if err := sp.Commit(ctx); err != nil {
				return nil, err
			}
			if err := tx.Commit(ctx); err != nil {
				return nil, err
			}
			return &t, nil
		}
		_ = sp.Rollback(ctx)
		if !isUniqueViolation(err) {
			return nil, err
		}
	}
	return nil, ErrBusy
}

// Get mengembalikan permintaan milik userID; ErrNotFound jika tidak ada atau milik orang lain.
func (r *Repository) Get(ctx context.Context, userID, id string) (*Topup, error) {
	var t Topup
	err := scan(r.db.QueryRow(ctx,
		`SELECT `+columns+` FROM topups t WHERE t.id = $1 AND t.user_id = $2`, id, userID), &t)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *Repository) ListByUser(ctx context.Context, userID string, limit int) ([]Topup, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+columns+` FROM topups t WHERE t.user_id = $1 ORDER BY t.created_at DESC, t.id LIMIT $2`,
		userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []Topup{}
	for rows.Next() {
		var t Topup
		if err := scan(rows, &t); err != nil {
			return nil, err
		}
		list = append(list, t)
	}
	return list, rows.Err()
}

// userAction menjalankan UPDATE bersyarat atas permintaan milik pengguna. Jika tidak ada baris
// yang berubah, dibedakan antara "tidak ada/bukan milik" (ErrNotFound) dan "status tidak cocok"
// (ErrInvalidState).
func (r *Repository) userAction(ctx context.Context, sql string, args ...any) (*Topup, error) {
	var t Topup
	err := scan(r.db.QueryRow(ctx, sql, args...), &t)
	if err == nil {
		return &t, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	var exists bool
	if err := r.db.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM topups WHERE id = $1 AND user_id = $2)`, args[0], args[1]).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrNotFound
	}
	return nil, ErrInvalidState
}

// Confirm mencatat bahwa pengguna sudah mentransfer. Boleh diulang selagi masih terbuka
// (untuk memperbaiki keterangan).
func (r *Repository) Confirm(ctx context.Context, userID, id, senderName, senderBank, note string) (*Topup, error) {
	return r.userAction(ctx,
		`UPDATE topups
		    SET status = 'submitted', sender_name = $3, sender_bank = $4, note = $5,
		        submitted_at = COALESCE(submitted_at, now())
		  WHERE id = $1 AND user_id = $2 AND status IN ('pending', 'submitted') `+returning,
		id, userID, senderName, senderBank, note)
}

// Cancel membatalkan permintaan yang masih pending. Yang sudah dikonfirmasi tidak bisa
// dibatalkan sendiri oleh pengguna: uangnya mungkin sudah masuk, admin yang memutuskan.
func (r *Repository) Cancel(ctx context.Context, userID, id string) (*Topup, error) {
	return r.userAction(ctx,
		`UPDATE topups SET status = 'cancelled'
		  WHERE id = $1 AND user_id = $2 AND status = 'pending' `+returning,
		id, userID)
}

// ListAdmin untuk admin. status: "open" (pending+submitted, terlama dulu), "all", atau satu status.
func (r *Repository) ListAdmin(ctx context.Context, status string, limit int) ([]AdminTopup, error) {
	order := "t.created_at DESC, t.id"
	if status == "open" {
		order = "t.created_at ASC, t.id"
	}
	rows, err := r.db.Query(ctx,
		`SELECT `+columns+`, u.name, u.email, t.reviewed_by::text
		   FROM topups t JOIN users u ON u.id = t.user_id
		  WHERE CASE $1 WHEN 'all' THEN true
		                WHEN 'open' THEN t.status IN ('pending', 'submitted')
		                ELSE t.status = $1 END
		  ORDER BY `+order+` LIMIT $2`, status, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []AdminTopup{}
	for rows.Next() {
		var a AdminTopup
		if err := scan(rows, &a.Topup, &a.UserName, &a.UserEmail, &a.ReviewedBy); err != nil {
			return nil, err
		}
		list = append(list, a)
	}
	return list, rows.Err()
}

// lockForReview mengunci permintaan dan memastikan peninjau boleh memprosesnya.
func lockForReview(ctx context.Context, tx pgx.Tx, adminID, id string) (userID, status string, transfer int64, err error) {
	err = tx.QueryRow(ctx,
		`SELECT user_id::text, status, transfer_amount FROM topups WHERE id = $1 FOR UPDATE`, id,
	).Scan(&userID, &status, &transfer)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", 0, ErrNotFound
	}
	if err != nil {
		return "", "", 0, err
	}
	if userID == adminID {
		return "", "", 0, ErrSelfReview
	}
	switch status {
	case StatusPending, StatusSubmitted, StatusExpired:
		return userID, status, transfer, nil
	default:
		return "", "", 0, ErrInvalidState
	}
}

// Approve memverifikasi transfer: saldo pengguna dikreditkan (dompet external berkurang)
// lewat wallet.Post, status menjadi approved, dan audit dicatat, semuanya dalam satu
// transaksi dengan permintaan terkunci. received nil = sebesar transfer_amount.
// Permintaan yang sudah kedaluwarsa tetap boleh disetujui: uang bisa saja datang terlambat.
func (r *Repository) Approve(ctx context.Context, adminID, id string, received *int64, note string) (*Topup, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	userID, _, transfer, err := lockForReview(ctx, tx, adminID, id)
	if err != nil {
		return nil, err
	}
	credited := transfer
	if received != nil {
		credited = *received
	}

	userWallet, err := wallet.EnsureUserWallet(ctx, tx, userID)
	if err != nil {
		return nil, err
	}
	external, err := wallet.SystemWallet(ctx, tx, wallet.TypeExternal)
	if err != nil {
		return nil, err
	}
	if _, err := wallet.Post(ctx, tx,
		wallet.Ref{Type: "topup", ID: id, Description: "Top-up via transfer bank"},
		wallet.Leg{WalletID: external, Amount: -credited},
		wallet.Leg{WalletID: userWallet, Amount: credited},
	); err != nil {
		if errors.Is(err, wallet.ErrAlreadyPosted) {
			return nil, ErrInvalidState // sudah pernah dikreditkan
		}
		return nil, err
	}

	var t Topup
	if err := scan(tx.QueryRow(ctx,
		`UPDATE topups
		    SET status = 'approved', received_amount = $2, reviewed_by = $3, reviewed_at = now(), review_note = $4
		  WHERE id = $1 `+returning, id, credited, adminID, note), &t); err != nil {
		return nil, err
	}
	if err := audit.Record(ctx, tx, adminID, "topup.approve", "topup", id,
		map[string]any{"user_id": userID, "transfer_amount": transfer, "received_amount": credited}); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &t, nil
}

// Reject menolak permintaan tanpa mengubah saldo.
func (r *Repository) Reject(ctx context.Context, adminID, id, note string) (*Topup, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	userID, _, transfer, err := lockForReview(ctx, tx, adminID, id)
	if err != nil {
		return nil, err
	}
	var t Topup
	if err := scan(tx.QueryRow(ctx,
		`UPDATE topups
		    SET status = 'rejected', reviewed_by = $2, reviewed_at = now(), review_note = $3
		  WHERE id = $1 `+returning, id, adminID, note), &t); err != nil {
		return nil, err
	}
	if err := audit.Record(ctx, tx, adminID, "topup.reject", "topup", id,
		map[string]any{"user_id": userID, "transfer_amount": transfer}); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &t, nil
}
