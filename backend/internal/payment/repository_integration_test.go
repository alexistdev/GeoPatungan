/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package payment

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/alexistdev/geopatungan/internal/contribution"
	"github.com/alexistdev/geopatungan/internal/group"
	"github.com/alexistdev/geopatungan/internal/round"
	"github.com/alexistdev/geopatungan/internal/settings"
	"github.com/alexistdev/geopatungan/internal/testutil"
	"github.com/alexistdev/geopatungan/internal/wallet"
)

// Tes ini butuh database Postgres dengan semua migrasi; dilewati jika TEST_DATABASE_URL kosong.
//
// Perhatian: JANGAN memanggil helper env (e.QueryInt, e.Balance, dst.) dari DALAM closure
// InTx atau dari banyak goroutine yang masing-masing memegang transaksi: itu meminjam koneksi
// kedua dari pool dan bisa membuntukan pool.

type fx struct {
	e       *testutil.Env
	repo    *Repository
	ctx     context.Context
	g       *group.Group
	members []group.Member
}

func fixedFees(bps int) FeeSource {
	return func(context.Context, settings.Querier) (int, error) { return bps, nil }
}

func newRepo(e *testutil.Env, bps int) *Repository {
	r := NewRepository(e.Pool)
	r.fees = fixedFees(bps)
	return r
}

// startGroup menetapkan urutan giliran sesuai urutan anggota, menjalankan grup, dan membuat jadwal.
func startGroup(t *testing.T, e *testutil.Env, g *group.Group, members []group.Member) {
	t.Helper()
	e.SetTurnOrders(members)
	e.Exec(`UPDATE groups SET status = 'running' WHERE id = $1`, g.ID)
	err := e.InTx(func(tx pgx.Tx) error { return round.CreateSchedule(context.Background(), tx, g.ID) })
	if err != nil {
		t.Fatalf("CreateSchedule: %v", err)
	}
}

// newFx membuat grup berjalan; setiap anggota didanai `funded` rupiah.
func newFx(t *testing.T, spec testutil.GroupSpec, funded int64, bps int) *fx {
	t.Helper()
	e := testutil.NewEnv(t)
	g, members := e.NewGroup(spec)
	startGroup(t, e, g, members)
	if funded > 0 {
		for _, m := range members {
			e.FundUser(m.UserID, funded)
		}
	}
	return &fx{e: e, repo: newRepo(e, bps), ctx: context.Background(), g: g, members: members}
}

func (f *fx) user(i int) string { return f.members[i].UserID }

func (f *fx) cid(roundNo, i int) string {
	return f.e.QueryString(`SELECT c.id::text FROM contributions c JOIN rounds r ON r.id = c.round_id
	                         WHERE r.group_id = $1 AND r.round_no = $2 AND c.member_id = $3`, f.g.ID, roundNo, f.members[i].ID)
}

func (f *fx) roundID(no int) string {
	return f.e.QueryString(`SELECT id::text FROM rounds WHERE group_id = $1 AND round_no = $2`, f.g.ID, no)
}

func (f *fx) roundStatus(no int) string {
	return f.e.QueryString(`SELECT status FROM rounds WHERE group_id = $1 AND round_no = $2`, f.g.ID, no)
}

func (f *fx) pay(roundNo, i int) (*Result, error) {
	return f.repo.Pay(f.ctx, f.user(i), f.cid(roundNo, i))
}

func (f *fx) mustPay(roundNo, i int) *Result {
	f.e.T.Helper()
	res, err := f.pay(roundNo, i)
	if err != nil {
		f.e.T.Fatalf("Pay(putaran %d, anggota %d): %v", roundNo, i, err)
	}
	return res
}

func (f *fx) bal(i int) int64 { return f.e.Balance(f.user(i)) }

func (f *fx) system(typ string) int64 {
	return int64(f.e.QueryInt(`SELECT balance FROM wallets WHERE type = $1`, typ))
}

