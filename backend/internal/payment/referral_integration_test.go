/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package payment

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/alexistdev/geopatungan/internal/group"
	"github.com/alexistdev/geopatungan/internal/testutil"
	"github.com/alexistdev/geopatungan/internal/wallet"
)

// Aturan bawaan: level 1..4 = 40% / 25% / 15% / 10% dari BIAYA PLATFORM.

// chain membuat rantai A -> B -> C -> D -> E (A mengajak B, dst.). Mengembalikan [A B C D E].
func chain(e *testutil.Env) []string {
	a := e.NewUser("a")
	ids := []string{a}
	for _, n := range []string{"b", "c", "d", "e"} {
		ids = append(ids, e.NewReferredUser(n, ids[len(ids)-1]))
	}
	return ids
}

// referralFx: E (pembayar) dan F berada dalam satu grup berisi 2 orang; iuran 100.000, biaya 2%
// sehingga biaya platform per iuran = 2.000. Penerima putaran 1 = E, jadi F tidak punya upline.
type referralFx struct {
	*fx
	ch []string // A B C D E
}

func newReferralFx(t *testing.T, bps int) *referralFx {
	t.Helper()
	e := testutil.NewEnv(t)
	ch := chain(e)
	f := e.NewUser("f")
	g, members := e.NewGroupFor(testutil.GroupSpec{Amount: 100_000}, ch[4], f)
	startGroup(t, e, g, members)
	for _, u := range append(append([]string{}, ch...), f) {
		e.FundUser(u, 1_000_000)
	}
	return &referralFx{fx: &fx{e: e, repo: newRepo(e, bps), ctx: context.Background(), g: g, members: members}, ch: ch}
}

func (r *referralFx) commissionRows() int {
	return r.e.QueryInt(`SELECT count(*) FROM referral_commissions WHERE source_user_id = $1`, r.ch[4])
}

func TestCommissionPaidToFourUplinesFromPlatformFee(t *testing.T) {
	r := newReferralFx(t, 200)
	r.e.Verify(r.ch[:4]...)
	platform0, escrow0 := r.system("platform"), r.system("escrow")
	bal0 := map[string]int64{}
	for _, u := range r.ch {
		bal0[u] = r.e.Balance(u)
	}

	res := r.mustPay(1, 0) // E membayar: 100.000 + biaya 2.000
	if res.Receipt.PlatformFee != 2_000 {
		t.Fatalf("biaya platform = %d", res.Receipt.PlatformFee)
	}
	// D = level 1, C = level 2, B = level 3, A = level 4
	want := map[string]int64{r.ch[3]: 800, r.ch[2]: 500, r.ch[1]: 300, r.ch[0]: 200}
	for u, w := range want {
		if d := r.e.Balance(u) - bal0[u]; d != w {
			t.Errorf("komisi %s = %d, want %d", u, d, w)
		}
	}
	if d := r.system("platform") - platform0; d != 200 { // 2.000 - 1.800
		t.Errorf("platform naik %d, want 200", d)
	}
	if d := r.system("escrow") - escrow0; d != 100_000 {
		t.Errorf("escrow naik %d: komisi tidak boleh menyentuh dana arisan", d)
	}
	// pembayar membayar persis iuran + biaya, tidak lebih
	if d := bal0[r.ch[4]] - r.e.Balance(r.ch[4]); d != 102_000 {
		t.Errorf("pembayar terpotong %d, want 102000", d)
	}
	if n := r.commissionRows(); n != 4 {
		t.Errorf("baris komisi = %d, want 4", n)
	}
	var sum int64
	if err := r.e.Pool.QueryRow(r.ctx, `SELECT sum(amount) FROM referral_commissions WHERE source_user_id = $1`, r.ch[4]).Scan(&sum); err != nil || sum != 1_800 {
		t.Errorf("jumlah tercatat = %d, %v", sum, err)
	}
	if n := r.e.QueryInt(`SELECT count(*) FROM ledger_entries WHERE ref_type = 'referral_commission'
	                       AND wallet_id IN (SELECT id FROM wallets WHERE user_id = ANY($1::uuid[]))`, r.ch[:4]); n != 4 {
		t.Errorf("entri ledger komisi = %d, want 4", n)
	}
	r.requireReconciled()
}

func TestOnlyVerifiedUplinesReceiveCommission(t *testing.T) {
	r := newReferralFx(t, 200)
	r.e.Verify(r.ch[3], r.ch[1]) // D (level 1) dan B (level 3); C dan A belum
	platform0 := r.system("platform")
	c0, a0 := r.e.Balance(r.ch[2]), r.e.Balance(r.ch[0])

	r.mustPay(1, 0)
	if r.e.Balance(r.ch[2]) != c0 || r.e.Balance(r.ch[0]) != a0 {
		t.Error("upline yang belum terverifikasi tidak boleh menerima komisi")
	}
	if n := r.commissionRows(); n != 2 {
		t.Errorf("baris komisi = %d, want 2", n)
	}
	// bagian yang tidak dibayarkan tetap di platform: 2.000 - 800 - 300
	if d := r.system("platform") - platform0; d != 900 {
		t.Errorf("platform naik %d, want 900", d)
	}
	r.requireReconciled()
}

