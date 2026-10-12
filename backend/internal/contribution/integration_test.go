/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package contribution

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/alexistdev/geopatungan/internal/group"
	"github.com/alexistdev/geopatungan/internal/round"
	"github.com/alexistdev/geopatungan/internal/testutil"
)

// Tes ini butuh database Postgres dengan semua migrasi; dilewati jika TEST_DATABASE_URL kosong.

type fixture struct {
	e       *testutil.Env
	g       *group.Group
	members []group.Member
}

// newFixture membuat grup berjalan: urutan giliran sudah ditetapkan dan jadwal dibuat.
func newFixture(t *testing.T, spec testutil.GroupSpec) *fixture {
	t.Helper()
	e := testutil.NewEnv(t)
	g, members := e.NewGroup(spec)
	e.SetTurnOrders(members)
	e.Exec(`UPDATE groups SET status = 'running' WHERE id = $1`, g.ID)
	err := e.InTx(func(tx pgx.Tx) error { return round.CreateSchedule(context.Background(), tx, g.ID) })
	if err != nil {
		t.Fatalf("CreateSchedule: %v", err)
	}
	return &fixture{e: e, g: g, members: members}
}

func (f *fixture) roundID(no int) string {
	return f.e.QueryString(`SELECT id::text FROM rounds WHERE group_id = $1 AND round_no = $2`, f.g.ID, no)
}

func (f *fixture) contributionID(roundNo int, member group.Member) string {
	return f.e.QueryString(`SELECT c.id::text FROM contributions c JOIN rounds r ON r.id = c.round_id
	                         WHERE r.group_id = $1 AND r.round_no = $2 AND c.member_id = $3`, f.g.ID, roundNo, member.ID)
}

// payer mengembalikan user_id pemilik iuran.
func (f *fixture) payer(contributionID string) string {
	return f.e.QueryString(`SELECT m.user_id::text FROM contributions c JOIN group_members m ON m.id = c.member_id WHERE c.id = $1`, contributionID)
}

func (f *fixture) pay(id string, at time.Time) (*Receipt, error) {
	payer := f.payer(id) // sebelum transaksi: meminjam koneksi kedua saat transaksi terbuka bisa membuntukan pool
	var rc *Receipt
	err := f.e.InTx(func(tx pgx.Tx) error {
		var err error
		rc, err = MarkPaid(context.Background(), tx, id, payer, at, 0)
		return err
	})
	return rc, err
}

func (f *fixture) setDue(roundNo int, date string) {
	f.e.Exec(`UPDATE rounds SET due_date = $3::date WHERE group_id = $1 AND round_no = $2`, f.g.ID, roundNo, date)
}

// ---- MarkPaid ---------------------------------------------------------------

func TestMarkPaidOnTime(t *testing.T) {
	f := newFixture(t, testutil.GroupSpec{Slots: 3, Amount: 200_000, LateFee: 10_000})
	f.setDue(1, "2026-12-01")
	id := f.contributionID(1, f.members[1])

	rc, err := f.pay(id, time.Date(2026, 12, 1, 20, 0, 0, 0, wib))
	if err != nil {
		t.Fatalf("MarkPaid: %v", err)
	}
	if !rc.PaidOnTime || rc.LateFee != 0 || rc.Total != 200_000 {
		t.Errorf("receipt = %+v, want tepat waktu, tanpa denda, total 200000", rc)
	}

	var status string
	var onTime bool
	var fee int64
	var paidAt *time.Time
	err = f.e.Pool.QueryRow(context.Background(),
		`SELECT status, paid_on_time, late_fee, paid_at FROM contributions WHERE id = $1`, id,
	).Scan(&status, &onTime, &fee, &paidAt)
	if err != nil {
		t.Fatal(err)
	}
	if status != "paid" || !onTime || fee != 0 || paidAt == nil {
		t.Errorf("DB: status=%s onTime=%v fee=%d paidAt=%v", status, onTime, fee, paidAt)
	}
}

