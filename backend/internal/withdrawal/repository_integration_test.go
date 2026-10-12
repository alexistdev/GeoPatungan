/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev/geopatungan
 * Email   : AlexistDev@gmail.com
 */

package withdrawal

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alexistdev/geopatungan/internal/testutil"
	"github.com/alexistdev/geopatungan/internal/wallet"
)

// Tes ini butuh database Postgres dengan semua migrasi; dilewati jika TEST_DATABASE_URL kosong.
//
// Perhatian: jangan memanggil helper env (e.QueryInt, e.Balance, dst.) dari banyak goroutine yang
// masing-masing memegang transaksi; itu meminjam koneksi kedua dan bisa membuntukan pool.

type fx struct {
	e    *testutil.Env
	repo *Repository
	ctx  context.Context
}

func newFx(t *testing.T) *fx {
	t.Helper()
	e := testutil.NewEnv(t)
	return &fx{e: e, repo: NewRepository(e.Pool), ctx: context.Background()}
}

func in(amount int64) Input {
	return Input{Amount: amount, BankName: "BCA", AccountNumber: "1234567890", AccountHolder: "Budi Santoso", Note: "uji"}
}

func (f *fx) mk(userID string, amount int64) (*Withdrawal, error) {
	return f.repo.Create(f.ctx, userID, in(amount), MaxOpenPerUser)
}

func (f *fx) mustMk(userID string, amount int64) *Withdrawal {
	f.e.T.Helper()
	w, err := f.mk(userID, amount)
	if err != nil {
		f.e.T.Fatalf("Create(%d): %v", amount, err)
	}
	return w
}

func (f *fx) holding() int64 {
	return int64(f.e.QueryInt(`SELECT balance FROM wallets WHERE type = 'holding'`))
}

func (f *fx) external() int64 {
	return int64(f.e.QueryInt(`SELECT balance FROM wallets WHERE type = 'external'`))
}

func (f *fx) status(id string) string {
	return f.e.QueryString(`SELECT status FROM withdrawals WHERE id = $1`, id)
}

func (f *fx) settleEvents(id string) int {
	return f.e.QueryInt(`SELECT count(*) FROM ledger_entries l JOIN wallets w ON w.id = l.wallet_id
	                      WHERE w.type = 'holding' AND l.ref_id = $1
	                        AND l.ref_type IN ('withdrawal_complete', 'withdrawal_reject', 'withdrawal_cancel')`, id)
}

func (f *fx) requireReconciled() {
	f.e.T.Helper()
	rep, err := wallet.Reconcile(f.ctx, f.e.Pool)
	if err != nil || !rep.Ok {
		f.e.T.Fatalf("rekonsiliasi gagal: %v %+v", err, rep)
	}
}

// funded membuat pengguna ber-saldo.
func (f *fx) funded(name string, balance int64) string {
	f.e.T.Helper()
	u := f.e.NewUser(name)
	if balance > 0 {
		f.e.FundUser(u, balance)
	}
	return u
}

// ---- Create ----------------------------------------------------------------

func TestCreateMovesFundsToHolding(t *testing.T) {
	f := newFx(t)
	user := f.funded("a", 500_000)
	holding0 := f.holding()

	w, err := f.repo.Create(f.ctx, user, in(200_000), MaxOpenPerUser)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if w.ID == "" || w.UserID != user || w.Amount != 200_000 || w.Status != StatusPending ||
		w.BankName != "BCA" || w.AccountNumber != "1234567890" || w.AccountHolder != "Budi Santoso" || w.Note != "uji" {
		t.Errorf("hasil = %+v", w)
	}
	if w.TransferReference != "" || w.ReviewedAt != nil || w.CreatedAt.IsZero() {
		t.Errorf("kolom lanjutan: %+v", w)
	}

	// dana langsung keluar dari saldo pengguna (tidak bisa dipakai dua kali) dan tertahan di holding
	if f.e.Balance(user) != 300_000 || f.holding()-holding0 != 200_000 {
		t.Errorf("saldo = %d, holding +%d; want 300000 dan +200000", f.e.Balance(user), f.holding()-holding0)
	}
	entries, err := wallet.NewRepository(f.e.Pool).Ledger(f.ctx, user, 5, 0)
	if err != nil || len(entries) < 1 {
		t.Fatalf("ledger = %v, %v", entries, err)
	}
	if e := entries[0]; e.Amount != -200_000 || e.RefType != "withdrawal_request" || e.RefID != w.ID || e.BalanceAfter != 300_000 {
		t.Errorf("entri ledger = %+v", e)
	}
	f.requireReconciled()
}