func (f *fx) ledgerCount(i int, refType string) int {
	return f.e.QueryInt(`SELECT count(*) FROM ledger_entries l JOIN wallets w ON w.id = l.wallet_id
	                      WHERE w.user_id = $1 AND l.ref_type = $2`, f.user(i), refType)
}

func (f *fx) requireReconciled() {
	f.e.T.Helper()
	rep, err := wallet.Reconcile(f.ctx, f.e.Pool)
	if err != nil || !rep.Ok {
		f.e.T.Fatalf("rekonsiliasi gagal: %v %+v", err, rep)
	}
}

// ---- pembayaran dasar --------------------------------------------------------

func TestPayDebitsPayerAndFillsEscrow(t *testing.T) {
	f := newFx(t, testutil.GroupSpec{Slots: 3, Amount: 100_000}, 500_000, 0)
	escrow0, platform0 := f.system("escrow"), f.system("platform")
	cid := f.cid(1, 1)

	res, err := f.repo.Pay(f.ctx, f.user(1), cid)
	if err != nil {
		t.Fatalf("Pay: %v", err)
	}
	if res.Balance != 400_000 || f.bal(1) != 400_000 {
		t.Errorf("saldo = %d / %d, want 400000", res.Balance, f.bal(1))
	}
	if d := f.system("escrow") - escrow0; d != 100_000 {
		t.Errorf("escrow naik %d, want 100000", d)
	}
	if d := f.system("platform") - platform0; d != 0 {
		t.Errorf("platform berubah %d padahal biaya 0", d)
	}
	if res.Receipt.Total != 100_000 || res.Receipt.ID != cid || !res.Receipt.PaidOnTime {
		t.Errorf("receipt = %+v", res.Receipt)
	}
	if res.Round.Completed || res.Round.Payout != nil {
		t.Errorf("putaran belum boleh selesai: %+v", res.Round)
	}
	if st := f.e.QueryString(`SELECT status FROM contributions WHERE id = $1`, cid); st != "paid" {
		t.Errorf("status iuran = %s", st)
	}

	entries, err := wallet.NewRepository(f.e.Pool).Ledger(f.ctx, f.user(1), 5, 0)
	if err != nil || len(entries) != 2 {
		t.Fatalf("ledger = %v, %v", entries, err)
	}
	if e := entries[0]; e.Amount != -100_000 || e.RefType != "contribution" || e.RefID != cid || !strings.Contains(e.Description, "Iuran putaran 1") {
		t.Errorf("entri terbaru = %+v", e)
	}
	f.requireReconciled()
}

func TestPayChargesFeesToPlatform(t *testing.T) {
	f := newFx(t, testutil.GroupSpec{Slots: 3, Amount: 200_000, LateFee: 10_000}, 1_000_000, 250)      // biaya 2,5%
	f.e.Exec(`UPDATE rounds SET due_date = '2020-01-01' WHERE group_id = $1 AND round_no = 1`, f.g.ID) // sudah lewat
	escrow0, platform0 := f.system("escrow"), f.system("platform")
	cid := f.cid(1, 1)

	res := f.mustPay(1, 1)
	// 200.000 iuran + 5.000 biaya + 10.000 denda
	if res.Receipt.PlatformFee != 5_000 || res.Receipt.LateFee != 10_000 || res.Receipt.Total != 215_000 || res.Receipt.PaidOnTime {
		t.Errorf("receipt = %+v", res.Receipt)
	}
	if f.bal(1) != 785_000 {
		t.Errorf("saldo = %d, want 785000", f.bal(1))
	}
	if d := f.system("escrow") - escrow0; d != 200_000 {
		t.Errorf("escrow naik %d, want 200000 (hanya iuran, tanpa biaya)", d)
	}
	if d := f.system("platform") - platform0; d != 15_000 {
		t.Errorf("platform naik %d, want 15000 (biaya + denda)", d)
	}
	var fee, late int64
	var onTime bool
	if err := f.e.Pool.QueryRow(f.ctx, `SELECT platform_fee, late_fee, paid_on_time FROM contributions WHERE id = $1`, cid).Scan(&fee, &late, &onTime); err != nil {
		t.Fatal(err)
	}
	if fee != 5_000 || late != 10_000 || onTime {
		t.Errorf("tersimpan fee=%d late=%d tepat=%v", fee, late, onTime)
	}
	f.requireReconciled()
}

