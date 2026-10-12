/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package payment

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/alexistdev/geopatungan/internal/contribution"
	"github.com/alexistdev/geopatungan/internal/referral"
	"github.com/alexistdev/geopatungan/internal/round"
	"github.com/alexistdev/geopatungan/internal/settings"
	"github.com/alexistdev/geopatungan/internal/wallet"
)

// FeeSource mengembalikan biaya platform (basis poin) yang berlaku. Dibaca di dalam
// transaksi pembayaran supaya konsisten.
type FeeSource func(ctx context.Context, q settings.Querier) (int, error)

type Repository struct {
	db   *pgxpool.Pool
	fees FeeSource
	now  func() time.Time
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{
		db: db,
		fees: func(ctx context.Context, q settings.Querier) (int, error) {
			s, err := settings.Load(ctx, q)
			return s.PlatformFeeBps, err
		},
		now: time.Now,
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// Pay membayar iuran milik userID dari saldonya. Semua langkah berjalan dalam SATU transaksi:
//
//  1. verifikasi kepemilikan, lalu kunci baris putaran;
//  2. kunci SEMUA dompet yang akan disentuh dalam urutan tetap;
//  3. tandai iuran lunas (menghitung biaya platform dan denda);
//  4. catat uang: pembayar -total, escrow +iuran, platform +(biaya+denda);
//  5. jika ini iuran terakhir putaran itu: tutup putaran dan cairkan seluruh dana
//     (escrow -> penerima giliran).
//
// Gagal di langkah mana pun membatalkan semuanya: tidak ada iuran setengah lunas, saldo
// terpotong tanpa iuran lunas, atau putaran lunas tanpa pencairan.
//
// Mengapa kunci putaran (1): beberapa anggota membayar bersamaan. Tanpa serialisasi, dua pembayar
// terakhir sama-sama menghitung "masih ada yang belum membayar" (masing-masing tidak melihat
// pembayaran lawannya yang belum commit), sehingga tidak ada yang menutup putaran dan dana
// tertahan di escrow selamanya. Saat ini kunci dompet escrow (2) kebetulan sudah menyerialkan
// semua pembayaran; kunci putaran dipertahankan agar kebenaran tidak bergantung pada detail
// itu (mis. jika escrow kelak dibuat per grup).
//
// Mengapa kunci dompet di awal (2): langkah 4 dan 5 memanggil wallet.Post dua kali. Jika dompet
// dikunci bertahap, dua transaksi pada putaran berbeda bisa mengunci dompet dalam urutan
// berlawanan (mis. penerima giliran di satu grup sedang membayar di grup lain) dan saling
// menunggu (deadlock).
func (r *Repository) Pay(ctx context.Context, userID, contributionID string) (*Result, error) {
	res, err := r.pay(ctx, userID, contributionID)
	var short *InsufficientFundsError
	if errors.As(err, &short) {
		// saldo dibaca setelah rollback (di luar transaksi)
		if err := r.db.QueryRow(ctx,
			`SELECT COALESCE((SELECT balance FROM wallets WHERE user_id = $1), 0)`, userID).Scan(&short.Balance); err != nil {
			return nil, err
		}
	}
	return res, err
}

func (r *Repository) pay(ctx context.Context, userID, contributionID string) (*Result, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	now := r.now()

	// 1. Kepemilikan dicek sebelum mengunci apa pun, supaya orang luar tidak bisa menahan kunci putaran.
	if _, err := contribution.QuoteFor(ctx, tx, contributionID, userID, now, 0); err != nil {
		return nil, err
	}
	var roundID, groupName string
	var roundNo int
	if err := tx.QueryRow(ctx,
		`SELECT r.id::text, r.round_no, g.name
		   FROM contributions c JOIN rounds r ON r.id = c.round_id JOIN groups g ON g.id = r.group_id
		  WHERE c.id = $1`, contributionID).Scan(&roundID, &roundNo, &groupName); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `SELECT 1 FROM rounds WHERE id = $1 FOR UPDATE`, roundID); err != nil {
		return nil, err
	}

	// 2. Dompet: pembayar, penerima giliran, escrow, platform.
	var recipientUserID, recipientName string
	if err := tx.QueryRow(ctx,
		`SELECT m.user_id::text, u.name
		   FROM rounds r JOIN group_members m ON m.id = r.recipient_member_id JOIN users u ON u.id = m.user_id
		  WHERE r.id = $1`, roundID).Scan(&recipientUserID, &recipientName); err != nil {
		return nil, err
	}
	payerWallet, err := wallet.EnsureUserWallet(ctx, tx, userID)
	if err != nil {
		return nil, err
	}
	recipientWallet, err := wallet.EnsureUserWallet(ctx, tx, recipientUserID)
	if err != nil {
		return nil, err
	}
	escrow, err := wallet.SystemWallet(ctx, tx, wallet.TypeEscrow)
	if err != nil {
		return nil, err
	}
	platform, err := wallet.SystemWallet(ctx, tx, wallet.TypePlatform)
	if err != nil {
		return nil, err
	}
	// Upline yang berhak atas komisi (maksimal 4). Dompet mereka dikunci bersama yang lain di awal:
	// komisi dibayar di Post tersendiri, dan tanpa kunci awal dua transaksi bisa mengunci dompet dalam
	// urutan berlawanan (mis. upline yang sedang menjadi pembayar di grup lain) lalu saling menunggu.
	plan, err := referral.Plan(ctx, tx, userID)
	if err != nil {
		return nil, err
	}
	uplineWallet := make(map[string]string, len(plan))
	lockSet := []string{payerWallet, recipientWallet, escrow, platform}
	for _, b := range plan {
		w, err := wallet.EnsureUserWallet(ctx, tx, b.UserID)
		if err != nil {
			return nil, err
		}
		uplineWallet[b.UserID] = w
		lockSet = append(lockSet, w)
	}
	if err := wallet.LockWallets(ctx, tx, lockSet...); err != nil {
		return nil, err
	}

	// 3. Tandai lunas (biaya platform dibaca di dalam transaksi).
	bps, err := r.fees(ctx, tx)
	if err != nil {
		return nil, err
	}
	receipt, err := contribution.MarkPaid(ctx, tx, contributionID, userID, now, bps)
	if err != nil {
		return nil, err
	}

	// 4. Catat uang.
	legs := []wallet.Leg{
		{WalletID: payerWallet, Amount: -receipt.Total},
		{WalletID: escrow, Amount: receipt.Amount},
	}
	if fees := receipt.PlatformFee + receipt.LateFee; fees > 0 {
		legs = append(legs, wallet.Leg{WalletID: platform, Amount: fees})
	}
	balances, err := wallet.Post(ctx, tx, wallet.Ref{
		Type:        "contribution",
		ID:          contributionID,
		Description: truncate(fmt.Sprintf("Iuran putaran %d - %s", roundNo, groupName), 200),
	}, legs...)
	switch {
	case errors.Is(err, wallet.ErrInsufficientFunds):
		return nil, &InsufficientFundsError{Required: receipt.Total}
	case errors.Is(err, wallet.ErrAlreadyPosted):
		return nil, contribution.ErrAlreadyPaid
	case err != nil:
		return nil, err
	}
	res := &Result{Receipt: receipt, Balance: balances[payerWallet]}

	// Komisi referral: HANYA dari biaya platform atas iuran ini (bukan denda, bukan dana arisan),
	// dipindahkan dari dompet platform ke saldo upline. Sisa pembulatan dan bagian upline yang tidak
	// berhak tetap di platform.
	if commissions := referral.Amounts(plan, receipt.PlatformFee); len(commissions) > 0 {
		legs := []wallet.Leg{{WalletID: platform, Amount: -referral.Total(commissions)}}
		for _, c := range commissions {
			legs = append(legs, wallet.Leg{WalletID: uplineWallet[c.UserID], Amount: c.Amount})
		}
		if _, err := wallet.Post(ctx, tx, wallet.Ref{
			Type:        "referral_commission",
			ID:          contributionID,
			Description: "Komisi referral dari biaya platform",
		}, legs...); err != nil {
			return nil, err
		}
		if err := referral.Record(ctx, tx, contributionID, userID, commissions); err != nil {
			return nil, err
		}
	}

	// 5. Iuran terakhir? Tutup putaran dan cairkan.
	var unpaid int
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM contributions WHERE round_id = $1 AND status <> 'paid'`, roundID).Scan(&unpaid); err != nil {
		return nil, err
	}
	if unpaid == 0 {
		var pot int64
		if err := tx.QueryRow(ctx,
			`SELECT COALESCE(SUM(amount), 0)::bigint FROM contributions WHERE round_id = $1`, roundID).Scan(&pot); err != nil {
			return nil, err
		}
		groupDone, err := round.Complete(ctx, tx, roundID)
		if err != nil {
			return nil, err
		}
		payout, err := wallet.Post(ctx, tx, wallet.Ref{
			Type:        "payout",
			ID:          roundID,
			Description: truncate(fmt.Sprintf("Pencairan putaran %d - %s", roundNo, groupName), 200),
		}, wallet.Leg{WalletID: escrow, Amount: -pot}, wallet.Leg{WalletID: recipientWallet, Amount: pot})
		if err != nil {
			return nil, err
		}
		if recipientWallet == payerWallet { // pembayar terakhir sekaligus penerima: saldo akhir sudah termasuk pencairan
			res.Balance = payout[payerWallet]
		}
		res.Round = RoundOutcome{Completed: true, GroupDone: groupDone, Payout: &Payout{RecipientName: recipientName, Amount: pot}}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return res, nil
}

// Quote menghitung rincian pembayaran saat ini dan kecukupan saldo, tanpa mengubah apa pun.
func (r *Repository) Quote(ctx context.Context, userID, contributionID string) (*QuoteResult, error) {
	bps, err := r.fees(ctx, r.db)
	if err != nil {
		return nil, err
	}
	q, err := contribution.QuoteFor(ctx, r.db, contributionID, userID, r.now(), bps)
	if err != nil {
		return nil, err
	}
	var balance int64
	if err := r.db.QueryRow(ctx,
		`SELECT COALESCE((SELECT balance FROM wallets WHERE user_id = $1), 0)`, userID).Scan(&balance); err != nil {
		return nil, err
	}
	return &QuoteResult{Quote: q, Balance: balance, Sufficient: q.Status == "paid" || balance >= q.Total}, nil
}