func TestCreateInsufficientFundsLeavesNoTrace(t *testing.T) {
	f := newFx(t)
	user := f.funded("a", 100_000)
	holding0 := f.holding()

	_, err := f.mk(user, 100_001)
	var short *wallet.InsufficientFundsError
	if !errors.As(err, &short) || !errors.Is(err, wallet.ErrInsufficientFunds) {
		t.Fatalf("err = %v, want InsufficientFundsError", err)
	}
	if short.Required != 100_001 || short.Balance != 100_000 {
		t.Errorf("required=%d balance=%d, want 100001 dan 100000", short.Required, short.Balance)
	}
	if n := f.e.QueryInt(`SELECT count(*) FROM withdrawals WHERE user_id = $1`, user); n != 0 {
		t.Errorf("baris permintaan tersisa = %d, want 0 (transaksi harus batal utuh)", n)
	}
	if f.e.Balance(user) != 100_000 || f.holding() != holding0 {
		t.Error("saldo/holding berubah padahal permintaan gagal")
	}

	// tepat sebesar saldo diperbolehkan -> saldo 0
	if _, err := f.mk(user, 100_000); err != nil {
		t.Fatalf("menarik seluruh saldo: %v", err)
	}
	if f.e.Balance(user) != 0 {
		t.Errorf("saldo = %d, want 0", f.e.Balance(user))
	}
	// pengguna tanpa dompet sama sekali
	if _, err := f.mk(f.e.NewUser("kosong"), 50_000); !errors.Is(err, wallet.ErrInsufficientFunds) {
		t.Errorf("tanpa dompet: err = %v, want ErrInsufficientFunds", err)
	}
}

func TestCreateEnforcesMaxOpenPerUser(t *testing.T) {
	f := newFx(t)
	user := f.funded("a", 1_000_000)

	var ids []string
	for i := 0; i < 3; i++ {
		ids = append(ids, f.mustMk(user, 50_000).ID)
	}
	if _, err := f.mk(user, 50_000); !errors.Is(err, ErrTooManyOpen) {
		t.Fatalf("permintaan ke-4: err = %v, want ErrTooManyOpen", err)
	}
	if f.e.Balance(user) != 850_000 {
		t.Errorf("saldo = %d, want 850000 (permintaan ke-4 tidak boleh memotong)", f.e.Balance(user))
	}
	if _, err := f.repo.Cancel(f.ctx, user, ids[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := f.mk(user, 50_000); err != nil {
		t.Errorf("setelah pembatalan seharusnya bisa lagi: %v", err)
	}
	if _, err := f.mk(f.funded("b", 100_000), 50_000); err != nil {
		t.Errorf("pengguna lain: %v", err)
	}
}

func TestCreateConcurrentRespectsMaxOpen(t *testing.T) {
	f := newFx(t)
	user := f.funded("a", 1_000_000)
	holding0 := f.holding()

	const n = 12
	results := make([]error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, results[i] = f.mk(user, 10_000)
		}()
	}
	wg.Wait()

	ok := 0
	for _, err := range results {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, ErrTooManyOpen):
		default:
			t.Errorf("error tak terduga: %v", err)
		}
	}
	if ok != MaxOpenPerUser {
		t.Errorf("%d permintaan berhasil, want tepat %d", ok, MaxOpenPerUser)
	}
	if f.e.Balance(user) != 970_000 || f.holding()-holding0 != 30_000 {
		t.Errorf("saldo = %d, holding +%d; want 970000 dan +30000", f.e.Balance(user), f.holding()-holding0)
	}
}