func TestPayInsufficientFunds(t *testing.T) {
	f := newFx(t, testutil.GroupSpec{Slots: 3, Amount: 100_000}, 0, 250) // total 102.500
	f.e.FundUser(f.user(1), 102_499)
	escrow0, platform0 := f.system("escrow"), f.system("platform")
	cid := f.cid(1, 1)

	_, err := f.repo.Pay(f.ctx, f.user(1), cid)
	var short *InsufficientFundsError
	if !errors.As(err, &short) || !errors.Is(err, wallet.ErrInsufficientFunds) {
		t.Fatalf("err = %v, want InsufficientFundsError", err)
	}
	if short.Required != 102_500 || short.Balance != 102_499 {
		t.Errorf("required=%d balance=%d, want 102500 dan 102499", short.Required, short.Balance)
	}
	// tidak ada jejak apa pun
	if st := f.e.QueryString(`SELECT status FROM contributions WHERE id = $1`, cid); st != "unpaid" {
		t.Errorf("status = %s, want unpaid", st)
	}
	if fee := f.e.QueryInt(`SELECT platform_fee FROM contributions WHERE id = $1`, cid); fee != 0 {
		t.Errorf("platform_fee = %d; kegagalan tidak boleh meninggalkan biaya", fee)
	}
	if f.bal(1) != 102_499 || f.system("escrow") != escrow0 || f.system("platform") != platform0 {
		t.Error("saldo/escrow/platform berubah padahal pembayaran gagal")
	}
	if n := f.ledgerCount(1, "contribution"); n != 0 {
		t.Errorf("entri ledger iuran = %d, want 0", n)
	}

	// kurang Rp1 lalu dilengkapi: berhasil dan saldo tepat 0
	f.e.FundUser(f.user(1), 1)
	res, err := f.repo.Pay(f.ctx, f.user(1), cid)
	if err != nil || res.Balance != 0 {
		t.Fatalf("setelah dilengkapi: %+v, %v", res, err)
	}
}

func TestPayAuthorizationAndState(t *testing.T) {
	f := newFx(t, testutil.GroupSpec{Slots: 3, Amount: 100_000}, 500_000, 0)
	escrow0 := f.system("escrow")

	t.Run("iuran milik anggota lain", func(t *testing.T) {
		if _, err := f.repo.Pay(f.ctx, f.user(2), f.cid(1, 1)); !errors.Is(err, contribution.ErrNotFound) {
			t.Errorf("err = %v, want ErrNotFound", err)
		}
	})
	t.Run("orang luar", func(t *testing.T) {
		stranger := f.e.NewUser("stranger")
		f.e.FundUser(stranger, 500_000)
		if _, err := f.repo.Pay(f.ctx, stranger, f.cid(1, 1)); !errors.Is(err, contribution.ErrNotFound) {
			t.Errorf("err = %v, want ErrNotFound", err)
		}
	})
	t.Run("id tidak ada", func(t *testing.T) {
		if _, err := f.repo.Pay(f.ctx, f.user(1), "00000000-0000-0000-0000-000000000000"); !errors.Is(err, contribution.ErrNotFound) {
			t.Errorf("err = %v, want ErrNotFound", err)
		}
	})
	t.Run("putaran belum dibuka", func(t *testing.T) {
		if _, err := f.pay(2, 1); !errors.Is(err, round.ErrRoundNotCollecting) {
			t.Errorf("err = %v, want ErrRoundNotCollecting", err)
		}
	})
	if f.bal(1) != 500_000 || f.bal(2) != 500_000 || f.system("escrow") != escrow0 {
		t.Error("penolakan tidak boleh mengubah saldo apa pun")
	}

	t.Run("dibayar dua kali", func(t *testing.T) {
		f.mustPay(1, 1)
		if _, err := f.pay(1, 1); !errors.Is(err, contribution.ErrAlreadyPaid) {
			t.Errorf("err = %v, want ErrAlreadyPaid", err)
		}
		if f.bal(1) != 400_000 {
			t.Errorf("saldo = %d, want 400000 (terpotong sekali)", f.bal(1))
		}
	})
}

