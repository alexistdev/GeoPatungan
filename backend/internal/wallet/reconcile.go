/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package wallet

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// Report adalah hasil rekonsiliasi. Ok=true berarti keempat invarian terpenuhi.
type Report struct {
	Ok       bool     `json:"ok"`
	Problems []string `json:"problems"`
	Wallets  int      `json:"wallets_checked"`
	Entries  int      `json:"entries_checked"`
}

const maxProblems = 50

// Querier dipenuhi *pgxpool.Pool maupun pgx.Tx.
type Querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Reconcile memeriksa invarian pembukuan:
//
//  1. saldo tiap dompet = jumlah seluruh mutasinya di ledger;
//  2. tiap entry_group berjumlah 0;
//  3. jumlah seluruh saldo = 0;
//  4. balance_after tiap baris = saldo berjalan dompet pada baris itu.
//
// Pelanggaran berarti ada uang yang "hilang" atau "muncul" dan harus diselidiki.
func Reconcile(ctx context.Context, q Querier) (*Report, error) {
	rep := &Report{Problems: []string{}}
	add := func(format string, a ...any) {
		if len(rep.Problems) < maxProblems {
			rep.Problems = append(rep.Problems, fmt.Sprintf(format, a...))
		}
	}

	if err := q.QueryRow(ctx, `SELECT count(*) FROM wallets`).Scan(&rep.Wallets); err != nil {
		return nil, err
	}
	if err := q.QueryRow(ctx, `SELECT count(*) FROM ledger_entries`).Scan(&rep.Entries); err != nil {
		return nil, err
	}

	// 1. saldo dompet = jumlah ledger
	rows, err := q.Query(ctx,
		`SELECT w.id::text, w.balance, COALESCE(SUM(l.amount), 0)::bigint
		   FROM wallets w LEFT JOIN ledger_entries l ON l.wallet_id = w.id
		  GROUP BY w.id, w.balance
		 HAVING w.balance <> COALESCE(SUM(l.amount), 0)`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id string
		var balance, ledger int64
		if err := rows.Scan(&id, &balance, &ledger); err != nil {
			rows.Close()
			return nil, err
		}
		add("dompet %s: saldo %d tidak sama dengan jumlah ledger %d", id, balance, ledger)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// 2. tiap entry_group seimbang
	rows, err = q.Query(ctx,
		`SELECT entry_group::text, SUM(amount)::bigint FROM ledger_entries
		  GROUP BY entry_group HAVING SUM(amount) <> 0`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var g string
		var sum int64
		if err := rows.Scan(&g, &sum); err != nil {
			rows.Close()
			return nil, err
		}
		add("entry_group %s tidak seimbang (jumlah %d)", g, sum)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// 3. jumlah seluruh saldo = 0
	var total int64
	if err := q.QueryRow(ctx, `SELECT COALESCE(SUM(balance), 0)::bigint FROM wallets`).Scan(&total); err != nil {
		return nil, err
	}
	if total != 0 {
		add("jumlah seluruh saldo seharusnya 0, tetapi %d", total)
	}

	// 4. rantai balance_after
	rows, err = q.Query(ctx,
		`SELECT id, wallet_id::text, balance_after, running FROM (
		     SELECT id, wallet_id, balance_after,
		            SUM(amount) OVER (PARTITION BY wallet_id ORDER BY id)::bigint AS running
		       FROM ledger_entries) x
		  WHERE balance_after <> running ORDER BY id`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id, balanceAfter, running int64
		var wallet string
		if err := rows.Scan(&id, &wallet, &balanceAfter, &running); err != nil {
			rows.Close()
			return nil, err
		}
		add("baris ledger %d (dompet %s): balance_after %d seharusnya %d", id, wallet, balanceAfter, running)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	rep.Ok = len(rep.Problems) == 0
	return rep, nil
}