func TestMarkPaidLateChargesGroupLateFee(t *testing.T) {
	f := newFixture(t, testutil.GroupSpec{Slots: 3, Amount: 200_000, LateFee: 10_000})
	f.setDue(1, "2026-12-01")
	id := f.contributionID(1, f.members[2])

	rc, err := f.pay(id, time.Date(2026, 12, 3, 9, 0, 0, 0, wib))
	if err != nil {
		t.Fatal(err)
	}
	if rc.PaidOnTime || rc.LateFee != 10_000 || rc.Total != 210_000 {
		t.Errorf("receipt = %+v, want terlambat, denda 10000, total 210000", rc)
	}
	var fee int64
	var onTime bool
	if err := f.e.Pool.QueryRow(context.Background(),
		`SELECT late_fee, paid_on_time FROM contributions WHERE id = $1`, id).Scan(&fee, &onTime); err != nil {
		t.Fatal(err)
	}
	if fee != 10_000 || onTime {
		t.Errorf("DB: late_fee=%d paid_on_time=%v", fee, onTime)
	}
}

func TestMarkPaidRejections(t *testing.T) {
	f := newFixture(t, testutil.GroupSpec{Slots: 3})
	now := time.Now()

	t.Run("sudah dibayar", func(t *testing.T) {
		id := f.contributionID(1, f.members[0])
		if _, err := f.pay(id, now); err != nil {
			t.Fatal(err)
		}
		if _, err := f.pay(id, now); !errors.Is(err, ErrAlreadyPaid) {
			t.Errorf("err = %v, want ErrAlreadyPaid", err)
		}
	})
	t.Run("putaran belum dibuka (upcoming)", func(t *testing.T) {
		id := f.contributionID(2, f.members[0])
		if _, err := f.pay(id, now); !errors.Is(err, round.ErrRoundNotCollecting) {
			t.Errorf("err = %v, want ErrRoundNotCollecting", err)
		}
		if st := f.e.QueryString(`SELECT status FROM contributions WHERE id = $1`, id); st != "unpaid" {
			t.Errorf("status = %s, want unpaid", st)
		}
	})
	t.Run("putaran sudah ditutup (paid_out)", func(t *testing.T) {
		f.e.Exec(`UPDATE rounds SET status = 'paid_out' WHERE group_id = $1 AND round_no = 3`, f.g.ID)
		id := f.contributionID(3, f.members[0])
		if _, err := f.pay(id, now); !errors.Is(err, round.ErrRoundNotCollecting) {
			t.Errorf("err = %v, want ErrRoundNotCollecting", err)
		}
	})
	t.Run("id tidak ada", func(t *testing.T) {
		if _, err := f.payWith("00000000-0000-0000-0000-000000000000", f.payer(f.contributionID(1, f.members[0])), now, 0); !errors.Is(err, ErrNotFound) {
			t.Errorf("err = %v, want ErrNotFound", err)
		}
	})
}

// MarkPaid berjalan di transaksi pemanggil: jika transaksi dibatalkan (mis. pencatatan
// ledger gagal), iuran harus kembali belum dibayar.
func TestMarkPaidRollsBackWithCallerTransaction(t *testing.T) {
	f := newFixture(t, testutil.GroupSpec{Slots: 3})
	id := f.contributionID(1, f.members[0])

	boom := errors.New("ledger gagal")
	payer := f.payer(id)
	err := f.e.InTx(func(tx pgx.Tx) error {
		if _, err := MarkPaid(context.Background(), tx, id, payer, time.Now(), 0); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	if st := f.e.QueryString(`SELECT status FROM contributions WHERE id = $1`, id); st != "unpaid" {
		t.Errorf("status = %s, want unpaid setelah rollback", st)
	}
	if _, err := f.pay(id, time.Now()); err != nil { // dan masih bisa dibayar sesudahnya
		t.Errorf("pembayaran ulang setelah rollback: %v", err)
	}
}

// Pembayaran ganda serentak untuk iuran yang sama: tepat satu yang berhasil.
func TestMarkPaidConcurrentPaysOnce(t *testing.T) {
	f := newFixture(t, testutil.GroupSpec{Slots: 3})
	id := f.contributionID(1, f.members[0])

	const n = 8
	results := make([]error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, results[i] = f.pay(id, time.Now())
		}()
	}
	wg.Wait()

	ok := 0
	for _, err := range results {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, ErrAlreadyPaid):
		default:
			t.Errorf("error tak terduga: %v", err)
		}
	}
	if ok != 1 {
		t.Errorf("%d pembayaran berhasil, want tepat 1", ok)
	}
}