// ---- penutupan putaran dan pencairan -----------------------------------------

func TestLastPaymentCompletesRoundAndPaysRecipient(t *testing.T) {
	f := newFx(t, testutil.GroupSpec{Slots: 3, Amount: 100_000}, 500_000, 0)
	escrow0 := f.system("escrow")
	recipientName := f.e.QueryString(`SELECT u.name FROM users u WHERE u.id = $1`, f.user(0))

	f.mustPay(1, 0)
	f.mustPay(1, 1)
	if f.roundStatus(1) != "collecting" || f.system("escrow")-escrow0 != 200_000 {
		t.Fatalf("sebelum pembayaran terakhir: status=%s escrow+%d", f.roundStatus(1), f.system("escrow")-escrow0)
	}

	res := f.mustPay(1, 2) // terakhir
	if !res.Round.Completed || res.Round.GroupDone || res.Round.Payout == nil {
		t.Fatalf("round = %+v, want selesai dengan pencairan", res.Round)
	}
	if res.Round.Payout.Amount != 300_000 || res.Round.Payout.RecipientName != recipientName {
		t.Errorf("payout = %+v, want 300000 untuk %s", res.Round.Payout, recipientName)
	}

	if f.roundStatus(1) != "paid_out" || f.roundStatus(2) != "collecting" || f.roundStatus(3) != "upcoming" {
		t.Errorf("status putaran = %s/%s/%s", f.roundStatus(1), f.roundStatus(2), f.roundStatus(3))
	}
	// penerima: 500.000 - 100.000 + 300.000; pembayar lain: 400.000; escrow kembali kosong
	if f.bal(0) != 700_000 || f.bal(1) != 400_000 || f.bal(2) != 400_000 || res.Balance != 400_000 {
		t.Errorf("saldo = %d/%d/%d (res %d), want 700000/400000/400000", f.bal(0), f.bal(1), f.bal(2), res.Balance)
	}
	if f.system("escrow") != escrow0 {
		t.Errorf("escrow = %+d dari semula, want 0 (semua dana dicairkan)", f.system("escrow")-escrow0)
	}

	entries, _ := wallet.NewRepository(f.e.Pool).Ledger(f.ctx, f.user(0), 1, 0)
	if len(entries) == 0 || entries[0].Amount != 300_000 || entries[0].RefType != "payout" ||
		entries[0].RefID != f.roundID(1) || !strings.Contains(entries[0].Description, "Pencairan putaran 1") {
		t.Errorf("entri pencairan = %+v", entries)
	}
	f.requireReconciled()
}

func TestRecipientPayingLastReceivesNetAmount(t *testing.T) {
	f := newFx(t, testutil.GroupSpec{Slots: 3, Amount: 100_000}, 500_000, 0)
	f.mustPay(1, 1)
	f.mustPay(1, 2)

	res := f.mustPay(1, 0) // penerima giliran sekaligus pembayar terakhir
	if !res.Round.Completed {
		t.Fatalf("round = %+v", res.Round)
	}
	// 500.000 - 100.000 (iuran) + 300.000 (pencairan)
	if res.Balance != 700_000 || f.bal(0) != 700_000 {
		t.Errorf("saldo = %d / %d, want 700000 (hasil harus memuat pencairan)", res.Balance, f.bal(0))
	}
	if f.ledgerCount(0, "contribution") != 1 || f.ledgerCount(0, "payout") != 1 {
		t.Errorf("entri iuran=%d pencairan=%d, want 1 dan 1", f.ledgerCount(0, "contribution"), f.ledgerCount(0, "payout"))
	}
	f.requireReconciled()
}

