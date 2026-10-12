/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package topup

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alexistdev/geopatungan/internal/testutil"
	"github.com/alexistdev/geopatungan/internal/wallet"
)

// Tes ini butuh database Postgres dengan semua migrasi; dilewati jika TEST_DATABASE_URL kosong.

type fx struct {
	e    *testutil.Env
	repo *Repository
	ctx  context.Context
}

func newFxRepo(t *testing.T) *fx {
	t.Helper()
	e := testutil.NewEnv(t)
	return &fx{e: e, repo: NewRepository(e.Pool), ctx: context.Background()}
}

// mk membuat top-up untuk userID dengan kandidat kode yang ditentukan.
func (f *fx) mk(userID string, amount int64, codes ...int) (*Topup, error) {
	return f.repo.Create(f.ctx, userID, amount, codes, 24, MaxOpenPerUser)
}

func (f *fx) mustMk(userID string, amount int64, code int) *Topup {
	f.e.T.Helper()
	t, err := f.mk(userID, amount, code)
	if err != nil {
		f.e.T.Fatalf("Create(%d, kode %d): %v", amount, code, err)
	}
	return t
}

func (f *fx) status(id string) string {
	return f.e.QueryString(`SELECT status FROM topups WHERE id = $1`, id)
}

func (f *fx) externalBalance() int64 {
	return int64(f.e.QueryInt(`SELECT balance FROM wallets WHERE type = 'external'`))
}

// ---- Create ----------------------------------------------------------------

func TestCreate(t *testing.T) {
	f := newFxRepo(t)
	user := f.e.NewUser("a")

	got, err := f.repo.Create(f.ctx, user, 100_000, []int{347}, 12, 3)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if got.ID == "" || got.UserID != user || got.Amount != 100_000 || got.UniqueCode != 347 ||
		got.TransferAmount != 100_347 || got.Status != StatusPending {
		t.Errorf("hasil = %+v", got)
	}
	if got.SubmittedAt != nil || got.ReviewedAt != nil || got.ReceivedAmount != nil || got.SenderName != "" {
		t.Errorf("kolom lanjutan harus kosong: %+v", got)
	}
	// kedaluwarsa = sekarang + 12 jam (toleransi beberapa detik)
	if d := time.Until(got.ExpiresAt); d < 11*time.Hour+59*time.Minute || d > 12*time.Hour+time.Minute {
		t.Errorf("expires_at %v dari sekarang, want ~12 jam", d)
	}
}

func TestCreateUniqueTransferAmountAcrossOpenRequests(t *testing.T) {
	f := newFxRepo(t)
	a, b, c := f.e.NewUser("a"), f.e.NewUser("b"), f.e.NewUser("c")

	first := f.mustMk(a, 200_000, 7)
	// B meminta nominal sama dengan kandidat kode [7, 8]: 7 sudah dipakai -> memakai 8
	second, err := f.mk(b, 200_000, 7, 8)
	if err != nil {
		t.Fatal(err)
	}
	if second.UniqueCode != 8 || second.TransferAmount != 200_008 {
		t.Errorf("kode = %d transfer = %d, want 8 dan 200008", second.UniqueCode, second.TransferAmount)
	}
	if first.TransferAmount == second.TransferAmount {
		t.Error("dua permintaan terbuka tidak boleh punya nominal transfer sama")
	}

	// semua kandidat sudah terpakai -> ErrBusy, dan tidak meninggalkan baris
	if _, err := f.mk(c, 200_000, 7, 8); !errors.Is(err, ErrBusy) {
		t.Errorf("err = %v, want ErrBusy", err)
	}
	if n := f.e.QueryInt(`SELECT count(*) FROM topups WHERE user_id = $1`, c); n != 0 {
		t.Errorf("baris tersisa setelah ErrBusy = %d, want 0", n)
	}
}

func TestCreateSameCodeDifferentAmountIsFine(t *testing.T) {
	f := newFxRepo(t)
	a, b := f.e.NewUser("a"), f.e.NewUser("b")
	f.mustMk(a, 100_000, 50)
	if _, err := f.mk(b, 150_000, 50); err != nil {
		t.Errorf("kode sama pada nominal berbeda seharusnya boleh: %v", err)
	}
}

