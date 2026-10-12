/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package wallet_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/alexistdev/geopatungan/internal/testutil"
	"github.com/alexistdev/geopatungan/internal/wallet"
)

// Tes ini butuh database Postgres dengan semua migrasi; dilewati jika TEST_DATABASE_URL kosong.

type world struct {
	e   *testutil.Env
	ctx context.Context
}

func newWorld(t *testing.T) *world { return &world{e: testutil.NewEnv(t), ctx: context.Background()} }

// user membuat pengguna uji beserta dompetnya, berisi `balance` rupiah.
func (w *world) user(name string, balance int64) (userID, walletID string) {
	w.e.T.Helper()
	userID = w.e.NewUser(name)
	if balance > 0 {
		w.e.FundUser(userID, balance)
	}
	id, err := wallet.EnsureUserWallet(w.ctx, w.e.Pool, userID)
	if err != nil {
		w.e.T.Fatal(err)
	}
	return userID, id
}

func (w *world) newRef(typ string) wallet.Ref {
	w.e.T.Helper()
	return wallet.Ref{Type: typ, ID: w.e.QueryString(`SELECT gen_random_uuid()::text`)}
}

func (w *world) post(ref wallet.Ref, legs ...wallet.Leg) (map[string]int64, error) {
	w.e.T.Helper()
	var balances map[string]int64
	err := w.e.InTx(func(tx pgx.Tx) error {
		var err error
		balances, err = wallet.Post(w.ctx, tx, ref, legs...)
		return err
	})
	return balances, err
}

func (w *world) balance(walletID string) int64 {
	w.e.T.Helper()
	return int64(w.e.QueryInt(`SELECT balance FROM wallets WHERE id = $1`, walletID))
}

func (w *world) entries(walletID string) int {
	w.e.T.Helper()
	return w.e.QueryInt(`SELECT count(*) FROM ledger_entries WHERE wallet_id = $1`, walletID)
}

func transfer(from, to string, amount int64) []wallet.Leg {
	return []wallet.Leg{{WalletID: from, Amount: -amount}, {WalletID: to, Amount: amount}}
}

// ---- Post: jalur normal -----------------------------------------------------

func TestPostTransfer(t *testing.T) {
	w := newWorld(t)
	_, a := w.user("a", 1000)
	_, b := w.user("b", 0)
	ref := w.newRef("test_transfer")
	ref.Description = "uji transfer"

	balances, err := w.post(ref, transfer(a, b, 300)...)
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	if balances[a] != 700 || balances[b] != 300 {
		t.Errorf("saldo kembalian = %v, want a=700 b=300", balances)
	}
	if w.balance(a) != 700 || w.balance(b) != 300 {
		t.Errorf("saldo DB a=%d b=%d, want 700 dan 300", w.balance(a), w.balance(b))
	}

	// dua baris ledger, satu entry_group, tanda dan balance_after benar
	var groupA, groupB string
	var amtA, amtB, afterA, afterB int64
	e := w.e
	if err := e.Pool.QueryRow(w.ctx, `SELECT entry_group::text, amount, balance_after FROM ledger_entries
	                                   WHERE wallet_id = $1 AND ref_id = $2`, a, ref.ID).Scan(&groupA, &amtA, &afterA); err != nil {
		t.Fatal(err)
	}
	if err := e.Pool.QueryRow(w.ctx, `SELECT entry_group::text, amount, balance_after FROM ledger_entries
	                                   WHERE wallet_id = $1 AND ref_id = $2`, b, ref.ID).Scan(&groupB, &amtB, &afterB); err != nil {
		t.Fatal(err)
	}
	if groupA != groupB {
		t.Error("kedua sisi harus satu entry_group")
	}
	if amtA != -300 || afterA != 700 || amtB != 300 || afterB != 300 {
		t.Errorf("A: %d/%d  B: %d/%d, want -300/700 dan 300/300", amtA, afterA, amtB, afterB)
	}
	if got := e.QueryString(`SELECT description FROM ledger_entries WHERE wallet_id = $1 AND ref_id = $2`, a, ref.ID); got != "uji transfer" {
		t.Errorf("description = %q", got)
	}
}

