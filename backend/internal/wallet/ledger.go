/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package wallet

import (
	"context"
	"errors"
	"math"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/alexistdev/geopatungan/internal/httpx"
)

var refTypeRe = regexp.MustCompile(`^[a-z][a-z_]{0,39}$`)

func validate(ref Ref, legs []Leg) error {
	if !refTypeRe.MatchString(ref.Type) || !httpx.IsUUID(ref.ID) || len(ref.Description) > 200 {
		return ErrInvalidLegs
	}
	if len(legs) < 2 {
		return ErrInvalidLegs
	}
	var sum int64
	seen := make(map[string]bool, len(legs))
	for _, l := range legs {
		id := strings.ToLower(l.WalletID)
		if l.Amount == 0 || !httpx.IsUUID(l.WalletID) || seen[id] {
			return ErrInvalidLegs
		}
		seen[id] = true
		// Penjumlahan int64 bisa overflow: +MaxInt64 +MaxInt64 +2 = 0 setelah wrap-around,
		// padahal semuanya kredit. Overflow dianggap tidak valid.
		if (l.Amount > 0 && sum > math.MaxInt64-l.Amount) || (l.Amount < 0 && sum < math.MinInt64-l.Amount) {
			return ErrInvalidLegs
		}
		sum += l.Amount
	}
	if sum != 0 {
		return ErrInvalidLegs
	}
	return nil
}

// Post mencatat satu peristiwa uang dalam transaksi pemanggil dan mengembalikan saldo
// baru tiap dompet. Aturannya:
//
//   - legs berjumlah tepat 0, minimal dua, tiap dompet muncul sekali, tanpa nominal 0;
//   - semua dompet dikunci (FOR UPDATE) dalam urutan id yang sama, sehingga dua Post
//     yang bersilangan tidak saling buntu (deadlock);
//   - dompet selain "external" tidak boleh menjadi negatif (ErrInsufficientFunds);
//   - peristiwa (ref) yang sama tidak bisa dicatat dua kali (ErrAlreadyPosted).
//
// Karena memakai transaksi pemanggil, catatan uang bisa digabung atomik dengan perubahan
// lain (mis. menandai iuran lunas). Jika transaksi dibatalkan, seluruhnya batal.
func Post(ctx context.Context, tx pgx.Tx, ref Ref, legs ...Leg) (map[string]int64, error) {
	if err := validate(ref, legs); err != nil {
		return nil, err
	}

	ids := make([]string, len(legs))
	for i, l := range legs {
		ids[i] = l.WalletID
	}
	rows, err := tx.Query(ctx,
		`SELECT id::text, type, balance FROM wallets WHERE id = ANY($1::uuid[]) ORDER BY id FOR UPDATE`, ids)
	if err != nil {
		return nil, err
	}
	type locked struct {
		typ     string
		balance int64
	}
	byID := make(map[string]locked, len(legs))
	for rows.Next() {
		var id, typ string
		var bal int64
		if err := rows.Scan(&id, &typ, &bal); err != nil {
			rows.Close()
			return nil, err
		}
		byID[id] = locked{typ, bal}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(byID) != len(legs) {
		return nil, ErrWalletNotFound
	}

	newBalances := make(map[string]int64, len(legs))
	for _, l := range legs {
		id := strings.ToLower(l.WalletID)
		w := byID[id]
		nb := w.balance + l.Amount
		if nb < 0 && w.typ != TypeExternal {
			return nil, ErrInsufficientFunds
		}
		newBalances[id] = nb
	}

	var group string
	if err := tx.QueryRow(ctx, `SELECT gen_random_uuid()::text`).Scan(&group); err != nil {
		return nil, err
	}
	for _, l := range legs {
		id := strings.ToLower(l.WalletID)
		if _, err := tx.Exec(ctx, `UPDATE wallets SET balance = $2 WHERE id = $1`, id, newBalances[id]); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO ledger_entries (entry_group, wallet_id, amount, balance_after, ref_type, ref_id, description)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			group, id, l.Amount, newBalances[id], ref.Type, ref.ID, ref.Description); err != nil {
			if isUniqueViolation(err) {
				return nil, ErrAlreadyPosted
			}
			return nil, err
		}
	}
	return newBalances, nil
}

// LockWallets mengunci dompet-dompet (FOR UPDATE) dalam urutan id yang tetap. Operasi yang
// memanggil Post lebih dari sekali dalam satu transaksi (mis. pembayaran iuran yang sekaligus
// mencairkan dana) HARUS memanggil ini lebih dulu dengan SEMUA dompet yang akan disentuh.
// Tanpanya, dua transaksi bisa mengunci dompet dalam urutan berbeda dan saling menunggu (deadlock).
func LockWallets(ctx context.Context, tx pgx.Tx, ids ...string) error {
	uniq := make([]string, 0, len(ids))
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		l := strings.ToLower(id)
		if !httpx.IsUUID(id) {
			return ErrInvalidLegs
		}
		if !seen[l] {
			seen[l] = true
			uniq = append(uniq, l)
		}
	}
	tag, err := tx.Exec(ctx, `SELECT id FROM wallets WHERE id = ANY($1::uuid[]) ORDER BY id FOR UPDATE`, uniq)
	if err != nil {
		return err
	}
	if int(tag.RowsAffected()) != len(uniq) {
		return ErrWalletNotFound
	}
	return nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// DB dipenuhi *pgxpool.Pool maupun pgx.Tx.
type DB interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// EnsureUserWallet mengembalikan ID dompet pengguna, membuatnya jika belum ada.
// Aman dipanggil bersamaan (INSERT ... ON CONFLICT DO NOTHING).
func EnsureUserWallet(ctx context.Context, db DB, userID string) (string, error) {
	if _, err := db.Exec(ctx,
		`INSERT INTO wallets (user_id, type) VALUES ($1, 'user') ON CONFLICT (user_id) DO NOTHING`, userID); err != nil {
		return "", err
	}
	var id string
	if err := db.QueryRow(ctx, `SELECT id::text FROM wallets WHERE user_id = $1`, userID).Scan(&id); err != nil {
		return "", err
	}
	return id, nil
}

// SystemWallet mengembalikan ID dompet sistem berjenis escrow/platform/holding/external.
func SystemWallet(ctx context.Context, db DB, typ string) (string, error) {
	if typ == TypeUser {
		return "", ErrWalletNotFound
	}
	var id string
	err := db.QueryRow(ctx, `SELECT id::text FROM wallets WHERE type = $1`, typ).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrWalletNotFound
	}
	return id, err
}