func TestNoCommissionWithoutPlatformFee(t *testing.T) {
	r := newReferralFx(t, 0)
	r.e.Verify(r.ch[:4]...)
	r.mustPay(1, 0)
	if n := r.commissionRows(); n != 0 {
		t.Errorf("baris komisi = %d, want 0 (tidak ada biaya platform)", n)
	}
	if n := r.e.QueryInt(`SELECT count(*) FROM ledger_entries WHERE ref_type = 'referral_commission'
	                       AND wallet_id IN (SELECT id FROM wallets WHERE user_id = ANY($1::uuid[]))`, r.ch[:4]); n != 0 {
		t.Errorf("entri ledger komisi = %d, want 0", n)
	}
}

// Denda keterlambatan bukan sumber komisi: hanya biaya platform.
func TestLateFeeIsNotCommissionable(t *testing.T) {
	r := newReferralFx(t, 200)
	r.e.Verify(r.ch[:4]...)
	r.e.Exec(`UPDATE groups SET late_fee = 50000 WHERE id = $1`, r.g.ID)
	r.e.Exec(`UPDATE rounds SET due_date = '2020-01-01' WHERE group_id = $1 AND round_no = 1`, r.g.ID)
	platform0 := r.system("platform")

	res := r.mustPay(1, 0)
	if res.Receipt.LateFee == 0 {
		t.Fatalf("denda seharusnya terkena: %+v", res.Receipt)
	}
	var sum int64
	_ = r.e.Pool.QueryRow(r.ctx, `SELECT sum(amount) FROM referral_commissions WHERE source_user_id = $1`, r.ch[4]).Scan(&sum)
	if sum != 1_800 {
		t.Errorf("komisi = %d, want 1800 (dari biaya 2000 saja)", sum)
	}
	if d := r.system("platform") - platform0; d != 2_000+res.Receipt.LateFee-1_800 {
		t.Errorf("platform naik %d", d)
	}
	r.requireReconciled()
}

func TestNoCommissionForPayerWithoutReferrer(t *testing.T) {
	r := newReferralFx(t, 200)
	r.e.Verify(r.ch[:4]...)
	platform0 := r.system("platform")
	r.mustPay(1, 1) // F tidak diajak siapa pun
	if d := r.system("platform") - platform0; d != 2_000 {
		t.Errorf("platform naik %d, want seluruh biaya 2000", d)
	}
	if r.commissionRows() != 0 {
		t.Error("tidak boleh ada komisi")
	}
}

// Komisi memakai aturan yang berlaku saat pembayaran; mengubah aturan tidak mengubah yang lalu.
func TestCommissionFollowsCurrentRulesAndKeepsHistory(t *testing.T) {
	r := newReferralFx(t, 200)
	r.e.Verify(r.ch[:4]...)
	t.Cleanup(func() {
		r.e.Exec(`UPDATE commission_rules SET share_bps = CASE level WHEN 1 THEN 4000 WHEN 2 THEN 2500 WHEN 3 THEN 1500 ELSE 1000 END`)
	})
	r.e.Exec(`UPDATE commission_rules SET share_bps = CASE level WHEN 1 THEN 1000 ELSE 0 END`)

	r.mustPay(1, 0)
	if n := r.commissionRows(); n != 1 {
		t.Fatalf("baris komisi = %d, want 1 (hanya level 1 yang bagiannya > 0)", n)
	}
	var amt int64
	var share int
	if err := r.e.Pool.QueryRow(r.ctx, `SELECT amount, share_bps FROM referral_commissions WHERE source_user_id = $1`, r.ch[4]).Scan(&amt, &share); err != nil {
		t.Fatal(err)
	}
	if amt != 200 || share != 1000 {
		t.Errorf("amount=%d share=%d, want 200/1000", amt, share)
	}
	r.requireReconciled()
}

// Pembayaran terakhir menutup putaran, mencairkan dana, DAN membayar komisi; semuanya konsisten.
func TestCommissionWithRoundCompletion(t *testing.T) {
	r := newReferralFx(t, 200)
	r.e.Verify(r.ch[:4]...)
	r.mustPay(1, 1) // F dulu (tanpa komisi)
	res := r.mustPay(1, 0)
	if !res.Round.Completed || res.Round.Payout == nil {
		t.Fatalf("putaran seharusnya selesai: %+v", res.Round)
	}
	if n := r.commissionRows(); n != 4 {
		t.Errorf("baris komisi = %d, want 4", n)
	}
	// seluruh uang: escrow kosong setelah pencairan, jumlah global tetap nol
	if r.system("escrow") != 0 {
		t.Errorf("escrow = %d, want 0", r.system("escrow"))
	}
	r.requireReconciled()
}