func TestPostThreeLegs(t *testing.T) {
	w := newWorld(t)
	_, payer := w.user("payer", 1000)
	_, escrow := w.user("escrow-sim", 0)
	_, fee := w.user("fee-sim", 0)

	// seperti pembayaran iuran: pembayar -1000, dua penerima +900 dan +100
	_, err := w.post(w.newRef("test_split"),
		wallet.Leg{WalletID: payer, Amount: -1000}, wallet.Leg{WalletID: escrow, Amount: 900}, wallet.Leg{WalletID: fee, Amount: 100})
	if err != nil {
		t.Fatal(err)
	}
	if w.balance(payer) != 0 || w.balance(escrow) != 900 || w.balance(fee) != 100 {
		t.Errorf("saldo = %d/%d/%d", w.balance(payer), w.balance(escrow), w.balance(fee))
	}
}

// ---- Post: input tidak valid ------------------------------------------------

func TestPostRejectsInvalidInput(t *testing.T) {
	w := newWorld(t)
	_, a := w.user("a", 500)
	_, b := w.user("b", 0)
	ghost := "00000000-0000-0000-0000-000000000000"
	ok := w.newRef("test_ok")

	tests := []struct {
		name string
		ref  wallet.Ref
		legs []wallet.Leg
		want error
	}{
		{"tanpa leg", ok, nil, wallet.ErrInvalidLegs},
		{"satu leg", ok, []wallet.Leg{{WalletID: a, Amount: 0}}, wallet.ErrInvalidLegs},
		{"satu leg bernilai", ok, []wallet.Leg{{WalletID: a, Amount: 5}}, wallet.ErrInvalidLegs},
		{"tidak seimbang", ok, []wallet.Leg{{WalletID: a, Amount: -100}, {WalletID: b, Amount: 99}}, wallet.ErrInvalidLegs},
		{"tidak seimbang (lebih)", ok, []wallet.Leg{{WalletID: a, Amount: -100}, {WalletID: b, Amount: 101}}, wallet.ErrInvalidLegs},
		{"leg bernilai nol", ok, []wallet.Leg{{WalletID: a, Amount: 0}, {WalletID: b, Amount: 0}}, wallet.ErrInvalidLegs},
		{"dompet sama dua kali", ok, []wallet.Leg{{WalletID: a, Amount: -100}, {WalletID: a, Amount: 100}}, wallet.ErrInvalidLegs},
		{"dompet sama beda huruf", ok, []wallet.Leg{{WalletID: a, Amount: -100}, {WalletID: strings.ToUpper(a), Amount: 100}}, wallet.ErrInvalidLegs},
		{"id dompet bukan UUID", ok, []wallet.Leg{{WalletID: "abc", Amount: -1}, {WalletID: b, Amount: 1}}, wallet.ErrInvalidLegs},
		{"jenis ref kosong", wallet.Ref{Type: "", ID: ok.ID}, transfer(a, b, 10), wallet.ErrInvalidLegs},
		{"jenis ref huruf besar", wallet.Ref{Type: "TopUp", ID: ok.ID}, transfer(a, b, 10), wallet.ErrInvalidLegs},
		{"jenis ref sisipan SQL", wallet.Ref{Type: "x'; DROP", ID: ok.ID}, transfer(a, b, 10), wallet.ErrInvalidLegs},
		{"id ref bukan UUID", wallet.Ref{Type: "test_ok", ID: "bukan-uuid"}, transfer(a, b, 10), wallet.ErrInvalidLegs},
		{"deskripsi terlalu panjang", wallet.Ref{Type: "test_ok", ID: ok.ID, Description: strings.Repeat("x", 201)}, transfer(a, b, 10), wallet.ErrInvalidLegs},
		{"dompet tidak ada", ok, transfer(a, ghost, 10), wallet.ErrWalletNotFound},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := w.post(tc.ref, tc.legs...); !errors.Is(err, tc.want) {
				t.Errorf("err = %v, want %v", err, tc.want)
			}
		})
	}
	if w.balance(a) != 500 || w.balance(b) != 0 {
		t.Errorf("saldo berubah oleh input tidak valid: a=%d b=%d", w.balance(a), w.balance(b))
	}
	if w.entries(a) != 1 || w.entries(b) != 0 { // 1 = pendanaan awal
		t.Errorf("ledger berubah oleh input tidak valid: a=%d b=%d baris", w.entries(a), w.entries(b))
	}
}