// Transaksi A melunasi iuran tetapi belum commit; pembayaran kedua yang dimulai saat itu
// harus menunggu lalu gagal ErrAlreadyPaid. Tanpa FOR UPDATE ia membaca status lama
// ("unpaid") dan membayar dua kali. (Tes balapan di atas bergantung waktu.)
func TestMarkPaidLocksContributionRow(t *testing.T) {
	f := newFixture(t, testutil.GroupSpec{Slots: 3})
	id := f.contributionID(1, f.members[0])

	ctx := context.Background()
	txA, err := f.e.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = txA.Rollback(ctx) }()
	if _, err := MarkPaid(ctx, txA, id, f.payer(id), time.Now(), 0); err != nil { // txA sudah terbuka; satu koneksi tambahan masih aman (tanpa goroutine lain)
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() { _, err := f.pay(id, time.Now()); done <- err }()

	select {
	case err := <-done:
		t.Fatalf("pembayaran kedua selesai (err=%v) padahal A belum commit; seharusnya menunggu", err)
	case <-time.After(300 * time.Millisecond):
	}
	if err := txA.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, ErrAlreadyPaid) {
			t.Errorf("err = %v, want ErrAlreadyPaid", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("pembayaran kedua tidak selesai setelah A commit")
	}
}

// ---- alur penuh: bayar semua -> tutup putaran -> putaran berikutnya terbuka --------

func TestFullRoundFlow(t *testing.T) {
	f := newFixture(t, testutil.GroupSpec{Slots: 3, Amount: 100_000})
	ctx := context.Background()
	now := time.Now()

	// putaran 2 belum bisa dibayar
	if _, err := f.pay(f.contributionID(2, f.members[0]), now); !errors.Is(err, round.ErrRoundNotCollecting) {
		t.Fatalf("putaran 2 sebelum waktunya: err = %v", err)
	}

	// dua dari tiga anggota membayar putaran 1: belum bisa ditutup
	for _, m := range f.members[:2] {
		if _, err := f.pay(f.contributionID(1, m), now); err != nil {
			t.Fatal(err)
		}
	}
	closeRound := func(no int) (bool, error) {
		var done bool
		err := f.e.InTx(func(tx pgx.Tx) error {
			var cerr error
			done, cerr = round.Complete(ctx, tx, f.roundID(no))
			return cerr
		})
		return done, err
	}
	if _, err := closeRound(1); !errors.Is(err, round.ErrContributionsPending) {
		t.Fatalf("err = %v, want ErrContributionsPending", err)
	}

	// anggota ketiga membayar -> bisa ditutup, putaran 2 terbuka dan kini bisa dibayar
	if _, err := f.pay(f.contributionID(1, f.members[2]), now); err != nil {
		t.Fatal(err)
	}
	if done, err := closeRound(1); err != nil || done {
		t.Fatalf("tutup putaran 1: done=%v err=%v", done, err)
	}
	if _, err := f.pay(f.contributionID(2, f.members[0]), now); err != nil {
		t.Errorf("putaran 2 seharusnya bisa dibayar setelah putaran 1 ditutup: %v", err)
	}
}

// ---- membaca daftar ---------------------------------------------------------