// Jika pencatatan komisi gagal, seluruh pembayaran batal: pembayar tidak terpotong dan tidak ada
// upline yang menerima apa pun.
func TestPayIsAtomicWhenCommissionPostFails(t *testing.T) {
	r := newReferralFx(t, 200)
	r.e.Verify(r.ch[:4]...)
	cid := r.cid(1, 0)

	external, _ := wallet.SystemWallet(r.ctx, r.e.Pool, wallet.TypeExternal)
	dWallet, _ := wallet.EnsureUserWallet(r.ctx, r.e.Pool, r.ch[3])
	// simulasi anomali: referensi komisi untuk iuran ini sudah tercatat
	err := r.e.InTx(func(tx pgx.Tx) error {
		_, err := wallet.Post(r.ctx, tx, wallet.Ref{Type: "referral_commission", ID: cid},
			wallet.Leg{WalletID: external, Amount: -1}, wallet.Leg{WalletID: dWallet, Amount: 1})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	payer0, escrow0, d0 := r.bal(0), r.system("escrow"), r.e.Balance(r.ch[3])
	platform0 := r.system("platform")

	if _, err := r.repo.Pay(r.ctx, r.ch[4], cid); !errors.Is(err, wallet.ErrAlreadyPosted) {
		t.Fatalf("err = %v, want ErrAlreadyPosted", err)
	}
	if st := r.e.QueryString(`SELECT status FROM contributions WHERE id = $1`, cid); st != "unpaid" {
		t.Errorf("status iuran = %s, want unpaid", st)
	}
	if r.bal(0) != payer0 || r.system("escrow") != escrow0 || r.system("platform") != platform0 || r.e.Balance(r.ch[3]) != d0 {
		t.Error("pembayaran yang gagal meninggalkan jejak pada saldo")
	}
	if r.commissionRows() != 0 {
		t.Error("baris komisi tidak boleh tersimpan")
	}
	r.requireReconciled()
}

// Upline membayar di grup lain pada saat yang sama dengan downline-nya. Tanpa penguncian dompet
// upline di awal, kedua transaksi menunggu satu sama lain (deadlock). Diulang karena urutan id acak.
func TestNoDeadlockWhenUplineIsPayingElsewhere(t *testing.T) {
	e := testutil.NewEnv(t)
	repo := newRepo(e, 100)
	ctx := context.Background()

	for iter := 0; iter < 25; iter++ {
		u := e.NewUser("u")                    // upline
		p := e.NewReferredUser("p", u)         // downline level 1
		s, x := e.NewUser("s"), e.NewUser("x") // rekan di grup masing-masing
		e.Verify(u)
		for _, id := range []string{u, p, s, x} {
			e.FundUser(id, 10_000_000)
		}
		// g1: penerima putaran 1 = s; p menutup putaran (komisi ke u + pencairan ke s)
		g1, m1 := e.NewGroupFor(testutil.GroupSpec{Amount: 100_000}, s, p)
		// g2: penerima putaran 1 = p; u menutup putaran (pencairan ke p)
		g2, m2 := e.NewGroupFor(testutil.GroupSpec{Amount: 100_000}, p, u)
		startGroup(t, e, g1, m1)
		startGroup(t, e, g2, m2)

		cid := func(g *group.Group, m group.Member) string {
			return e.QueryString(`SELECT c.id::text FROM contributions c JOIN rounds r ON r.id = c.round_id
			                       WHERE r.group_id = $1 AND r.round_no = 1 AND c.member_id = $2`, g.ID, m.ID)
		}
		if _, err := repo.Pay(ctx, s, cid(g1, m1[0])); err != nil {
			t.Fatal(err)
		}
		if _, err := repo.Pay(ctx, p, cid(g2, m2[0])); err != nil {
			t.Fatal(err)
		}
		cP, cU := cid(g1, m1[1]), cid(g2, m2[1])

		start := make(chan struct{})
		errs := make([]error, 2)
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); <-start; _, errs[0] = repo.Pay(ctx, p, cP) }()
		go func() { defer wg.Done(); <-start; _, errs[1] = repo.Pay(ctx, u, cU) }()
		close(start)
		wg.Wait()

		for i, err := range errs {
			if err != nil {
				t.Fatalf("iterasi %d, pembayar %d: %v (kemungkinan deadlock)", iter, i, err)
			}
		}
		if n := e.QueryInt(`SELECT count(*) FROM referral_commissions WHERE beneficiary_id = $1`, u); n < 1 {
			t.Fatalf("iterasi %d: komisi upline tidak tercatat", iter)
		}
	}
	if rep, err := wallet.Reconcile(ctx, e.Pool); err != nil || !rep.Ok {
		t.Fatalf("rekonsiliasi: %v %+v", err, rep)
	}
}