// ---- Post: saldo tidak cukup ------------------------------------------------

func TestPostInsufficientFunds(t *testing.T) {
	w := newWorld(t)
	_, a := w.user("a", 100)
	_, b := w.user("b", 0)

	if _, err := w.post(w.newRef("test_over"), transfer(a, b, 101)...); !errors.Is(err, wallet.ErrInsufficientFunds) {
		t.Fatalf("err = %v, want ErrInsufficientFunds", err)
	}
	if w.balance(a) != 100 || w.balance(b) != 0 || w.entries(a) != 1 || w.entries(b) != 0 {
		t.Error("penolakan tidak boleh meninggalkan perubahan apa pun")
	}

	// tepat sebesar saldo diperbolehkan -> saldo 0, bukan negatif
	if _, err := w.post(w.newRef("test_exact"), transfer(a, b, 100)...); err != nil {
		t.Fatalf("menghabiskan saldo: %v", err)
	}
	if w.balance(a) != 0 || w.balance(b) != 100 {
		t.Errorf("a=%d b=%d, want 0 dan 100", w.balance(a), w.balance(b))
	}
	if _, err := w.post(w.newRef("test_zero"), transfer(a, b, 1)...); !errors.Is(err, wallet.ErrInsufficientFunds) {
		t.Errorf("saldo 0: err = %v, want ErrInsufficientFunds", err)
	}
}

func TestExternalWalletMayGoNegative(t *testing.T) {
	w := newWorld(t)
	userID, uw := w.user("a", 0)
	ctx := w.ctx

	ext, err := wallet.SystemWallet(ctx, w.e.Pool, wallet.TypeExternal)
	if err != nil {
		t.Fatal(err)
	}
	before := w.balance(ext)
	// uang masuk dari bank: external turun, pengguna naik
	if _, err := w.post(w.newRef("test_in"), wallet.Leg{WalletID: ext, Amount: -250}, wallet.Leg{WalletID: uw, Amount: 250}); err != nil {
		t.Fatalf("external harus boleh negatif: %v", err)
	}
	if w.balance(ext) != before-250 || w.e.Balance(userID) != 250 {
		t.Errorf("external=%d (sebelumnya %d), pengguna=%d", w.balance(ext), before, w.e.Balance(userID))
	}
	// namun dompet pengguna tetap tidak boleh negatif
	if _, err := w.post(w.newRef("test_out"), wallet.Leg{WalletID: uw, Amount: -251}, wallet.Leg{WalletID: ext, Amount: 251}); !errors.Is(err, wallet.ErrInsufficientFunds) {
		t.Errorf("err = %v, want ErrInsufficientFunds", err)
	}
}

// ---- Post: idempotensi dan atomisitas ---------------------------------------

func TestPostIsIdempotentPerRef(t *testing.T) {
	w := newWorld(t)
	_, a := w.user("a", 1000)
	_, b := w.user("b", 0)
	ref := w.newRef("test_once")

	if _, err := w.post(ref, transfer(a, b, 100)...); err != nil {
		t.Fatal(err)
	}
	if _, err := w.post(ref, transfer(a, b, 100)...); !errors.Is(err, wallet.ErrAlreadyPosted) {
		t.Errorf("pencatatan ulang: err = %v, want ErrAlreadyPosted", err)
	}
	if w.balance(a) != 900 || w.balance(b) != 100 {
		t.Errorf("saldo a=%d b=%d, want 900 dan 100 (tidak boleh terpotong dua kali)", w.balance(a), w.balance(b))
	}
	// ref berbeda = peristiwa berbeda
	if _, err := w.post(w.newRef("test_once"), transfer(a, b, 100)...); err != nil {
		t.Errorf("ref lain: %v", err)
	}
	// jenis ref berbeda dengan id sama = peristiwa berbeda
	other := wallet.Ref{Type: "test_other", ID: ref.ID}
	if _, err := w.post(other, transfer(a, b, 100)...); err != nil {
		t.Errorf("jenis ref lain: %v", err)
	}
}