func TestFullGroupLifecycle(t *testing.T) {
	f := newFx(t, testutil.GroupSpec{Slots: 3, Amount: 100_000}, 1_000_000, 250) // biaya 2.500 per iuran
	escrow0, platform0 := f.system("escrow"), f.system("platform")

	var last *Result
	for r := 1; r <= 3; r++ {
		for i := 0; i < 3; i++ {
			last = f.mustPay(r, i)
		}
		if !last.Round.Completed {
			t.Fatalf("putaran %d seharusnya selesai", r)
		}
		wantName := f.e.QueryString(`SELECT name FROM users WHERE id = $1`, f.user(r-1))
		if last.Round.Payout.RecipientName != wantName {
			t.Errorf("putaran %d: penerima %q, want %q (anggota ber-turn_order %d)", r, last.Round.Payout.RecipientName, wantName, r)
		}
		if last.Round.GroupDone != (r == 3) {
			t.Errorf("putaran %d: group_done = %v", r, last.Round.GroupDone)
		}
	}

	if st := f.e.QueryString(`SELECT status FROM groups WHERE id = $1`, f.g.ID); st != "done" {
		t.Errorf("status grup = %s, want done", st)
	}
	for r := 1; r <= 3; r++ {
		if f.roundStatus(r) != "paid_out" {
			t.Errorf("putaran %d = %s", r, f.roundStatus(r))
		}
	}
	// tiap anggota: bayar 3 x 102.500, terima 300.000 sekali => rugi tepat 7.500 (3 x biaya)
	for i := 0; i < 3; i++ {
		if f.bal(i) != 992_500 {
			t.Errorf("anggota %d: saldo %d, want 992500", i, f.bal(i))
		}
		if f.ledgerCount(i, "payout") != 1 || f.ledgerCount(i, "contribution") != 3 {
			t.Errorf("anggota %d: pencairan=%d iuran=%d, want 1 dan 3", i, f.ledgerCount(i, "payout"), f.ledgerCount(i, "contribution"))
		}
	}
	if d := f.system("platform") - platform0; d != 22_500 {
		t.Errorf("platform naik %d, want 22500 (9 iuran x 2.500)", d)
	}
	if f.system("escrow") != escrow0 {
		t.Errorf("escrow berubah %d, want 0", f.system("escrow")-escrow0)
	}
	f.requireReconciled()
}

// ---- konkurensi --------------------------------------------------------------

