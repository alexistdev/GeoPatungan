/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package withdrawal

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/alexistdev/geopatungan/internal/audit"
	"github.com/alexistdev/geopatungan/internal/wallet"
)

const columns = `w.id, w.user_id, w.amount, w.bank_name, w.account_number, w.account_holder, w.note,
	w.status, w.transfer_reference, w.review_note, w.reviewed_at, w.created_at`

const returning = `RETURNING id, user_id, amount, bank_name, account_number, account_holder, note,
	status, transfer_reference, review_note, reviewed_at, created_at`

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func scan(row pgx.Row, w *Withdrawal, extra ...any) error {
	dest := []any{&w.ID, &w.UserID, &w.Amount, &w.BankName, &w.AccountNumber, &w.AccountHolder, &w.Note,
		&w.Status, &w.TransferReference, &w.ReviewNote, &w.ReviewedAt, &w.CreatedAt}
	return row.Scan(append(dest, extra...)...)
}

// Create membuat permintaan penarikan dan, dalam transaksi yang sama, memindahkan dana dari
// dompet pengguna ke "holding". Karena Post menolak saldo negatif di bawah kunci dompet,
// permintaan serentak tidak bisa menarik lebih dari saldo (double-spend); permintaan yang gagal
// tidak meninggalkan baris karena transaksinya dibatalkan.
func (r *Repository) Create(ctx context.Context, userID string, in Input, maxOpen int) (*Withdrawal, error) {
	w, err := r.create(ctx, userID, in, maxOpen)
	var short *wallet.InsufficientFundsError
	if errors.As(err, &short) {
		// saldo dibaca setelah rollback (di luar transaksi)
		if err := r.db.QueryRow(ctx,
			`SELECT COALESCE((SELECT balance FROM wallets WHERE user_id = $1), 0)`, userID).Scan(&short.Balance); err != nil {
			return nil, err
		}
	}
	return w, err
}

func (r *Repository) create(ctx context.Context, userID string, in Input, maxOpen int) (*Withdrawal, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Satu permintaan baru per pengguna pada satu waktu, supaya batas maxOpen tidak bisa dilewati.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('withdraw:' || $1::text))`, userID); err != nil {
		return nil, err
	}
	var open int
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM withdrawals WHERE user_id = $1 AND status = 'pending'`, userID).Scan(&open); err != nil {
		return nil, err
	}
	if open >= maxOpen {
		return nil, ErrTooManyOpen
	}

	var w Withdrawal
	if err := scan(tx.QueryRow(ctx,
		`INSERT INTO withdrawals (user_id, amount, bank_name, account_number, account_holder, note)
		 VALUES ($1, $2, $3, $4, $5, $6) `+returning,
		userID, in.Amount, in.BankName, in.AccountNumber, in.AccountHolder, in.Note), &w); err != nil {
		return nil, err
	}

	userWallet, err := wallet.EnsureUserWallet(ctx, tx, userID)
	if err != nil {
		return nil, err
	}
	holding, err := wallet.SystemWallet(ctx, tx, wallet.TypeHolding)
	if err != nil {
		return nil, err
	}
	if _, err := wallet.Post(ctx, tx, wallet.Ref{Type: "withdrawal_request", ID: w.ID, Description: "Permintaan penarikan dana"},
		wallet.Leg{WalletID: userWallet, Amount: -in.Amount},
		wallet.Leg{WalletID: holding, Amount: in.Amount}); err != nil {
		if errors.Is(err, wallet.ErrInsufficientFunds) {
			return nil, &wallet.InsufficientFundsError{Required: in.Amount}
		}
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &w, nil
}

// Get mengembalikan permintaan milik userID; ErrNotFound jika tidak ada atau milik orang lain.
func (r *Repository) Get(ctx context.Context, userID, id string) (*Withdrawal, error) {
	var w Withdrawal
	err := scan(r.db.QueryRow(ctx,
		`SELECT `+columns+` FROM withdrawals w WHERE w.id = $1 AND w.user_id = $2`, id, userID), &w)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &w, nil
}

func (r *Repository) ListByUser(ctx context.Context, userID string, limit int) ([]Withdrawal, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+columns+` FROM withdrawals w WHERE w.user_id = $1 ORDER BY w.created_at DESC, w.id LIMIT $2`,
		userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []Withdrawal{}
	for rows.Next() {
		var w Withdrawal
		if err := scan(rows, &w); err != nil {
			return nil, err
		}
		list = append(list, w)
	}
	return list, rows.Err()
}