func TestListByRound(t *testing.T) {
	f := newFixture(t, testutil.GroupSpec{Slots: 3, Amount: 150_000})
	repo := NewRepository(f.e.Pool)
	ctx := context.Background()

	f.setDue(1, "2020-01-01") // lewat jatuh tempo
	paidID := f.contributionID(1, f.members[0])
	if _, err := f.pay(paidID, time.Date(2020, 1, 1, 8, 0, 0, 0, wib)); err != nil {
		t.Fatal(err)
	}

	list, err := repo.ListByRound(ctx, f.g.ID, f.roundID(1))
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 {
		t.Fatalf("len = %d, want 3", len(list))
	}
	overdue, paid := 0, 0
	for _, c := range list {
		if c.RoundNo != 1 || c.DueDate != "2020-01-01" || c.Amount != 150_000 || c.UserName == "" {
			t.Errorf("isi tidak sesuai: %+v", c)
		}
		if c.Status == "paid" {
			paid++
			if c.Overdue {
				t.Error("iuran yang sudah dibayar tidak boleh ditandai overdue")
			}
			if c.PaidOnTime == nil || !*c.PaidOnTime || c.PaidAt == nil {
				t.Errorf("iuran lunas harus punya paid_on_time & paid_at: %+v", c)
			}
		} else if c.Overdue {
			overdue++
		}
	}
	if paid != 1 || overdue != 2 {
		t.Errorf("paid=%d overdue=%d, want 1 dan 2", paid, overdue)
	}

	// putaran masa depan: belum overdue
	f.setDue(2, "2999-01-01")
	future, err := repo.ListByRound(ctx, f.g.ID, f.roundID(2))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range future {
		if c.Overdue {
			t.Error("iuran dengan jatuh tempo di masa depan tidak boleh overdue")
		}
	}

	// putaran milik grup lain -> tidak ditemukan (bukan bocor lintas grup)
	other := newFixture(t, testutil.GroupSpec{Slots: 2})
	if _, err := repo.ListByRound(ctx, f.g.ID, other.roundID(1)); !errors.Is(err, round.ErrNotFound) {
		t.Errorf("putaran grup lain: err = %v, want ErrNotFound", err)
	}
	if _, err := repo.ListByRound(ctx, f.g.ID, "00000000-0000-0000-0000-000000000000"); !errors.Is(err, round.ErrNotFound) {
		t.Errorf("putaran tidak ada: err = %v, want ErrNotFound", err)
	}
}

func TestListByUser(t *testing.T) {
	f := newFixture(t, testutil.GroupSpec{Slots: 3, Amount: 100_000})
	repo := NewRepository(f.e.Pool)
	ctx := context.Background()

	ownerID := f.e.QueryString(`SELECT user_id::text FROM group_members WHERE id = $1`, f.members[0].ID)
	f.setDue(1, "2026-12-01")
	f.setDue(2, "2027-01-01")
	f.setDue(3, "2027-02-01")
	if _, err := f.pay(f.contributionID(1, f.members[0]), time.Date(2026, 11, 30, 8, 0, 0, 0, wib)); err != nil {
		t.Fatal(err)
	}

	all, err := repo.ListByUser(ctx, ownerID, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Fatalf("len = %d, want 3 (satu per putaran)", len(all))
	}
	for i := 1; i < len(all); i++ {
		if all[i-1].DueDate > all[i].DueDate {
			t.Error("harus berurutan menurut jatuh tempo")
		}
	}
	for _, m := range all {
		if m.GroupID != f.g.ID || m.GroupName == "" {
			t.Errorf("info grup tidak terisi: %+v", m)
		}
	}

	unpaid, _ := repo.ListByUser(ctx, ownerID, "unpaid")
	paid, _ := repo.ListByUser(ctx, ownerID, "paid")
	if len(unpaid) != 2 || len(paid) != 1 {
		t.Errorf("unpaid=%d paid=%d, want 2 dan 1", len(unpaid), len(paid))
	}

	// pengguna lain tidak melihat iuran ini
	stranger := f.e.NewUser("stranger")
	if none, err := repo.ListByUser(ctx, stranger, ""); err != nil || none == nil || len(none) != 0 {
		t.Errorf("orang luar: %v, %v (harus slice kosong non-nil)", none, err)
	}

	// anggota yang bukan aktif tidak punya iuran yang dihitung
	f.e.Exec(`UPDATE group_members SET status = 'removed' WHERE id = $1`, f.members[1].ID)
	removedUser := f.e.QueryString(`SELECT user_id::text FROM group_members WHERE id = $1`, f.members[1].ID)
	if got, _ := repo.ListByUser(ctx, removedUser, ""); len(got) != 0 {
		t.Errorf("anggota removed melihat %d iuran, want 0", len(got))
	}
}

