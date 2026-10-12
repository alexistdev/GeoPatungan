/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package wallet

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

// ByUser mengembalikan dompet pengguna, membuatnya jika belum ada.
func (r *Repository) ByUser(ctx context.Context, userID string) (*Wallet, error) {
	id, err := EnsureUserWallet(ctx, r.db, userID)
	if err != nil {
		return nil, err
	}
	w := Wallet{ID: id}
	if err := r.db.QueryRow(ctx,
		`SELECT balance, created_at FROM wallets WHERE id = $1`, id).Scan(&w.Balance, &w.CreatedAt); err != nil {
		return nil, err
	}
	return &w, nil
}

// Ledger mengembalikan riwayat mutasi dompet pengguna, terbaru lebih dulu.
// before=0 berarti dari yang terbaru; selain itu hanya entri ber-id lebih kecil.
// Mengambil limit+1 baris agar pemanggil tahu apakah masih ada halaman berikutnya.
func (r *Repository) Ledger(ctx context.Context, userID string, limit int, before int64) ([]Entry, error) {
	rows, err := r.db.Query(ctx,
		`SELECT l.id, l.amount, l.balance_after, l.ref_type, l.ref_id::text, l.description, l.created_at
		   FROM ledger_entries l JOIN wallets w ON w.id = l.wallet_id
		  WHERE w.user_id = $1 AND ($2 = 0 OR l.id < $2)
		  ORDER BY l.id DESC LIMIT $3`, userID, before, limit+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entries := []Entry{}
	for rows.Next() {
		var e Entry
		if err := rows.Scan(&e.ID, &e.Amount, &e.BalanceAfter, &e.RefType, &e.RefID, &e.Description, &e.CreatedAt); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

func (r *Repository) Reconcile(ctx context.Context) (*Report, error) {
	return Reconcile(ctx, r.db)
}