// Beberapa permintaan serentak yang masing-masing menarik seluruh saldo: hanya satu yang boleh
// berhasil (double-spend). maxOpen sengaja dilonggarkan supaya yang diuji adalah pengecekan saldo.
func TestCreateConcurrentCannotDoubleSpend(t *testing.T) {
	f := newFx(t)
	user := f.funded("a", 100_000)
	holding0 := f.holding()

	const n = 8
	results := make([]error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, results[i] = f.repo.Create(f.ctx, user, in(100_000), 100)
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
	if ok != 1 {
		t.Errorf("%d penarikan Rp100.000 berhasil dari saldo Rp100.000, want tepat 1", ok)
	}
	if f.e.Balance(user) != 0 || f.holding()-holding0 != 100_000 {
		t.Errorf("saldo = %d, holding +%d; want 0 dan +100000", f.e.Balance(user), f.holding()-holding0)
	}
	if n := f.e.QueryInt(`SELECT count(*) FROM withdrawals WHERE user_id = $1`, user); n != 1 {
		t.Errorf("baris permintaan = %d, want 1 (yang gagal tidak boleh meninggalkan baris)", n)
	}
	f.requireReconciled()
}

// ---- Cancel ----------------------------------------------------------------

func TestCancelReturnsFunds(t *testing.T) {
	f := newFx(t)
	user, other := f.funded("a", 500_000), f.funded("b", 500_000)
	holding0 := f.holding()
	w := f.mustMk(user, 200_000)

	if _, err := f.repo.Cancel(f.ctx, other, w.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("milik orang lain: err = %v, want ErrNotFound", err)
	}
	if _, err := f.repo.Cancel(f.ctx, user, "00000000-0000-0000-0000-000000000000"); !errors.Is(err, ErrNotFound) {
		t.Errorf("id tidak ada: err = %v, want ErrNotFound", err)
	}
	if f.status(w.ID) != StatusPending || f.e.Balance(user) != 300_000 {
		t.Fatal("percobaan yang ditolak tidak boleh mengubah apa pun")
	}

	got, err := f.repo.Cancel(f.ctx, user, w.ID)
	if err != nil || got.Status != StatusCancelled {
		t.Fatalf("Cancel: %+v, %v", got, err)
	}
	if f.e.Balance(user) != 500_000 || f.holding() != holding0 {
		t.Errorf("saldo = %d, holding %+d; want 500000 dan 0", f.e.Balance(user), f.holding()-holding0)
	}
	entries, _ := wallet.NewRepository(f.e.Pool).Ledger(f.ctx, user, 1, 0)
	if len(entries) == 0 || entries[0].Amount != 200_000 || entries[0].RefType != "withdrawal_cancel" || entries[0].RefID != w.ID {
		t.Errorf("entri pembatalan = %+v", entries)
	}

	if _, err := f.repo.Cancel(f.ctx, user, w.ID); !errors.Is(err, ErrInvalidState) {
		t.Errorf("batal ulang: err = %v, want ErrInvalidState", err)
	}
	if f.e.Balance(user) != 500_000 {
		t.Errorf("saldo = %d; pembatalan ganda tidak boleh mengembalikan dua kali", f.e.Balance(user))
	}
	f.requireReconciled()
}

// ---- Complete --------------------------------------------------------------

func TestCompleteSendsFundsOutOfSystem(t *testing.T) {
	f := newFx(t)
	user, admin := f.funded("a", 500_000), f.e.NewAdmin("admin")
	w := f.mustMk(user, 200_000)
	holding0, external0 := f.holding(), f.external()

	got, err := f.repo.Complete(f.ctx, admin, w.ID, "TRX-20261012-001", "ditransfer via BCA")
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if got.Status != StatusCompleted || got.TransferReference != "TRX-20261012-001" || got.ReviewNote != "ditransfer via BCA" || got.ReviewedAt == nil {
		t.Errorf("hasil = %+v", got)
	}
	// holding berkurang dan external bertambah sebesar nominal; saldo pengguna tidak berubah lagi
	if f.holding()-holding0 != -200_000 || f.external()-external0 != 200_000 {
		t.Errorf("holding %+d external %+d; want -200000 dan +200000", f.holding()-holding0, f.external()-external0)
	}
	if f.e.Balance(user) != 300_000 {
		t.Errorf("saldo = %d, want 300000", f.e.Balance(user))
	}
	if n := f.e.QueryInt(`SELECT count(*) FROM audit_logs WHERE action = 'withdrawal.complete' AND entity_id = $1 AND actor_id = $2
	                        AND meta->>'transfer_reference' = 'TRX-20261012-001'`, w.ID, admin); n != 1 {
		t.Errorf("audit complete = %d, want 1", n)
	}
	if n := f.e.QueryInt(`SELECT count(*) FROM withdrawals WHERE id = $1 AND reviewed_by = $2`, w.ID, admin); n != 1 {
		t.Error("reviewed_by harus mencatat admin")
	}

	// final
	if _, err := f.repo.Complete(f.ctx, admin, w.ID, "TRX-2", ""); !errors.Is(err, ErrInvalidState) {
		t.Errorf("complete ulang: err = %v", err)
	}
	if _, err := f.repo.Reject(f.ctx, admin, w.ID, "alasan"); !errors.Is(err, ErrInvalidState) {
		t.Errorf("reject setelah complete: err = %v", err)
	}
	if _, err := f.repo.Cancel(f.ctx, user, w.ID); !errors.Is(err, ErrInvalidState) {
		t.Errorf("cancel setelah complete: err = %v", err)
	}
	if f.holding()-holding0 != -200_000 || f.external()-external0 != 200_000 || f.e.Balance(user) != 300_000 {
		t.Error("percobaan setelah selesai tidak boleh mengubah uang")
	}
	if f.settleEvents(w.ID) != 1 {
		t.Errorf("peristiwa penyelesaian = %d, want 1", f.settleEvents(w.ID))
	}
	f.requireReconciled()
}

func TestReviewRejectionsLeaveNoTrace(t *testing.T) {
	f := newFx(t)
	user, admin := f.funded("a", 500_000), f.e.NewAdmin("admin")

	t.Run("admin memproses penarikan sendiri", func(t *testing.T) {
		selfAdmin := f.e.NewAdmin("self")
		f.e.FundUser(selfAdmin, 300_000)
		w := f.mustMk(selfAdmin, 100_000)
		if _, err := f.repo.Complete(f.ctx, selfAdmin, w.ID, "TRX-1", ""); !errors.Is(err, ErrSelfReview) {
			t.Errorf("complete: err = %v, want ErrSelfReview", err)
		}
		if _, err := f.repo.Reject(f.ctx, selfAdmin, w.ID, "alasan"); !errors.Is(err, ErrSelfReview) {
			t.Errorf("reject: err = %v, want ErrSelfReview", err)
		}
		if f.status(w.ID) != StatusPending || f.e.Balance(selfAdmin) != 200_000 {
			t.Errorf("status = %s saldo = %d; tidak boleh berubah", f.status(w.ID), f.e.Balance(selfAdmin))
		}
		// pemilik tetap boleh membatalkan permintaannya sendiri (itu bukan peninjauan)
		if _, err := f.repo.Cancel(f.ctx, selfAdmin, w.ID); err != nil {
			t.Errorf("admin membatalkan permintaannya sendiri: %v", err)
		}
	})
	t.Run("id tidak ada", func(t *testing.T) {
		zero := "00000000-0000-0000-0000-000000000000"
		if _, err := f.repo.Complete(f.ctx, admin, zero, "TRX-1", ""); !errors.Is(err, ErrNotFound) {
			t.Errorf("complete: %v", err)
		}
		if _, err := f.repo.Reject(f.ctx, admin, zero, "alasan"); !errors.Is(err, ErrNotFound) {
			t.Errorf("reject: %v", err)
		}
	})
	t.Run("sudah dibatalkan", func(t *testing.T) {
		w := f.mustMk(user, 50_000)
		if _, err := f.repo.Cancel(f.ctx, user, w.ID); err != nil {
			t.Fatal(err)
		}
		before := f.e.Balance(user)
		if _, err := f.repo.Complete(f.ctx, admin, w.ID, "TRX-1", ""); !errors.Is(err, ErrInvalidState) {
			t.Errorf("complete: %v", err)
		}
		if _, err := f.repo.Reject(f.ctx, admin, w.ID, "alasan"); !errors.Is(err, ErrInvalidState) {
			t.Errorf("reject: %v", err)
		}
		if f.e.Balance(user) != before {
			t.Errorf("saldo berubah dari %d menjadi %d", before, f.e.Balance(user))
		}
	})
}

// ---- Reject ----------------------------------------------------------------

func TestRejectReturnsFunds(t *testing.T) {
	f := newFx(t)
	user, admin := f.funded("a", 500_000), f.e.NewAdmin("admin")
	holding0, external0 := f.holding(), f.external()
	w := f.mustMk(user, 200_000)

	got, err := f.repo.Reject(f.ctx, admin, w.ID, "nama rekening tidak cocok dengan nama akun")
	if err != nil {
		t.Fatalf("Reject: %v", err)
	}
	if got.Status != StatusRejected || got.ReviewNote != "nama rekening tidak cocok dengan nama akun" || got.ReviewedAt == nil || got.TransferReference != "" {
		t.Errorf("hasil = %+v", got)
	}
	if f.e.Balance(user) != 500_000 || f.holding() != holding0 || f.external() != external0 {
		t.Errorf("saldo = %d holding %+d external %+d; dana harus kembali utuh, tidak keluar sistem",
			f.e.Balance(user), f.holding()-holding0, f.external()-external0)
	}
	entries, _ := wallet.NewRepository(f.e.Pool).Ledger(f.ctx, user, 1, 0)
	if len(entries) == 0 || entries[0].Amount != 200_000 || entries[0].RefType != "withdrawal_reject" {
		t.Errorf("entri = %+v", entries)
	}
	if n := f.e.QueryInt(`SELECT count(*) FROM audit_logs WHERE action = 'withdrawal.reject' AND entity_id = $1 AND actor_id = $2`, w.ID, admin); n != 1 {
		t.Errorf("audit reject = %d, want 1", n)
	}
	if _, err := f.repo.Reject(f.ctx, admin, w.ID, "lagi"); !errors.Is(err, ErrInvalidState) {
		t.Errorf("reject ulang: %v", err)
	}
	// tidak lagi dihitung terbuka
	for i := 0; i < 3; i++ {
		f.mustMk(user, 10_000)
	}
	f.requireReconciled()
}

// ---- konkurensi ------------------------------------------------------------

// Admin menyelesaikan (belum commit); pembatalan oleh pengguna yang dimulai saat itu harus menunggu
// lalu gagal. Tanpa FOR UPDATE, pembatalan membaca status lama (pending), mengembalikan dana, dan
// menimpa penyelesaian admin: uang sudah dikirim ke bank tetapi saldo pengguna juga dikembalikan.
func TestCancelLocksRequestAgainstConcurrentComplete(t *testing.T) {
	f := newFx(t)
	user, admin := f.funded("a", 500_000), f.e.NewAdmin("admin")
	w := f.mustMk(user, 200_000)

	txA, err := f.e.Pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = txA.Rollback(f.ctx) }()
	if _, err := txA.Exec(f.ctx, `SELECT 1 FROM withdrawals WHERE id = $1 FOR UPDATE`, w.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := txA.Exec(f.ctx,
		`UPDATE withdrawals SET status = 'completed', transfer_reference = 'TRX-A', reviewed_by = $2, reviewed_at = now() WHERE id = $1`,
		w.ID, admin); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() { _, err := f.repo.Cancel(f.ctx, user, w.ID); done <- err }()

	select {
	case err := <-done:
		t.Fatalf("Cancel selesai (err=%v) padahal A belum commit; seharusnya menunggu", err)
	case <-time.After(300 * time.Millisecond):
	}
	if err := txA.Commit(f.ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, ErrInvalidState) {
			t.Errorf("err = %v, want ErrInvalidState (sudah diselesaikan A)", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Cancel tidak selesai setelah A commit")
	}
	if f.status(w.ID) != StatusCompleted || f.e.Balance(user) != 300_000 {
		t.Errorf("status = %s saldo = %d, want completed dan 300000 (dana tidak boleh kembali)", f.status(w.ID), f.e.Balance(user))
	}
}

// Complete, Reject, dan Cancel serentak atas satu permintaan: tepat satu yang menang dan dana
// diselesaikan sekali (ke bank, atau kembali ke pengguna), tidak pernah keduanya.
func TestConcurrentSettlementsExactlyOneWins(t *testing.T) {
	f := newFx(t)
	user := f.funded("a", 500_000)
	admins := []string{f.e.NewAdmin("a1"), f.e.NewAdmin("a2"), f.e.NewAdmin("a3")}
	holding0 := f.holding() // sebelum permintaan dibuat
	w := f.mustMk(user, 200_000)

	const n = 9
	results := make([]error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			switch i % 3 {
			case 0:
				_, results[i] = f.repo.Complete(f.ctx, admins[i%3], w.ID, "TRX-X", "")
			case 1:
				_, results[i] = f.repo.Reject(f.ctx, admins[i%3], w.ID, "alasan")
			default:
				_, results[i] = f.repo.Cancel(f.ctx, user, w.ID)
			}
		}()
	}
	wg.Wait()

	ok := 0
	for _, err := range results {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, ErrInvalidState):
		default:
			t.Errorf("error tak terduga: %v", err)
		}
	}
	if ok != 1 {
		t.Fatalf("%d penyelesaian berhasil, want tepat 1", ok)
	}
	if f.settleEvents(w.ID) != 1 {
		t.Errorf("peristiwa penyelesaian di ledger = %d, want 1", f.settleEvents(w.ID))
	}
	if f.holding() != holding0 {
		t.Errorf("holding %+d; dana harus sudah keluar dari holding sepenuhnya", f.holding()-holding0)
	}
	switch f.status(w.ID) {
	case StatusCompleted:
		if f.e.Balance(user) != 300_000 {
			t.Errorf("completed: saldo = %d, want 300000", f.e.Balance(user))
		}
	case StatusRejected, StatusCancelled:
		if f.e.Balance(user) != 500_000 {
			t.Errorf("%s: saldo = %d, want 500000", f.status(w.ID), f.e.Balance(user))
		}
	default:
		t.Errorf("status = %s", f.status(w.ID))
	}
	f.requireReconciled()
}