func TestPostRollsBackWithCallerTransaction(t *testing.T) {
	w := newWorld(t)
	_, a := w.user("a", 1000)
	_, b := w.user("b", 0)

	boom := errors.New("langkah berikutnya gagal")
	err := w.e.InTx(func(tx pgx.Tx) error {
		if _, err := wallet.Post(w.ctx, tx, w.newRef("test_rb"), transfer(a, b, 400)...); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	if w.balance(a) != 1000 || w.balance(b) != 0 || w.entries(b) != 0 {
		t.Errorf("rollback tidak bersih: a=%d b=%d entri b=%d", w.balance(a), w.balance(b), w.entries(b))
	}
}

// ---- Post: konkurensi -------------------------------------------------------

func TestPostConcurrentNeverOverdraws(t *testing.T) {
	w := newWorld(t)
	_, a := w.user("a", 100)
	_, b := w.user("b", 0)

	const n = 30
	results := make([]error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, results[i] = w.post(w.newRef("test_conc"), transfer(a, b, 10)...)
		}()
	}
	wg.Wait()

	ok := 0
	for _, err := range results {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, wallet.ErrInsufficientFunds):
		default:
			t.Errorf("error tak terduga: %v", err)
		}
	}
	if ok != 10 {
		t.Errorf("%d transfer berhasil, want tepat 10 (saldo 100 / 10)", ok)
	}
	if w.balance(a) != 0 || w.balance(b) != 100 {
		t.Errorf("a=%d b=%d, want 0 dan 100", w.balance(a), w.balance(b))
	}
}

// Transfer yang saling berlawanan (A->B dan B->A) tidak boleh saling buntu.
func TestPostOpposingTransfersDoNotDeadlock(t *testing.T) {
	w := newWorld(t)
	_, a := w.user("a", 1000)
	_, b := w.user("b", 1000)

	const n = 60
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			from, to := a, b
			if i%2 == 1 {
				from, to = b, a
			}
			_, errs[i] = w.post(w.newRef("test_dl"), transfer(from, to, 1)...)
		}()
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("transfer %d gagal: %v (kemungkinan deadlock)", i, err)
		}
	}
	if w.balance(a)+w.balance(b) != 2000 || w.balance(a) != 1000 {
		t.Errorf("a=%d b=%d, want 1000 dan 1000 (30 transfer ke tiap arah saling meniadakan)", w.balance(a), w.balance(b))
	}
}