// Semua anggota membayar bersamaan: tepat satu yang menutup putaran dan penerima dibayar sekali.
// Tanpa kunci putaran, dua pembayar terakhir sama-sama melihat "masih ada yang belum membayar"
// sehingga tidak ada yang menutup putaran, dan dana tertahan di escrow selamanya.
func TestConcurrentPaymentsCompleteRoundExactlyOnce(t *testing.T) {
	for iter := 0; iter < 8; iter++ {
		f := newFx(t, testutil.GroupSpec{Slots: 6, Amount: 50_000}, 100_000, 0)
		escrow0 := f.system("escrow")

		start := make(chan struct{})
		results := make([]*Result, 6)
		errs := make([]error, 6)
		var wg sync.WaitGroup
		for i := 0; i < 6; i++ {
			cid := f.cid(1, i) // dihitung sebelum goroutine: tidak meminjam koneksi saat transaksi terbuka
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				results[i], errs[i] = f.repo.Pay(f.ctx, f.user(i), cid)
			}()
		}
		close(start)
		wg.Wait()

		completed := 0
		for i := range results {
			if errs[i] != nil {
				t.Fatalf("iterasi %d, anggota %d: %v", iter, i, errs[i])
			}
			if results[i].Round.Completed {
				completed++
			}
		}
		if completed != 1 {
			t.Fatalf("iterasi %d: %d pembayaran menutup putaran, want tepat 1", iter, completed)
		}
		if f.roundStatus(1) != "paid_out" || f.roundStatus(2) != "collecting" {
			t.Fatalf("iterasi %d: status putaran %s/%s", iter, f.roundStatus(1), f.roundStatus(2))
		}
		// penerima (anggota 0): 100.000 - 50.000 + 300.000
		if f.bal(0) != 350_000 || f.ledgerCount(0, "payout") != 1 {
			t.Fatalf("iterasi %d: saldo penerima %d, pencairan %d, want 350000 dan 1", iter, f.bal(0), f.ledgerCount(0, "payout"))
		}
		if f.system("escrow") != escrow0 {
			t.Fatalf("iterasi %d: escrow berubah %d (dana tertahan?)", iter, f.system("escrow")-escrow0)
		}
	}
}

// Pembayaran pada putaran yang sama diserialkan lewat baris putaran. Transaksi A memegang kunci
// putaran; pembayaran (bukan yang terakhir, jadi tidak menyentuh putaran di tempat lain) harus
// menunggu. Perilaku ini sengaja dikunci karena kebenaran penutupan putaran bergantung padanya,
// terlepas dari kunci dompet escrow yang kini juga menyerialkan pembayaran.
func TestPaySerializesOnRoundRow(t *testing.T) {
	f := newFx(t, testutil.GroupSpec{Slots: 3, Amount: 100_000}, 500_000, 0)
	cid := f.cid(1, 1)
	user := f.user(1)

	txA, err := f.e.Pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = txA.Rollback(f.ctx) }()
	if _, err := txA.Exec(f.ctx, `SELECT 1 FROM rounds WHERE id = $1 FOR UPDATE`, f.roundID(1)); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() { _, err := f.repo.Pay(f.ctx, user, cid); done <- err }()

	select {
	case err := <-done:
		t.Fatalf("Pay selesai (err=%v) padahal baris putaran dikunci transaksi lain; seharusnya menunggu", err)
	case <-time.After(300 * time.Millisecond):
	}
	if err := txA.Commit(f.ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Pay setelah kunci dilepas: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Pay tidak selesai setelah kunci dilepas")
	}
}