// ListAdmin untuk admin. status: "pending" (terlama dulu), "all", atau satu status.
func (r *Repository) ListAdmin(ctx context.Context, status string, limit int) ([]AdminWithdrawal, error) {
	order := "w.created_at DESC, w.id"
	if status == StatusPending {
		order = "w.created_at ASC, w.id"
	}
	rows, err := r.db.Query(ctx,
		`SELECT `+columns+`, u.name, u.email, w.reviewed_by::text
		   FROM withdrawals w JOIN users u ON u.id = w.user_id
		  WHERE ($1 = 'all' OR w.status = $1)
		  ORDER BY `+order+` LIMIT $2`, status, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []AdminWithdrawal{}
	for rows.Next() {
		var a AdminWithdrawal
		if err := scan(rows, &a.Withdrawal, &a.UserName, &a.UserEmail, &a.ReviewedBy); err != nil {
			return nil, err
		}
		list = append(list, a)
	}
	return list, rows.Err()
}

// lockPending mengunci permintaan (FOR UPDATE) dan memastikan masih pending.
func lockPending(ctx context.Context, tx pgx.Tx, id string) (userID string, amount int64, err error) {
	var status string
	err = tx.QueryRow(ctx,
		`SELECT user_id::text, amount, status FROM withdrawals WHERE id = $1 FOR UPDATE`, id,
	).Scan(&userID, &amount, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", 0, ErrNotFound
	}
	if err != nil {
		return "", 0, err
	}
	if status != StatusPending {
		return "", 0, ErrInvalidState
	}
	return userID, amount, nil
}

// settle memindahkan dana dari holding ke tujuan (external atau dompet pengguna) untuk satu
// peristiwa (refType), lalu memperbarui baris permintaan. Peristiwa yang sama tidak bisa
// tercatat dua kali (ref unik di ledger).
func settle(ctx context.Context, tx pgx.Tx, id, userID string, amount int64, refType, desc, toType, setSQL string, setArgs ...any) (*Withdrawal, error) {
	holding, err := wallet.SystemWallet(ctx, tx, wallet.TypeHolding)
	if err != nil {
		return nil, err
	}
	var dest string
	if toType == wallet.TypeUser {
		dest, err = wallet.EnsureUserWallet(ctx, tx, userID)
	} else {
		dest, err = wallet.SystemWallet(ctx, tx, toType)
	}
	if err != nil {
		return nil, err
	}
	if _, err := wallet.Post(ctx, tx, wallet.Ref{Type: refType, ID: id, Description: desc},
		wallet.Leg{WalletID: holding, Amount: -amount}, wallet.Leg{WalletID: dest, Amount: amount}); err != nil {
		if errors.Is(err, wallet.ErrAlreadyPosted) {
			return nil, ErrInvalidState
		}
		return nil, err
	}
	var w Withdrawal
	if err := scan(tx.QueryRow(ctx, setSQL+` `+returning, append([]any{id}, setArgs...)...), &w); err != nil {
		return nil, err
	}
	return &w, nil
}

// Complete menandai penarikan selesai setelah admin mentransfer ke rekening pengguna:
// dana holding -> external. reference adalah nomor referensi/bukti transfer dari bank.
func (r *Repository) Complete(ctx context.Context, adminID, id, reference, note string) (*Withdrawal, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	userID, amount, err := lockPending(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	if userID == adminID {
		return nil, ErrSelfReview
	}
	w, err := settle(ctx, tx, id, userID, amount, "withdrawal_complete", "Penarikan dana ke rekening bank", wallet.TypeExternal,
		`UPDATE withdrawals SET status = 'completed', transfer_reference = $2, review_note = $3,
		                        reviewed_by = $4, reviewed_at = now() WHERE id = $1`,
		reference, note, adminID)
	if err != nil {
		return nil, err
	}
	if err := audit.Record(ctx, tx, adminID, "withdrawal.complete", "withdrawal", id,
		map[string]any{"user_id": userID, "amount": amount, "transfer_reference": reference}); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return w, nil
}

// Reject menolak penarikan: dana holding kembali ke pengguna.
func (r *Repository) Reject(ctx context.Context, adminID, id, note string) (*Withdrawal, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	userID, amount, err := lockPending(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	if userID == adminID {
		return nil, ErrSelfReview
	}
	w, err := settle(ctx, tx, id, userID, amount, "withdrawal_reject", "Penarikan dana ditolak, dana dikembalikan", wallet.TypeUser,
		`UPDATE withdrawals SET status = 'rejected', review_note = $2, reviewed_by = $3, reviewed_at = now() WHERE id = $1`,
		note, adminID)
	if err != nil {
		return nil, err
	}
	if err := audit.Record(ctx, tx, adminID, "withdrawal.reject", "withdrawal", id,
		map[string]any{"user_id": userID, "amount": amount}); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return w, nil
}

// Cancel dipakai pengguna untuk membatalkan permintaannya sendiri selagi pending: dana holding
// kembali ke dompetnya. Milik orang lain atau yang tidak ada dianggap tidak ditemukan.
// Kepemilikan dicek lebih dulu TANPA kunci, supaya orang lain tidak bisa menahan kunci baris ini.
func (r *Repository) Cancel(ctx context.Context, userID, id string) (*Withdrawal, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var owns bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM withdrawals WHERE id = $1 AND user_id = $2)`, id, userID).Scan(&owns); err != nil {
		return nil, err
	}
	if !owns {
		return nil, ErrNotFound
	}
	_, amount, err := lockPending(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	w, err := settle(ctx, tx, id, userID, amount, "withdrawal_cancel", "Penarikan dana dibatalkan, dana dikembalikan", wallet.TypeUser,
		`UPDATE withdrawals SET status = 'cancelled' WHERE id = $1`)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return w, nil
}