// Transaksi A menguras dompet tetapi belum commit; Post kedua yang dimulai saat itu harus
// menunggu lalu melihat saldo terbaru dan gagal ErrInsufficientFunds. Tanpa FOR UPDATE ia
// membaca saldo lama, lolos, lalu menimpa saldo (lost update). Tes balapan di atas
// bergantung waktu dan bisa lolos walau kunci hilang.
func TestPostLocksWalletRows(t *testing.T) {
	w := newWorld(t)
	_, a := w.user("a", 100)
	_, b := w.user("b", 0)

	txA, err := w.e.Pool.Begin(w.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = txA.Rollback(w.ctx) }()
	if _, err := wallet.Post(w.ctx, txA, w.newRef("test_lock_a"), transfer(a, b, 100)...); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() {
		_, err := w.post(w.newRef("test_lock_b"), transfer(a, b, 50)...)
		done <- err
	}()

	select {
	case err := <-done:
		t.Fatalf("Post kedua selesai (err=%v) padahal A belum commit; seharusnya menunggu", err)
	case <-time.After(300 * time.Millisecond):
	}
	if err := txA.Commit(w.ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, wallet.ErrInsufficientFunds) {
			t.Errorf("err = %v, want ErrInsufficientFunds (saldo sudah dikuras A)", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Post kedua tidak selesai setelah A commit")
	}
	if w.balance(a) != 0 || w.balance(b) != 100 {
		t.Errorf("a=%d b=%d, want 0 dan 100", w.balance(a), w.balance(b))
	}
}

// ---- dompet pengguna dan dompet sistem --------------------------------------

func TestEnsureUserWalletConcurrent(t *testing.T) {
	w := newWorld(t)
	userID := w.e.NewUser("a")

	const n = 12
	ids := make([]string, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ids[i], errs[i] = wallet.EnsureUserWallet(w.ctx, w.e.Pool, userID)
		}()
	}
	wg.Wait()

	for i := range ids {
		if errs[i] != nil {
			t.Fatalf("goroutine %d: %v", i, errs[i])
		}
		if ids[i] != ids[0] {
			t.Errorf("ID berbeda: %s vs %s", ids[i], ids[0])
		}
	}
	if n := w.e.QueryInt(`SELECT count(*) FROM wallets WHERE user_id = $1`, userID); n != 1 {
		t.Errorf("jumlah dompet = %d, want 1", n)
	}
}

func TestSystemWallets(t *testing.T) {
	w := newWorld(t)
	seen := map[string]bool{}
	for _, typ := range []string{wallet.TypeEscrow, wallet.TypePlatform, wallet.TypeHolding, wallet.TypeExternal} {
		id, err := wallet.SystemWallet(w.ctx, w.e.Pool, typ)
		if err != nil || id == "" {
			t.Fatalf("%s: %q, %v", typ, id, err)
		}
		if seen[id] {
			t.Errorf("dompet %s memakai ID yang sama dengan dompet sistem lain", typ)
		}
		seen[id] = true
	}
	if _, err := wallet.SystemWallet(w.ctx, w.e.Pool, wallet.TypeUser); !errors.Is(err, wallet.ErrWalletNotFound) {
		t.Errorf("jenis user: err = %v, want ErrWalletNotFound", err)
	}
	if _, err := wallet.SystemWallet(w.ctx, w.e.Pool, "tidak-ada"); !errors.Is(err, wallet.ErrWalletNotFound) {
		t.Errorf("jenis asing: err = %v, want ErrWalletNotFound", err)
	}
}

// ---- perlindungan di level database -----------------------------------------