// R adalah penerima giliran di grup 1 dan sekaligus pembayar terakhir di grup 2; pada saat yang
// sama A menutup putaran grup 1 (pencairan ke R) dan R menutup putaran grup 2 (pencairan ke B).
// Tanpa penguncian dompet di awal, kedua transaksi mengunci dompet dalam urutan berlawanan
// dan saling menunggu (deadlock). Urutan id dompet acak, jadi dicoba berulang kali.
func TestNoDeadlockWhenRecipientPaysInAnotherGroup(t *testing.T) {
	e := testutil.NewEnv(t)
	repo := newRepo(e, 100) // biaya 1% supaya dompet platform ikut tersentuh
	ctx := context.Background()

	for iter := 0; iter < 25; iter++ {
		r, a, b := e.NewUser("r"), e.NewUser("a"), e.NewUser("b")
		for _, u := range []string{r, a, b} {
			e.FundUser(u, 10_000_000)
		}
		g1, m1 := e.NewGroupFor(testutil.GroupSpec{Amount: 100_000}, r, a) // penerima putaran 1: r
		g2, m2 := e.NewGroupFor(testutil.GroupSpec{Amount: 100_000}, b, r) // penerima putaran 1: b
		startGroup(t, e, g1, m1)
		startGroup(t, e, g2, m2)

		cid := func(g *group.Group, m group.Member) string {
			return e.QueryString(`SELECT c.id::text FROM contributions c JOIN rounds r ON r.id = c.round_id
			                       WHERE r.group_id = $1 AND r.round_no = 1 AND c.member_id = $2`, g.ID, m.ID)
		}
		// pembayar pertama di tiap grup
		if _, err := repo.Pay(ctx, r, cid(g1, m1[0])); err != nil {
			t.Fatal(err)
		}
		if _, err := repo.Pay(ctx, b, cid(g2, m2[0])); err != nil {
			t.Fatal(err)
		}
		cA, cR := cid(g1, m1[1]), cid(g2, m2[1])

		start := make(chan struct{})
		errs := make([]error, 2)
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); <-start; _, errs[0] = repo.Pay(ctx, a, cA) }()
		go func() { defer wg.Done(); <-start; _, errs[1] = repo.Pay(ctx, r, cR) }()
		close(start)
		wg.Wait()

		for i, err := range errs {
			if err != nil {
				t.Fatalf("iterasi %d, pembayar %d: %v (kemungkinan deadlock)", iter, i, err)
			}
		}
		for _, g := range []*group.Group{g1, g2} {
			if st := e.QueryString(`SELECT status FROM rounds WHERE group_id = $1 AND round_no = 1`, g.ID); st != "paid_out" {
				t.Fatalf("iterasi %d: putaran 1 grup %s = %s", iter, g.ID, st)
			}
		}
	}
	if rep, err := wallet.Reconcile(ctx, e.Pool); err != nil || !rep.Ok {
		t.Fatalf("rekonsiliasi: %v %+v", err, rep)
	}
}

// ---- atomisitas --------------------------------------------------------------