// ---- biaya platform, pembayar, dan kuotasi ----------------------------------------

func (f *fixture) payWith(id, payer string, at time.Time, bps int) (*Receipt, error) {
	var rc *Receipt
	err := f.e.InTx(func(tx pgx.Tx) error {
		var err error
		rc, err = MarkPaid(context.Background(), tx, id, payer, at, bps)
		return err
	})
	return rc, err
}

func TestMarkPaidChargesPlatformFeeAndLateFee(t *testing.T) {
	f := newFixture(t, testutil.GroupSpec{Slots: 3, Amount: 200_000, LateFee: 10_000})
	f.setDue(1, "2026-12-01")

	// tepat waktu: iuran + biaya 2,5% (5.000)
	id := f.contributionID(1, f.members[0])
	rc, err := f.payWith(id, f.payer(id), time.Date(2026, 12, 1, 9, 0, 0, 0, wib), 250)
	if err != nil {
		t.Fatal(err)
	}
	if rc.Amount != 200_000 || rc.PlatformFee != 5_000 || rc.LateFee != 0 || rc.Total != 205_000 || !rc.PaidOnTime {
		t.Errorf("tepat waktu: %+v", rc)
	}
	var stored int64
	if err := f.e.Pool.QueryRow(context.Background(), `SELECT platform_fee FROM contributions WHERE id = $1`, id).Scan(&stored); err != nil || stored != 5_000 {
		t.Errorf("platform_fee tersimpan = %d (%v), want 5000", stored, err)
	}

	// terlambat: iuran + biaya + denda
	id2 := f.contributionID(1, f.members[1])
	rc2, err := f.payWith(id2, f.payer(id2), time.Date(2026, 12, 5, 9, 0, 0, 0, wib), 250)
	if err != nil {
		t.Fatal(err)
	}
	if rc2.PlatformFee != 5_000 || rc2.LateFee != 10_000 || rc2.Total != 215_000 || rc2.PaidOnTime {
		t.Errorf("terlambat: %+v", rc2)
	}
}

func TestMarkPaidOnlyByOwnerOfTheContribution(t *testing.T) {
	f := newFixture(t, testutil.GroupSpec{Slots: 3})
	id := f.contributionID(1, f.members[0])
	other := f.payer(f.contributionID(1, f.members[1]))
	stranger := f.e.NewUser("stranger")
	now := time.Now()

	for name, payer := range map[string]string{"anggota lain di grup yang sama": other, "orang luar": stranger} {
		if _, err := f.payWith(id, payer, now, 0); !errors.Is(err, ErrNotFound) {
			t.Errorf("%s: err = %v, want ErrNotFound (keberadaan iuran tidak boleh bocor)", name, err)
		}
	}
	if st := f.e.QueryString(`SELECT status FROM contributions WHERE id = $1`, id); st != "unpaid" {
		t.Errorf("status = %s; pembayar yang salah tidak boleh melunasi", st)
	}

	// anggota yang sudah tidak aktif tidak bisa membayar
	f.e.Exec(`UPDATE group_members SET status = 'removed' WHERE id = $1`, f.members[0].ID)
	if _, err := f.payWith(id, f.payer(id), now, 0); !errors.Is(err, ErrNotFound) {
		t.Errorf("anggota removed: err = %v, want ErrNotFound", err)
	}
}