func TestDatabaseProtections(t *testing.T) {
	w := newWorld(t)
	userID, uw := w.user("a", 500)
	_, b := w.user("b", 0)
	if _, err := w.post(w.newRef("test_prot"), transfer(uw, b, 10)...); err != nil {
		t.Fatal(err)
	}
	execErr := func(sql string, args ...any) error {
		_, err := w.e.Pool.Exec(w.ctx, sql, args...)
		return err
	}

	t.Run("ledger tidak bisa diubah", func(t *testing.T) {
		if err := execErr(`UPDATE ledger_entries SET amount = amount + 1 WHERE wallet_id = $1`, uw); err == nil || !strings.Contains(err.Error(), "append-only") {
			t.Errorf("UPDATE: err = %v, want penolakan append-only", err)
		}
		if err := execErr(`UPDATE ledger_entries SET description = 'x' WHERE wallet_id = $1`, uw); err == nil {
			t.Error("UPDATE kolom apa pun harus ditolak")
		}
	})
	t.Run("ledger tidak bisa dihapus", func(t *testing.T) {
		if err := execErr(`DELETE FROM ledger_entries WHERE wallet_id = $1`, uw); err == nil || !strings.Contains(err.Error(), "append-only") {
			t.Errorf("DELETE: err = %v, want penolakan append-only", err)
		}
		if n := w.entries(uw); n != 2 {
			t.Errorf("baris ledger = %d, want 2 (tidak boleh berkurang)", n)
		}
	})
	t.Run("trigger anti-TRUNCATE terpasang", func(t *testing.T) {
		// tidak menjalankan TRUNCATE sungguhan: jika trigger hilang, ledger asli ikut terhapus.
		if n := w.e.QueryInt(`SELECT count(*) FROM pg_trigger WHERE tgname IN
		        ('trg_ledger_immutable','trg_ledger_no_truncate','trg_ledger_balanced') AND NOT tgisinternal`); n != 3 {
			t.Errorf("trigger ledger terpasang = %d, want 3", n)
		}
	})
	t.Run("entry_group tidak seimbang ditolak saat commit", func(t *testing.T) {
		err := w.e.InTx(func(tx pgx.Tx) error {
			_, err := tx.Exec(w.ctx,
				`INSERT INTO ledger_entries (entry_group, wallet_id, amount, balance_after, ref_type, ref_id)
				 VALUES (gen_random_uuid(), $1, 5, 5, 'test_bad', gen_random_uuid())`, uw)
			return err // INSERT sendiri lolos; pengecekan terjadi saat commit
		})
		if err == nil || !strings.Contains(err.Error(), "tidak seimbang") {
			t.Errorf("err = %v, want 'tidak seimbang'", err)
		}
	})
	t.Run("saldo pengguna tidak boleh negatif", func(t *testing.T) {
		if err := execErr(`UPDATE wallets SET balance = -1 WHERE id = $1`, uw); err == nil {
			t.Error("saldo negatif dompet pengguna harus ditolak CHECK")
		}
	})
	t.Run("dompet sistem tunggal", func(t *testing.T) {
		if err := execErr(`INSERT INTO wallets (type) VALUES ('escrow')`); err == nil {
			t.Error("dompet escrow kedua harus ditolak")
		}
		if err := execErr(`INSERT INTO wallets (type) VALUES ('external')`); err == nil {
			t.Error("dompet external kedua harus ditolak")
		}
	})
	t.Run("aturan jenis dompet", func(t *testing.T) {
		if err := execErr(`INSERT INTO wallets (type) VALUES ('user')`); err == nil {
			t.Error("dompet user tanpa user_id harus ditolak")
		}
		if err := execErr(`INSERT INTO wallets (user_id, type) VALUES ($1, 'escrow')`, w.e.NewUser("c")); err == nil {
			t.Error("dompet sistem dengan user_id harus ditolak")
		}
		if err := execErr(`INSERT INTO wallets (user_id, type) VALUES ($1, 'user')`, userID); err == nil {
			t.Error("dompet kedua untuk pengguna yang sama harus ditolak")
		}
		if err := execErr(`INSERT INTO wallets (type) VALUES ('bebas')`); err == nil {
			t.Error("jenis dompet asing harus ditolak")
		}
	})
	t.Run("nominal ledger tidak boleh nol", func(t *testing.T) {
		err := execErr(`INSERT INTO ledger_entries (entry_group, wallet_id, amount, balance_after, ref_type, ref_id)
		                VALUES (gen_random_uuid(), $1, 0, 0, 'test_zero', gen_random_uuid())`, uw)
		if err == nil {
			t.Error("amount 0 harus ditolak CHECK")
		}
	})
	t.Run("referensi bisnis unik per dompet", func(t *testing.T) {
		ref := w.newRef("test_dupref")
		if _, err := w.post(ref, transfer(uw, b, 1)...); err != nil {
			t.Fatal(err)
		}
		err := execErr(`INSERT INTO ledger_entries (entry_group, wallet_id, amount, balance_after, ref_type, ref_id)
		                VALUES (gen_random_uuid(), $1, 1, 1, $2, $3)`, uw, ref.Type, ref.ID)
		if err == nil {
			t.Error("ref yang sama pada dompet yang sama harus ditolak unique index")
		}
	})
}

// ---- rekonsiliasi -----------------------------------------------------------