// Jika pencairan gagal, pembayaran terakhir ikut batal seluruhnya: pembayar tidak terpotong,
// iuran tetap belum lunas, putaran tetap terbuka.
func TestPayIsAtomicWhenPayoutFails(t *testing.T) {
	f := newFx(t, testutil.GroupSpec{Slots: 2, Amount: 100_000}, 500_000, 0)
	f.mustPay(1, 1) // escrow kini berisi 100.000

	// simulasi anomali: referensi pencairan putaran ini sudah tercatat sebelumnya
	escrow, _ := wallet.SystemWallet(f.ctx, f.e.Pool, wallet.TypeEscrow)
	recipientWallet, _ := wallet.EnsureUserWallet(f.ctx, f.e.Pool, f.user(0))
	rid := f.roundID(1)
	err := f.e.InTx(func(tx pgx.Tx) error {
		_, err := wallet.Post(f.ctx, tx, wallet.Ref{Type: "payout", ID: rid},
			wallet.Leg{WalletID: escrow, Amount: 1}, wallet.Leg{WalletID: recipientWallet, Amount: -1})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	escrow0, balance0, entries0 := f.system("escrow"), f.bal(0), f.ledgerCount(0, "contribution")
	cid := f.cid(1, 0)

	if _, err := f.repo.Pay(f.ctx, f.user(0), cid); !errors.Is(err, wallet.ErrAlreadyPosted) {
		t.Fatalf("err = %v, want ErrAlreadyPosted", err)
	}
	if st := f.e.QueryString(`SELECT status FROM contributions WHERE id = $1`, cid); st != "unpaid" {
		t.Errorf("status iuran = %s, want unpaid", st)
	}
	if f.bal(0) != balance0 || f.system("escrow") != escrow0 || f.ledgerCount(0, "contribution") != entries0 {
		t.Errorf("saldo=%d (semula %d) escrow=%d (semula %d); pembayaran yang gagal tidak boleh meninggalkan jejak",
			f.bal(0), balance0, f.system("escrow"), escrow0)
	}
	if f.roundStatus(1) != "collecting" || f.roundStatus(2) != "upcoming" {
		t.Errorf("status putaran = %s/%s; harus tetap terbuka", f.roundStatus(1), f.roundStatus(2))
	}
	f.requireReconciled()
}

func TestPayFailsCleanlyWhenFeeSourceFails(t *testing.T) {
	f := newFx(t, testutil.GroupSpec{Slots: 3, Amount: 100_000}, 500_000, 0)
	boom := errors.New("pengaturan rusak")
	f.repo.fees = func(context.Context, settings.Querier) (int, error) { return 0, boom }
	cid := f.cid(1, 1)

	if _, err := f.repo.Pay(f.ctx, f.user(1), cid); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
	if st := f.e.QueryString(`SELECT status FROM contributions WHERE id = $1`, cid); st != "unpaid" || f.bal(1) != 500_000 {
		t.Errorf("status = %s saldo = %d; harus utuh", st, f.bal(1))
	}
}

// ---- kuotasi -----------------------------------------------------------------

func TestQuote(t *testing.T) {
	f := newFx(t, testutil.GroupSpec{Slots: 3, Amount: 100_000}, 0, 250) // total 102.500
	f.e.FundUser(f.user(1), 102_499)
	cid := f.cid(1, 1)

	q, err := f.repo.Quote(f.ctx, f.user(1), cid)
	if err != nil {
		t.Fatal(err)
	}
	if q.Quote.Total != 102_500 || q.Quote.PlatformFee != 2_500 || !q.Quote.Payable || q.Balance != 102_499 || q.Sufficient {
		t.Errorf("quote = %+v %+v", q, q.Quote)
	}
	f.e.FundUser(f.user(1), 1)
	if q, _ := f.repo.Quote(f.ctx, f.user(1), cid); !q.Sufficient || q.Balance != 102_500 {
		t.Errorf("setelah ditambah: %+v", q)
	}
	// kuotasi tidak mengubah apa pun
	if st := f.e.QueryString(`SELECT status FROM contributions WHERE id = $1`, cid); st != "unpaid" || f.bal(1) != 102_500 {
		t.Errorf("status = %s saldo = %d", st, f.bal(1))
	}

	if _, err := f.repo.Quote(f.ctx, f.user(2), cid); !errors.Is(err, contribution.ErrNotFound) {
		t.Errorf("bukan pemilik: err = %v, want ErrNotFound", err)
	}

	f.mustPay(1, 1)
	if q, _ := f.repo.Quote(f.ctx, f.user(1), cid); q.Quote.Payable || q.Quote.Reason != "sudah dibayar" || !q.Sufficient {
		t.Errorf("sudah dibayar: %+v %+v", q, q.Quote)
	}
	if q, _ := f.repo.Quote(f.ctx, f.user(1), f.cid(2, 1)); q.Quote.Payable || q.Quote.Reason != "putaran belum dibuka" {
		t.Errorf("putaran berikutnya: %+v", q.Quote)
	}
}

// Sumber biaya bawaan membaca pengaturan platform yang berlaku.
func TestDefaultFeeSourceReadsPlatformSettings(t *testing.T) {
	e := testutil.NewEnv(t)
	g, members := e.NewGroup(testutil.GroupSpec{Slots: 2, Amount: 100_000})
	startGroup(t, e, g, members)
	cid := e.QueryString(`SELECT c.id::text FROM contributions c JOIN rounds r ON r.id = c.round_id
	                       WHERE r.group_id = $1 AND r.round_no = 1 AND c.member_id = $2`, g.ID, members[1].ID)

	cfg, err := settings.Load(context.Background(), e.Pool)
	if err != nil {
		t.Fatal(err)
	}
	q, err := NewRepository(e.Pool).Quote(context.Background(), members[1].UserID, cid)
	if err != nil {
		t.Fatal(err)
	}
	if want := contribution.PlatformFee(100_000, cfg.PlatformFeeBps); q.Quote.PlatformFee != want {
		t.Errorf("biaya = %d, want %d (bps %d dari pengaturan)", q.Quote.PlatformFee, want, cfg.PlatformFeeBps)
	}
}