func TestQuoteFor(t *testing.T) {
	f := newFixture(t, testutil.GroupSpec{Slots: 3, Amount: 200_000, LateFee: 10_000})
	ctx := context.Background()
	f.setDue(1, "2026-12-01")
	id := f.contributionID(1, f.members[0])
	owner := f.payer(id)
	quote := func(userID string, at time.Time, bps int) (*Quote, error) {
		return QuoteFor(ctx, f.e.Pool, id, userID, at, bps)
	}

	t.Run("tepat waktu", func(t *testing.T) {
		q, err := quote(owner, time.Date(2026, 12, 1, 22, 0, 0, 0, wib), 250)
		if err != nil {
			t.Fatal(err)
		}
		if q.Amount != 200_000 || q.PlatformFee != 5_000 || q.LateFee != 0 || q.Total != 205_000 || q.Late || !q.Payable || q.Reason != "" {
			t.Errorf("quote = %+v", q)
		}
	})
	t.Run("terlambat", func(t *testing.T) {
		q, _ := quote(owner, time.Date(2026, 12, 2, 0, 0, 0, 0, wib), 250)
		if q.LateFee != 10_000 || q.Total != 215_000 || !q.Late || !q.Payable {
			t.Errorf("quote = %+v", q)
		}
	})
	t.Run("kuotasi tidak mengubah apa pun", func(t *testing.T) {
		if st := f.e.QueryString(`SELECT status FROM contributions WHERE id = $1`, id); st != "unpaid" {
			t.Errorf("status = %s", st)
		}
	})
	t.Run("bukan pemilik", func(t *testing.T) {
		if _, err := quote(f.e.NewUser("x"), time.Now(), 0); !errors.Is(err, ErrNotFound) {
			t.Errorf("err = %v, want ErrNotFound", err)
		}
		if _, err := QuoteFor(ctx, f.e.Pool, "00000000-0000-0000-0000-000000000000", owner, time.Now(), 0); !errors.Is(err, ErrNotFound) {
			t.Errorf("id tidak ada: err = %v", err)
		}
	})
	t.Run("putaran belum dibuka", func(t *testing.T) {
		up := f.contributionID(2, f.members[0])
		q, err := QuoteFor(ctx, f.e.Pool, up, owner, time.Now(), 0)
		if err != nil || q.Payable || q.Reason != "putaran belum dibuka" || q.RoundStatus != "upcoming" {
			t.Errorf("quote = %+v, %v", q, err)
		}
	})
	t.Run("putaran sudah ditutup", func(t *testing.T) {
		f.e.Exec(`UPDATE rounds SET status = 'paid_out' WHERE group_id = $1 AND round_no = 3`, f.g.ID)
		pd := f.contributionID(3, f.members[0])
		q, _ := QuoteFor(ctx, f.e.Pool, pd, owner, time.Now(), 0)
		if q.Payable || q.Reason != "putaran sudah ditutup" {
			t.Errorf("quote = %+v", q)
		}
	})
	t.Run("sudah dibayar menampilkan nominal yang tercatat", func(t *testing.T) {
		if _, err := f.payWith(id, owner, time.Date(2026, 12, 3, 9, 0, 0, 0, wib), 250); err != nil {
			t.Fatal(err)
		}
		// bps berbeda saat melihat tidak boleh mengubah angka yang sudah tercatat
		q, err := quote(owner, time.Now(), 999)
		if err != nil {
			t.Fatal(err)
		}
		if q.Payable || q.Reason != "sudah dibayar" || q.PlatformFee != 5_000 || q.LateFee != 10_000 || q.Total != 215_000 || !q.Late {
			t.Errorf("quote = %+v", q)
		}
	})
}