func TestReconcileCleanSystem(t *testing.T) {
	w := newWorld(t)
	_, a := w.user("a", 1000)
	_, b := w.user("b", 0)
	for i := 0; i < 5; i++ {
		if _, err := w.post(w.newRef("test_rec"), transfer(a, b, 100)...); err != nil {
			t.Fatal(err)
		}
	}
	rep, err := wallet.Reconcile(w.ctx, w.e.Pool)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Ok || len(rep.Problems) != 0 {
		t.Errorf("sistem sehat harus bersih, got problems: %v", rep.Problems)
	}
	if rep.Wallets < 6 || rep.Entries < 7 {
		t.Errorf("jumlah yang diperiksa terlalu kecil: wallets=%d entries=%d", rep.Wallets, rep.Entries)
	}
	if rep.Problems == nil {
		t.Error("Problems harus [] bukan null di JSON")
	}
}

// Setiap pelanggaran invarian harus terdeteksi. Pengerusakan dilakukan di dalam transaksi
// yang SELALU di-rollback; trigger dilewati dengan session_replication_role=replica.
func TestReconcileDetectsCorruption(t *testing.T) {
	w := newWorld(t)
	_, a := w.user("a", 1000)
	_, b := w.user("b", 0)
	if _, err := w.post(w.newRef("test_corrupt"), transfer(a, b, 100)...); err != nil {
		t.Fatal(err)
	}

	corrupt := func(t *testing.T, sqls ...string) *wallet.Report {
		t.Helper()
		tx, err := w.e.Pool.Begin(w.ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback(w.ctx) }()
		if _, err := tx.Exec(w.ctx, `SET LOCAL session_replication_role = replica`); err != nil {
			t.Skipf("butuh superuser: %v", err)
		}
		for _, q := range sqls {
			if _, err := tx.Exec(w.ctx, q); err != nil {
				t.Fatalf("%s: %v", q, err)
			}
		}
		rep, err := wallet.Reconcile(w.ctx, tx)
		if err != nil {
			t.Fatal(err)
		}
		return rep
	}
	has := func(rep *wallet.Report, substr string) bool {
		for _, p := range rep.Problems {
			if strings.Contains(p, substr) {
				return true
			}
		}
		return false
	}

	t.Run("saldo dompet diubah langsung", func(t *testing.T) {
		rep := corrupt(t, fmt.Sprintf(`UPDATE wallets SET balance = balance + 7 WHERE id = '%s'`, a))
		if rep.Ok || !has(rep, a) || !has(rep, "tidak sama dengan jumlah ledger") {
			t.Errorf("saldo diubah tidak terdeteksi: %+v", rep)
		}
		if !has(rep, "jumlah seluruh saldo") {
			t.Errorf("penyimpangan total saldo tidak terdeteksi: %+v", rep.Problems)
		}
	})
	t.Run("uang muncul dari ketiadaan (baris ledger tak seimbang)", func(t *testing.T) {
		rep := corrupt(t, fmt.Sprintf(
			`INSERT INTO ledger_entries (entry_group, wallet_id, amount, balance_after, ref_type, ref_id)
			 VALUES (gen_random_uuid(), '%s', 50, 0, 'test_phantom', gen_random_uuid())`, a))
		if rep.Ok || !has(rep, "tidak seimbang") {
			t.Errorf("entry_group tak seimbang tidak terdeteksi: %+v", rep)
		}
	})
	t.Run("balance_after dipalsukan", func(t *testing.T) {
		rep := corrupt(t, fmt.Sprintf(`UPDATE ledger_entries SET balance_after = balance_after + 1
		                                WHERE wallet_id = '%s' AND ref_type = 'test_corrupt'`, a))
		if rep.Ok || !has(rep, "balance_after") {
			t.Errorf("balance_after palsu tidak terdeteksi: %+v", rep)
		}
	})
	t.Run("baris ledger dihapus diam-diam", func(t *testing.T) {
		rep := corrupt(t, fmt.Sprintf(`DELETE FROM ledger_entries WHERE wallet_id = '%s' AND ref_type = 'test_corrupt'`, b))
		if rep.Ok {
			t.Errorf("baris hilang tidak terdeteksi: %+v", rep)
		}
	})
	t.Run("kontrol: tanpa perusakan tetap bersih", func(t *testing.T) {
		if rep := corrupt(t, `SELECT 1`); !rep.Ok {
			t.Errorf("seharusnya bersih: %v", rep.Problems)
		}
	})
}