// Banyak downline membayar bersamaan dan semuanya memberi komisi ke upline yang sama: tidak ada
// komisi hilang atau ganda, saldo upline = jumlah komisi.
func TestConcurrentCommissionsToSameUpline(t *testing.T) {
	e := testutil.NewEnv(t)
	repo := newRepo(e, 200)
	ctx := context.Background()

	up := e.NewUser("up")
	e.Verify(up)
	const n = 6
	downs := make([]string, n)
	for i := range downs {
		downs[i] = e.NewReferredUser("d", up)
		e.FundUser(downs[i], 1_000_000)
	}
	g, members := e.NewGroupFor(testutil.GroupSpec{Amount: 100_000}, downs...)
	startGroup(t, e, g, members)
	cids := make([]string, n)
	for i, m := range members {
		cids[i] = e.QueryString(`SELECT c.id::text FROM contributions c JOIN rounds r ON r.id = c.round_id
		                          WHERE r.group_id = $1 AND r.round_no = 1 AND c.member_id = $2`, g.ID, m.ID)
	}
	up0 := e.Balance(up)

	start := make(chan struct{})
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := range members {
		wg.Add(1)
		go func(i int) { defer wg.Done(); <-start; _, errs[i] = repo.Pay(ctx, members[i].UserID, cids[i]) }(i)
	}
	close(start)
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("pembayar %d: %v", i, err)
		}
	}
	// tiap downline = level 1 -> 40% x 2.000 = 800
	if got := e.Balance(up) - up0; got != n*800 {
		t.Errorf("saldo upline naik %d, want %d", got, n*800)
	}
	if c := e.QueryInt(`SELECT count(*) FROM referral_commissions WHERE beneficiary_id = $1`, up); c != n {
		t.Errorf("baris komisi = %d, want %d", c, n)
	}
	if rep, err := wallet.Reconcile(ctx, e.Pool); err != nil || !rep.Ok {
		t.Fatalf("rekonsiliasi: %v %+v", err, rep)
	}
}

// Deterministik: dompet upline dikunci SEBELUM ada tulisan apa pun. Dompet upline ditahan oleh
// transaksi lain; Pay yang tertahan olehnya tidak boleh sudah menulis ke ledger (artinya ia
// sedang menunggu di tahap penguncian awal, belum memegang kunci lain sambil menulis).
// Tanpa penguncian awal, Pay menulis pembayaran lebih dulu dan baru terhenti saat membayar komisi,
// dengan kunci dompet pembayar/penerima masih dipegang, itulah bahan deadlock.
func TestUplineWalletIsLockedBeforeAnyWrite(t *testing.T) {
	r := newReferralFx(t, 200)
	r.e.Verify(r.ch[:4]...)
	upline := r.ch[3]
	uw, err := wallet.EnsureUserWallet(r.ctx, r.e.Pool, upline)
	if err != nil {
		t.Fatal(err)
	}

	hold, err := r.e.Pool.Begin(r.ctx)
	if err != nil {
		t.Fatal(err)
	}
	released := false
	release := func() {
		if !released {
			released = true
			_ = hold.Rollback(r.ctx)
		}
	}
	defer release()
	if _, err := hold.Exec(r.ctx, `SELECT 1 FROM wallets WHERE id = $1 FOR UPDATE`, uw); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() { _, err := r.repo.Pay(r.ctx, r.ch[4], r.cid(1, 0)); done <- err }()

	var blocked int
	for i := 0; i < 100 && blocked == 0; i++ { // tunggu sampai Pay benar-benar tertahan (maks ~5 dtk)
		blocked = r.e.QueryInt(`SELECT COALESCE(max(pid), 0) FROM pg_stat_activity
		                         WHERE datname = current_database() AND wait_event_type = 'Lock' AND pid <> pg_backend_pid()`)
		if blocked == 0 {
			time.Sleep(50 * time.Millisecond)
		}
	}
	if blocked == 0 {
		t.Fatal("Pay tidak tertahan oleh kunci dompet upline")
	}
	wrote := r.e.QueryInt(`SELECT count(*) FROM pg_locks WHERE pid = $1 AND granted
	                        AND relation = 'ledger_entries'::regclass AND mode = 'RowExclusiveLock'`, blocked)
	if wrote != 0 {
		t.Error("Pay sudah menulis ke ledger sebelum mengunci dompet upline")
	}

	release()
	if err := <-done; err != nil {
		t.Fatalf("Pay setelah kunci dilepas: %v", err)
	}
	if n := r.commissionRows(); n != 4 {
		t.Errorf("baris komisi = %d, want 4", n)
	}
}