// Kolom transfer_amount unik hanya untuk permintaan terbuka. Nominal yang BERBEDA tetapi
// menghasilkan transfer yang sama (100.000+500 vs 100.500+0 tidak mungkin karena kode >= 1;
// 100.000+501 vs 100.500+1) juga harus ditolak: yang dibandingkan adalah total transfer.
func TestCreateCollisionIsOnTotalTransferAmount(t *testing.T) {
	f := newFxRepo(t)
	a, b := f.e.NewUser("a"), f.e.NewUser("b")
	f.mustMk(a, 100_000, 501) // transfer 100.501
	got, err := f.mk(b, 100_500, 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if got.UniqueCode != 2 {
		t.Errorf("kode = %d, want 2 (100.500+1 = 100.501 bentrok dengan permintaan A)", got.UniqueCode)
	}
}

func TestCreateEnforcesMaxOpenPerUser(t *testing.T) {
	f := newFxRepo(t)
	user := f.e.NewUser("a")

	var ids []string
	for i := 0; i < 3; i++ {
		ids = append(ids, f.mustMk(user, 100_000, 10+i).ID)
	}
	if _, err := f.mk(user, 100_000, 99); !errors.Is(err, ErrTooManyOpen) {
		t.Fatalf("permintaan ke-4: err = %v, want ErrTooManyOpen", err)
	}
	// submitted tetap dihitung terbuka
	if _, err := f.repo.Confirm(f.ctx, user, ids[0], "Budi", "BCA", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := f.mk(user, 100_000, 98); !errors.Is(err, ErrTooManyOpen) {
		t.Errorf("submitted masih terbuka: err = %v, want ErrTooManyOpen", err)
	}
	// membatalkan satu pending membuka tempat
	if _, err := f.repo.Cancel(f.ctx, user, ids[1]); err != nil {
		t.Fatal(err)
	}
	if _, err := f.mk(user, 100_000, 97); err != nil {
		t.Errorf("setelah pembatalan seharusnya bisa lagi: %v", err)
	}
	// pengguna lain tidak terpengaruh
	if _, err := f.mk(f.e.NewUser("b"), 100_000, 96); err != nil {
		t.Errorf("pengguna lain: %v", err)
	}
}

// Pembuatan serentak oleh satu pengguna tidak boleh melewati batas (kunci advisory per pengguna).
func TestCreateConcurrentRespectsMaxOpen(t *testing.T) {
	f := newFxRepo(t)
	user := f.e.NewUser("a")

	const n = 12
	results := make([]error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, results[i] = f.mk(user, 100_000, 100+i)
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
	if n := f.e.QueryInt(`SELECT count(*) FROM topups WHERE user_id = $1 AND status = 'pending'`, user); n != MaxOpenPerUser {
		t.Errorf("baris pending = %d, want %d", n, MaxOpenPerUser)
	}
}

func TestLazyExpiryFreesCodesButNotSubmitted(t *testing.T) {
	f := newFxRepo(t)
	a, b := f.e.NewUser("a"), f.e.NewUser("b")

	stale := f.mustMk(a, 100_000, 5)
	submitted := f.mustMk(a, 100_000, 6)
	if _, err := f.repo.Confirm(f.ctx, a, submitted.ID, "Budi", "BCA", ""); err != nil {
		t.Fatal(err)
	}
	f.e.Exec(`UPDATE topups SET expires_at = now() - interval '1 hour' WHERE id IN ($1, $2)`, stale.ID, submitted.ID)

	// permintaan baru oleh pengguna LAIN memakai kode yang sama dengan yang kedaluwarsa
	got, err := f.mk(b, 100_000, 5)
	if err != nil {
		t.Fatalf("kode milik permintaan kedaluwarsa seharusnya bebas: %v", err)
	}
	if got.UniqueCode != 5 {
		t.Errorf("kode = %d, want 5", got.UniqueCode)
	}
	if f.status(stale.ID) != StatusExpired {
		t.Errorf("status = %s, want expired", f.status(stale.ID))
	}
	if f.status(submitted.ID) != StatusSubmitted {
		t.Errorf("submitted tidak boleh kedaluwarsa (uangnya mungkin sudah masuk): %s", f.status(submitted.ID))
	}
	// kode submitted tetap terkunci
	if _, err := f.mk(f.e.NewUser("c"), 100_000, 6); !errors.Is(err, ErrBusy) {
		t.Errorf("kode milik submitted harus tetap terpakai: err = %v", err)
	}
}

// ---- Confirm / Cancel / Get / List -------------------------------------------

func TestConfirm(t *testing.T) {
	f := newFxRepo(t)
	user, other := f.e.NewUser("a"), f.e.NewUser("b")
	tp := f.mustMk(user, 100_000, 21)

	got, err := f.repo.Confirm(f.ctx, user, tp.ID, "Budi Santoso", "BCA", "pukul 10.00")
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if got.Status != StatusSubmitted || got.SenderName != "Budi Santoso" || got.SenderBank != "BCA" ||
		got.Note != "pukul 10.00" || got.SubmittedAt == nil {
		t.Errorf("hasil = %+v", got)
	}
	firstSubmitted := *got.SubmittedAt

	// mengulang memperbaiki keterangan tanpa mengubah waktu konfirmasi pertama
	again, err := f.repo.Confirm(f.ctx, user, tp.ID, "Budi S.", "Mandiri", "koreksi")
	if err != nil || again.SenderName != "Budi S." || again.SenderBank != "Mandiri" || !again.SubmittedAt.Equal(firstSubmitted) {
		t.Errorf("konfirmasi ulang: %+v, %v", again, err)
	}

	if _, err := f.repo.Confirm(f.ctx, other, tp.ID, "X Y", "BCA", ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("milik orang lain: err = %v, want ErrNotFound", err)
	}
	if _, err := f.repo.Confirm(f.ctx, user, "00000000-0000-0000-0000-000000000000", "Budi", "BCA", ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("id tidak ada: err = %v, want ErrNotFound", err)
	}
	if f.e.QueryString(`SELECT sender_name FROM topups WHERE id = $1`, tp.ID) != "Budi S." {
		t.Error("percobaan orang lain tidak boleh mengubah keterangan")
	}

	for _, final := range []string{"approved", "rejected", "cancelled", "expired"} {
		t.Run("status "+final, func(t *testing.T) {
			x := f.mustMk(user, 100_000, 30+len(final))
			f.forceStatus(x.ID, final)
			if _, err := f.repo.Confirm(f.ctx, user, x.ID, "Budi", "BCA", ""); !errors.Is(err, ErrInvalidState) {
				t.Errorf("err = %v, want ErrInvalidState", err)
			}
		})
	}
}

// forceStatus memaksa status langsung di database (melengkapi kolom yang disyaratkan CHECK).
func (f *fx) forceStatus(id, status string) {
	f.e.T.Helper()
	switch status {
	case "approved":
		f.e.Exec(`UPDATE topups SET status = 'approved', received_amount = transfer_amount, reviewed_at = now() WHERE id = $1`, id)
	case "rejected":
		f.e.Exec(`UPDATE topups SET status = 'rejected', reviewed_at = now() WHERE id = $1`, id)
	default:
		f.e.Exec(`UPDATE topups SET status = $2 WHERE id = $1`, id, status)
	}
}

func TestCancel(t *testing.T) {
	f := newFxRepo(t)
	user, other := f.e.NewUser("a"), f.e.NewUser("b")

	tp := f.mustMk(user, 100_000, 41)
	if _, err := f.repo.Cancel(f.ctx, other, tp.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("milik orang lain: err = %v, want ErrNotFound", err)
	}
	got, err := f.repo.Cancel(f.ctx, user, tp.ID)
	if err != nil || got.Status != StatusCancelled {
		t.Fatalf("Cancel: %+v, %v", got, err)
	}
	if _, err := f.repo.Cancel(f.ctx, user, tp.ID); !errors.Is(err, ErrInvalidState) {
		t.Errorf("batal ulang: err = %v, want ErrInvalidState", err)
	}

	// kode permintaan yang dibatalkan kembali bebas
	if _, err := f.mk(other, 100_000, 41); err != nil {
		t.Errorf("kode seharusnya bebas setelah dibatalkan: %v", err)
	}

	// submitted tidak bisa dibatalkan pengguna: uangnya mungkin sudah masuk
	sub := f.mustMk(user, 100_000, 42)
	if _, err := f.repo.Confirm(f.ctx, user, sub.ID, "Budi", "BCA", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := f.repo.Cancel(f.ctx, user, sub.ID); !errors.Is(err, ErrInvalidState) {
		t.Errorf("submitted: err = %v, want ErrInvalidState", err)
	}
	if f.status(sub.ID) != StatusSubmitted {
		t.Errorf("status berubah: %s", f.status(sub.ID))
	}
}

func TestGetAndListAreScopedToOwner(t *testing.T) {
	f := newFxRepo(t)
	a, b := f.e.NewUser("a"), f.e.NewUser("b")
	t1 := f.mustMk(a, 100_000, 51)
	time.Sleep(5 * time.Millisecond)
	t2 := f.mustMk(a, 100_000, 52)
	f.mustMk(b, 100_000, 53)

	if got, err := f.repo.Get(f.ctx, a, t1.ID); err != nil || got.ID != t1.ID {
		t.Errorf("Get milik sendiri: %+v, %v", got, err)
	}
	if _, err := f.repo.Get(f.ctx, b, t1.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get milik orang lain: err = %v, want ErrNotFound", err)
	}

	list, err := f.repo.ListByUser(f.ctx, a, 10)
	if err != nil || len(list) != 2 {
		t.Fatalf("list = %d, %v, want 2", len(list), err)
	}
	if list[0].ID != t2.ID || list[1].ID != t1.ID {
		t.Error("harus terbaru lebih dulu")
	}
	if one, _ := f.repo.ListByUser(f.ctx, a, 1); len(one) != 1 {
		t.Errorf("limit 1 -> %d baris", len(one))
	}
	if none, err := f.repo.ListByUser(f.ctx, f.e.NewUser("c"), 10); err != nil || none == nil || len(none) != 0 {
		t.Errorf("tanpa riwayat: %v, %v (harus slice kosong non-nil)", none, err)
	}
}

// ---- Approve ---------------------------------------------------------------

func TestApproveCreditsWalletThroughLedger(t *testing.T) {
	f := newFxRepo(t)
	user := f.e.NewUser("a")
	admin := f.e.NewAdmin("admin")
	tp := f.mustMk(user, 250_000, 123) // transfer 250.123
	externalBefore := f.externalBalance()

	got, err := f.repo.Approve(f.ctx, admin, tp.ID, nil, "cocok dengan mutasi BCA")
	if err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if got.Status != StatusApproved || got.ReceivedAmount == nil || *got.ReceivedAmount != 250_123 ||
		got.ReviewedAt == nil || got.ReviewNote != "cocok dengan mutasi BCA" {
		t.Errorf("hasil = %+v", got)
	}

	// saldo pengguna naik sebesar yang ditransfer; external turun sebesar itu
	if f.e.Balance(user) != 250_123 {
		t.Errorf("saldo = %d, want 250123", f.e.Balance(user))
	}
	if d := f.externalBalance() - externalBefore; d != -250_123 {
		t.Errorf("perubahan external = %d, want -250123", d)
	}

	// tercatat di ledger dengan referensi top-up
	entries, err := wallet.NewRepository(f.e.Pool).Ledger(f.ctx, user, 10, 0)
	if err != nil || len(entries) != 1 {
		t.Fatalf("ledger = %v, %v", entries, err)
	}
	if e := entries[0]; e.Amount != 250_123 || e.RefType != "topup" || e.RefID != tp.ID || e.BalanceAfter != 250_123 {
		t.Errorf("entri ledger = %+v", e)
	}

	// audit mencatat siapa yang menyetujui
	if n := f.e.QueryInt(`SELECT count(*) FROM audit_logs WHERE action = 'topup.approve' AND entity_id = $1 AND actor_id = $2
	                        AND (meta->>'received_amount')::bigint = 250123`, tp.ID, admin); n != 1 {
		t.Errorf("audit approve = %d, want 1", n)
	}
	if n := f.e.QueryInt(`SELECT count(*) FROM topups WHERE id = $1 AND reviewed_by = $2`, tp.ID, admin); n != 1 {
		t.Error("reviewed_by harus mencatat admin")
	}

	// pembukuan tetap seimbang
	if rep, err := wallet.Reconcile(f.ctx, f.e.Pool); err != nil || !rep.Ok {
		t.Errorf("rekonsiliasi: %v, %+v", err, rep)
	}
}

func TestApproveWithAdjustedAmount(t *testing.T) {
	f := newFxRepo(t)
	user, admin := f.e.NewUser("a"), f.e.NewAdmin("admin")
	tp := f.mustMk(user, 100_000, 200) // seharusnya 100.200; pengguna salah transfer 100.000

	received := int64(100_000)
	got, err := f.repo.Approve(f.ctx, admin, tp.ID, &received, "kurang kode unik, nominal disesuaikan")
	if err != nil {
		t.Fatal(err)
	}
	if *got.ReceivedAmount != 100_000 || f.e.Balance(user) != 100_000 {
		t.Errorf("received = %d saldo = %d, want 100000 keduanya", *got.ReceivedAmount, f.e.Balance(user))
	}
	if got.TransferAmount != 100_200 {
		t.Errorf("transfer_amount asli tidak boleh berubah: %d", got.TransferAmount)
	}
}

func TestApproveRejectionsLeaveNoTrace(t *testing.T) {
	f := newFxRepo(t)
	user, admin := f.e.NewUser("a"), f.e.NewAdmin("admin")

	t.Run("memproses permintaan sendiri", func(t *testing.T) {
		selfAdmin := f.e.NewAdmin("self")
		tp := f.mustMk(selfAdmin, 100_000, 61)
		if _, err := f.repo.Approve(f.ctx, selfAdmin, tp.ID, nil, ""); !errors.Is(err, ErrSelfReview) {
			t.Errorf("approve: err = %v, want ErrSelfReview", err)
		}
		if _, err := f.repo.Reject(f.ctx, selfAdmin, tp.ID, "alasan"); !errors.Is(err, ErrSelfReview) {
			t.Errorf("reject: err = %v, want ErrSelfReview", err)
		}
		if f.e.Balance(selfAdmin) != 0 || f.status(tp.ID) != StatusPending {
			t.Errorf("saldo = %d status = %s; tidak boleh berubah", f.e.Balance(selfAdmin), f.status(tp.ID))
		}
	})
	t.Run("id tidak ada", func(t *testing.T) {
		if _, err := f.repo.Approve(f.ctx, admin, "00000000-0000-0000-0000-000000000000", nil, ""); !errors.Is(err, ErrNotFound) {
			t.Errorf("err = %v, want ErrNotFound", err)
		}
	})
	for _, st := range []string{"approved", "rejected", "cancelled"} {
		t.Run("status "+st, func(t *testing.T) {
			tp := f.mustMk(user, 100_000, 70+len(st))
			f.forceStatus(tp.ID, st)
			before := f.e.Balance(user)
			if _, err := f.repo.Approve(f.ctx, admin, tp.ID, nil, ""); !errors.Is(err, ErrInvalidState) {
				t.Errorf("err = %v, want ErrInvalidState", err)
			}
			if f.e.Balance(user) != before {
				t.Errorf("saldo berubah dari %d menjadi %d", before, f.e.Balance(user))
			}
		})
	}
	t.Run("nominal nol gagal tanpa efek samping", func(t *testing.T) {
		tp := f.mustMk(user, 100_000, 90)
		zero := int64(0)
		if _, err := f.repo.Approve(f.ctx, admin, tp.ID, &zero, ""); err == nil {
			t.Fatal("nominal 0 seharusnya gagal")
		}
		if f.status(tp.ID) != StatusPending || f.e.Balance(user) != 0 {
			t.Errorf("status = %s saldo = %d; harus utuh", f.status(tp.ID), f.e.Balance(user))
		}
		if n := f.e.QueryInt(`SELECT count(*) FROM audit_logs WHERE entity_id = $1`, tp.ID); n != 0 {
			t.Errorf("audit tersisa = %d, want 0", n)
		}
	})
}

func TestApproveTwiceCreditsOnce(t *testing.T) {
	f := newFxRepo(t)
	user, admin := f.e.NewUser("a"), f.e.NewAdmin("admin")
	tp := f.mustMk(user, 100_000, 111)

	if _, err := f.repo.Approve(f.ctx, admin, tp.ID, nil, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := f.repo.Approve(f.ctx, admin, tp.ID, nil, ""); !errors.Is(err, ErrInvalidState) {
		t.Errorf("approve ulang: err = %v, want ErrInvalidState", err)
	}
	if f.e.Balance(user) != 100_111 {
		t.Errorf("saldo = %d, want 100111 (tidak boleh dikreditkan dua kali)", f.e.Balance(user))
	}
}

func TestApproveExpiredIsAllowed(t *testing.T) {
	f := newFxRepo(t)
	user, admin := f.e.NewUser("a"), f.e.NewAdmin("admin")
	tp := f.mustMk(user, 100_000, 112)
	f.forceStatus(tp.ID, "expired")

	got, err := f.repo.Approve(f.ctx, admin, tp.ID, nil, "transfer datang terlambat")
	if err != nil || got.Status != StatusApproved {
		t.Fatalf("approve expired: %+v, %v", got, err)
	}
	if f.e.Balance(user) != 100_112 {
		t.Errorf("saldo = %d, want 100112", f.e.Balance(user))
	}
}

// Beberapa persetujuan bersamaan: tepat satu yang mengkreditkan.
func TestApproveConcurrentCreditsOnce(t *testing.T) {
	f := newFxRepo(t)
	user := f.e.NewUser("a")
	admins := []string{f.e.NewAdmin("a1"), f.e.NewAdmin("a2"), f.e.NewAdmin("a3")}
	tp := f.mustMk(user, 100_000, 113)

	const n = 9
	results := make([]error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, results[i] = f.repo.Approve(f.ctx, admins[i%len(admins)], tp.ID, nil, "")
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
		t.Errorf("%d persetujuan berhasil, want tepat 1", ok)
	}
	if f.e.Balance(user) != 100_113 {
		t.Errorf("saldo = %d, want 100113", f.e.Balance(user))
	}
}

// Admin A menolak (belum commit); persetujuan oleh admin B yang dimulai saat itu harus
// menunggu lalu gagal. Tanpa FOR UPDATE, B membaca status lama (pending), mengkreditkan
// saldo, dan menimpa penolakan A: permintaan "ditolak" di audit tetapi uangnya sudah masuk.
func TestApproveLocksRequestAgainstConcurrentReject(t *testing.T) {
	f := newFxRepo(t)
	user := f.e.NewUser("a")
	adminA, adminB := f.e.NewAdmin("adminA"), f.e.NewAdmin("adminB")
	tp := f.mustMk(user, 100_000, 114)

	txA, err := f.e.Pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = txA.Rollback(f.ctx) }()
	if _, err := txA.Exec(f.ctx, `SELECT 1 FROM topups WHERE id = $1 FOR UPDATE`, tp.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := txA.Exec(f.ctx,
		`UPDATE topups SET status = 'rejected', reviewed_by = $2, reviewed_at = now(), review_note = 'ditolak A' WHERE id = $1`,
		tp.ID, adminA); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() { _, err := f.repo.Approve(f.ctx, adminB, tp.ID, nil, ""); done <- err }()

	select {
	case err := <-done:
		t.Fatalf("Approve selesai (err=%v) padahal A belum commit; seharusnya menunggu", err)
	case <-time.After(300 * time.Millisecond):
	}
	if err := txA.Commit(f.ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, ErrInvalidState) {
			t.Errorf("err = %v, want ErrInvalidState (sudah ditolak A)", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Approve tidak selesai setelah A commit")
	}
	if f.status(tp.ID) != StatusRejected || f.e.Balance(user) != 0 {
		t.Errorf("status = %s saldo = %d, want rejected dan 0", f.status(tp.ID), f.e.Balance(user))
	}
}

// ---- Reject ----------------------------------------------------------------

func TestReject(t *testing.T) {
	f := newFxRepo(t)
	user, admin := f.e.NewUser("a"), f.e.NewAdmin("admin")
	tp := f.mustMk(user, 100_000, 131)

	got, err := f.repo.Reject(f.ctx, admin, tp.ID, "transfer tidak ditemukan di mutasi")
	if err != nil {
		t.Fatalf("Reject: %v", err)
	}
	if got.Status != StatusRejected || got.ReviewedAt == nil || got.ReviewNote != "transfer tidak ditemukan di mutasi" || got.ReceivedAmount != nil {
		t.Errorf("hasil = %+v", got)
	}
	if f.e.Balance(user) != 0 {
		t.Errorf("penolakan tidak boleh mengubah saldo: %d", f.e.Balance(user))
	}
	if n := f.e.QueryInt(`SELECT count(*) FROM audit_logs WHERE action = 'topup.reject' AND entity_id = $1 AND actor_id = $2`, tp.ID, admin); n != 1 {
		t.Errorf("audit reject = %d, want 1", n)
	}

	// final: tidak bisa disetujui atau ditolak lagi
	if _, err := f.repo.Approve(f.ctx, admin, tp.ID, nil, ""); !errors.Is(err, ErrInvalidState) {
		t.Errorf("approve setelah reject: err = %v", err)
	}
	if _, err := f.repo.Reject(f.ctx, admin, tp.ID, "lagi"); !errors.Is(err, ErrInvalidState) {
		t.Errorf("reject ulang: err = %v", err)
	}
	// kodenya kembali bebas
	if _, err := f.mk(f.e.NewUser("b"), 100_000, 131); err != nil {
		t.Errorf("kode seharusnya bebas setelah ditolak: %v", err)
	}
	// dapat menolak submitted dan expired
	for i, st := range []string{"submitted", "expired"} {
		x := f.mustMk(user, 100_000, 140+i)
		f.forceStatus(x.ID, st)
		if _, err := f.repo.Reject(f.ctx, admin, x.ID, "alasan"); err != nil {
			t.Errorf("reject %s: %v", st, err)
		}
	}
}

// ---- daftar admin ----------------------------------------------------------

func TestListAdmin(t *testing.T) {
	f := newFxRepo(t)
	admin := f.e.NewAdmin("admin")
	u1, u2 := f.e.NewUser("u1"), f.e.NewUser("u2")

	p := f.mustMk(u1, 100_000, 151)
	time.Sleep(5 * time.Millisecond)
	s := f.mustMk(u2, 100_000, 152)
	if _, err := f.repo.Confirm(f.ctx, u2, s.ID, "Dua", "BNI", ""); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	a := f.mustMk(u1, 100_000, 153)
	if _, err := f.repo.Approve(f.ctx, admin, a.ID, nil, ""); err != nil {
		t.Fatal(err)
	}

	mine := func(status string) []AdminTopup {
		t.Helper()
		all, err := f.repo.ListAdmin(f.ctx, status, 200)
		if err != nil {
			t.Fatal(err)
		}
		var out []AdminTopup
		for _, x := range all {
			if strings.Contains(x.UserEmail, f.e.Prefix) {
				out = append(out, x)
			}
		}
		return out
	}
	ids := func(l []AdminTopup) string {
		var s []string
		for _, x := range l {
			s = append(s, x.ID)
		}
		return strings.Join(s, ",")
	}

	open := mine("open")
	if ids(open) != p.ID+","+s.ID {
		t.Errorf("open = %v, want [pending, submitted] terlama dulu", ids(open))
	}
	if all := mine("all"); len(all) != 3 || all[0].ID != a.ID {
		t.Errorf("all = %v, want 3 baris terbaru dulu", ids(all))
	}
	if got := mine("approved"); ids(got) != a.ID {
		t.Errorf("approved = %v", ids(got))
	}
	if got := mine("submitted"); ids(got) != s.ID {
		t.Errorf("submitted = %v", ids(got))
	}
	if got := mine("rejected"); len(got) != 0 {
		t.Errorf("rejected = %v, want kosong", ids(got))
	}

	// identitas pemohon & peninjau tampil
	approved := mine("approved")[0]
	if approved.UserName == "" || !strings.Contains(approved.UserEmail, f.e.Prefix) {
		t.Errorf("identitas pemohon: %+v", approved)
	}
	if approved.ReviewedBy == nil || *approved.ReviewedBy != admin {
		t.Errorf("reviewed_by = %v, want %s", approved.ReviewedBy, admin)
	}
	if open[0].ReviewedBy != nil {
		t.Error("reviewed_by harus kosong untuk yang belum ditinjau")
	}
}

// ---- perlindungan di database ----------------------------------------------

func TestDatabaseConstraints(t *testing.T) {
	f := newFxRepo(t)
	user := f.e.NewUser("a")
	f.mustMk(user, 100_000, 161) // transfer 100.161
	exec := func(sql string, args ...any) error {
		_, err := f.e.Pool.Exec(f.ctx, sql, args...)
		return err
	}
	insert := func(amount int64, code int, transfer int64, extra string) error {
		return exec(fmt.Sprintf(`INSERT INTO topups (user_id, amount, unique_code, transfer_amount, expires_at %s)
		                          VALUES ($1, $2, $3, $4, now() + interval '1 day' %s)`, extra, ""),
			user, amount, code, transfer)
	}

	t.Run("transfer harus = nominal + kode", func(t *testing.T) {
		if err := insert(100_000, 5, 100_999, ""); err == nil {
			t.Error("transfer_amount yang tidak sama dengan amount+code harus ditolak")
		}
	})
	t.Run("kode di luar 1..999", func(t *testing.T) {
		if err := insert(100_000, 0, 100_000, ""); err == nil {
			t.Error("kode 0 harus ditolak")
		}
		if err := insert(100_000, 1000, 101_000, ""); err == nil {
			t.Error("kode 1000 harus ditolak")
		}
	})
	t.Run("nominal harus positif", func(t *testing.T) {
		if err := insert(0, 5, 5, ""); err == nil {
			t.Error("amount 0 harus ditolak")
		}
	})
	t.Run("nominal transfer unik untuk permintaan terbuka", func(t *testing.T) {
		if err := insert(100_000, 161, 100_161, ""); err == nil {
			t.Error("transfer_amount ganda pada permintaan terbuka harus ditolak indeks unik")
		}
	})
	t.Run("approved wajib punya received_amount dan reviewed_at", func(t *testing.T) {
		if err := exec(`UPDATE topups SET status = 'approved' WHERE user_id = $1`, user); err == nil {
			t.Error("approved tanpa received_amount/reviewed_at harus ditolak")
		}
		if err := exec(`UPDATE topups SET status = 'approved', received_amount = 1 WHERE user_id = $1`, user); err == nil {
			t.Error("approved tanpa reviewed_at harus ditolak")
		}
	})
	t.Run("received_amount hanya untuk approved", func(t *testing.T) {
		if err := exec(`UPDATE topups SET received_amount = 5 WHERE user_id = $1`, user); err == nil {
			t.Error("received_amount pada status pending harus ditolak")
		}
	})
	t.Run("status asing", func(t *testing.T) {
		if err := exec(`UPDATE topups SET status = 'selesai' WHERE user_id = $1`, user); err == nil {
			t.Error("status asing harus ditolak")
		}
	})
}