// ---- riwayat ----------------------------------------------------------------

func TestLedgerPagination(t *testing.T) {
	w := newWorld(t)
	userA, a := w.user("a", 1000) // entri #1: pendanaan
	userB, b := w.user("b", 0)
	for i := 0; i < 6; i++ { // entri #2..#7 milik A
		if _, err := w.post(w.newRef("test_page"), transfer(a, b, 10)...); err != nil {
			t.Fatal(err)
		}
	}
	svc := wallet.NewService(wallet.NewRepository(w.e.Pool))

	var all []wallet.Entry
	var before int64
	pages := 0
	for {
		page, err := svc.Ledger(w.ctx, userA, 3, before)
		if err != nil {
			t.Fatal(err)
		}
		pages++
		if len(page.Entries) > 3 {
			t.Fatalf("halaman berisi %d entri, maksimal 3", len(page.Entries))
		}
		all = append(all, page.Entries...)
		if page.NextBefore == nil {
			break
		}
		before = *page.NextBefore
		if pages > 10 {
			t.Fatal("paginasi tidak berhenti")
		}
	}
	if len(all) != 7 || pages != 3 {
		t.Fatalf("entri = %d dalam %d halaman, want 7 dalam 3", len(all), pages)
	}
	for i := 1; i < len(all); i++ {
		if all[i-1].ID <= all[i].ID {
			t.Errorf("harus terbaru lebih dulu & tanpa duplikat: %d lalu %d", all[i-1].ID, all[i].ID)
		}
	}
	if all[0].Amount != -10 || all[0].BalanceAfter != 940 || all[len(all)-1].Amount != 1000 {
		t.Errorf("isi tidak sesuai: terbaru=%+v tertua=%+v", all[0], all[len(all)-1])
	}

	// tepat satu halaman penuh: tidak ada halaman berikutnya
	page, err := svc.Ledger(w.ctx, userA, 7, 0)
	if err != nil || len(page.Entries) != 7 || page.NextBefore != nil {
		t.Errorf("limit = jumlah entri: len=%d next=%v err=%v", len(page.Entries), page.NextBefore, err)
	}

	// pengguna hanya melihat mutasi dompetnya sendiri
	pb, _ := svc.Ledger(w.ctx, userB, 50, 0)
	if len(pb.Entries) != 6 {
		t.Errorf("B melihat %d entri, want 6 (hanya transfer masuk)", len(pb.Entries))
	}
	for _, e := range pb.Entries {
		if e.Amount != 10 {
			t.Errorf("B melihat entri milik A: %+v", e)
		}
	}
	// pengguna tanpa riwayat -> daftar kosong, dompet dibuat otomatis
	empty, err := svc.Ledger(w.ctx, w.e.NewUser("kosong"), 5, 0)
	if err != nil || empty.Entries == nil || len(empty.Entries) != 0 || empty.NextBefore != nil {
		t.Errorf("riwayat kosong: %+v, %v", empty, err)
	}
}

func TestWalletByUserCreatesLazily(t *testing.T) {
	w := newWorld(t)
	userID := w.e.NewUser("baru")
	if n := w.e.QueryInt(`SELECT count(*) FROM wallets WHERE user_id = $1`, userID); n != 0 {
		t.Fatalf("dompet sudah ada sebelum diminta: %d", n)
	}
	got, err := wallet.NewService(wallet.NewRepository(w.e.Pool)).Wallet(w.ctx, userID)
	if err != nil || got.Balance != 0 || got.ID == "" {
		t.Fatalf("Wallet = %+v, %v", got, err)
	}
	again, _ := wallet.NewService(wallet.NewRepository(w.e.Pool)).Wallet(w.ctx, userID)
	if again.ID != got.ID {
		t.Error("pemanggilan kedua harus mengembalikan dompet yang sama")
	}
}