// ---- daftar ----------------------------------------------------------------

func TestLists(t *testing.T) {
	f := newFx(t)
	admin := f.e.NewAdmin("admin")
	u1, u2 := f.funded("u1", 1_000_000), f.funded("u2", 1_000_000)

	p := f.mustMk(u1, 60_000)
	time.Sleep(5 * time.Millisecond)
	c := f.mustMk(u2, 70_000)
	time.Sleep(5 * time.Millisecond)
	d := f.mustMk(u1, 80_000)
	if _, err := f.repo.Complete(f.ctx, admin, d.ID, "TRX-D", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := f.repo.Cancel(f.ctx, u2, c.ID); err != nil {
		t.Fatal(err)
	}

	mine := func(status string) []AdminWithdrawal {
		t.Helper()
		all, err := f.repo.ListAdmin(f.ctx, status, 200)
		if err != nil {
			t.Fatal(err)
		}
		var out []AdminWithdrawal
		for _, x := range all {
			if strings.Contains(x.UserEmail, f.e.Prefix) {
				out = append(out, x)
			}
		}
		return out
	}
	ids := func(l []AdminWithdrawal) string {
		var s []string
		for _, x := range l {
			s = append(s, x.ID)
		}
		return strings.Join(s, ",")
	}

	if got := mine(StatusPending); ids(got) != p.ID {
		t.Errorf("pending = %v, want hanya yang pertama", ids(got))
	}
	if got := mine("all"); len(got) != 3 || got[0].ID != d.ID {
		t.Errorf("all = %v, want 3 baris terbaru dulu", ids(got))
	}
	if got := mine(StatusCompleted); ids(got) != d.ID {
		t.Errorf("completed = %v", ids(got))
	}
	if got := mine(StatusCancelled); ids(got) != c.ID {
		t.Errorf("cancelled = %v", ids(got))
	}
	if got := mine(StatusRejected); len(got) != 0 {
		t.Errorf("rejected = %v", ids(got))
	}
	done := mine(StatusCompleted)[0]
	if done.UserName == "" || !strings.Contains(done.UserEmail, f.e.Prefix) || done.AccountNumber != "1234567890" {
		t.Errorf("identitas dan rekening tujuan harus tampil untuk admin: %+v", done)
	}
	if done.ReviewedBy == nil || *done.ReviewedBy != admin {
		t.Errorf("reviewed_by = %v, want %s", done.ReviewedBy, admin)
	}

	list, err := f.repo.ListByUser(f.ctx, u1, 10)
	if err != nil || len(list) != 2 || list[0].ID != d.ID || list[1].ID != p.ID {
		t.Errorf("ListByUser(u1) = %v, %v; want [d, p] terbaru dulu", list, err)
	}
	if one, _ := f.repo.ListByUser(f.ctx, u1, 1); len(one) != 1 {
		t.Errorf("limit 1 -> %d baris", len(one))
	}
	if none, err := f.repo.ListByUser(f.ctx, f.e.NewUser("kosong"), 10); err != nil || none == nil || len(none) != 0 {
		t.Errorf("tanpa riwayat: %v, %v (harus slice kosong non-nil)", none, err)
	}
	if _, err := f.repo.Get(f.ctx, u2, p.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get milik orang lain: err = %v, want ErrNotFound", err)
	}
	if got, err := f.repo.Get(f.ctx, u1, p.ID); err != nil || got.ID != p.ID {
		t.Errorf("Get milik sendiri: %+v, %v", got, err)
	}
}

// ---- siklus uang penuh -----------------------------------------------------

func TestFullMoneyCycleKeepsSystemBalanced(t *testing.T) {
	f := newFx(t)
	user, admin := f.e.NewUser("a"), f.e.NewAdmin("admin")
	external0 := f.external()

	f.e.FundUser(user, 500_000) // uang masuk dari bank
	w := f.mustMk(user, 500_000)
	if _, err := f.repo.Complete(f.ctx, admin, w.ID, "TRX-FULL", ""); err != nil { // uang keluar ke bank
		t.Fatal(err)
	}
	// masuk 500.000 lalu keluar 500.000: saldo pengguna 0, dompet external kembali ke semula
	if f.e.Balance(user) != 0 || f.external() != external0 {
		t.Errorf("saldo = %d, external %+d; want 0 dan 0", f.e.Balance(user), f.external()-external0)
	}
	f.requireReconciled()
}

// ---- perlindungan di database ----------------------------------------------

func TestDatabaseConstraints(t *testing.T) {
	f := newFx(t)
	user := f.e.NewUser("a")
	exec := func(sql string, args ...any) error {
		_, err := f.e.Pool.Exec(f.ctx, sql, args...)
		return err
	}
	insert := func(amount int64, status, ref string, reviewed bool) error {
		sql := `INSERT INTO withdrawals (user_id, amount, bank_name, account_number, account_holder, status, transfer_reference, reviewed_at)
		        VALUES ($1, $2, 'BCA', '12345', 'Budi', $3, $4, ` + map[bool]string{true: "now()", false: "NULL"}[reviewed] + `)`
		return exec(sql, user, amount, status, ref)
	}

	if err := insert(0, "pending", "", false); err == nil {
		t.Error("nominal 0 harus ditolak")
	}
	if err := insert(-5, "pending", "", false); err == nil {
		t.Error("nominal negatif harus ditolak")
	}
	if err := insert(50_000, "selesai", "", false); err == nil {
		t.Error("status asing harus ditolak")
	}
	if err := insert(50_000, "completed", "", true); err == nil {
		t.Error("completed tanpa referensi transfer harus ditolak")
	}
	if err := insert(50_000, "completed", "TRX-1", false); err == nil {
		t.Error("completed tanpa reviewed_at harus ditolak")
	}
	if err := insert(50_000, "rejected", "", false); err == nil {
		t.Error("rejected tanpa reviewed_at harus ditolak")
	}
	if err := insert(50_000, "pending", "", true); err == nil {
		t.Error("pending dengan reviewed_at harus ditolak")
	}
	if err := insert(50_000, "completed", "TRX-1", true); err != nil {
		t.Errorf("completed yang lengkap seharusnya diterima: %v", err)
	}
}
