/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package round

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/alexistdev/geopatungan/internal/testutil"
)

// Tes ini butuh database Postgres dengan semua migrasi; dilewati jika TEST_DATABASE_URL kosong.

func schedule(e *testutil.Env, groupID string) error {
	return e.InTx(func(tx pgx.Tx) error { return CreateSchedule(context.Background(), tx, groupID) })
}

func dueDates(t *testing.T, e *testutil.Env, groupID string) []string {
	t.Helper()
	rows, err := e.Pool.Query(context.Background(),
		`SELECT to_char(due_date, 'YYYY-MM-DD') FROM rounds WHERE group_id = $1 ORDER BY round_no`, groupID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		out = append(out, s)
	}
	return out
}

func TestCreateScheduleWeekly(t *testing.T) {
	e := testutil.NewEnv(t)
	g, members := e.NewGroup(testutil.GroupSpec{Slots: 4, Period: "weekly", StartDate: "2026-12-01"})
	e.SetTurnOrders(members)

	if err := schedule(e, g.ID); err != nil {
		t.Fatalf("CreateSchedule: %v", err)
	}

	want := []string{"2026-12-01", "2026-12-08", "2026-12-15", "2026-12-22"}
	if got := dueDates(t, e, g.ID); !reflect.DeepEqual(got, want) {
		t.Errorf("jatuh tempo = %v, want %v", got, want)
	}

	// penerima putaran k = anggota ber-turn_order k; putaran 1 sedang menerima iuran
	for i, m := range members {
		no := i + 1
		recipient := e.QueryString(`SELECT recipient_member_id::text FROM rounds WHERE group_id = $1 AND round_no = $2`, g.ID, no)
		if recipient != m.ID {
			t.Errorf("penerima putaran %d = %s, want %s", no, recipient, m.ID)
		}
		status := e.QueryString(`SELECT status FROM rounds WHERE group_id = $1 AND round_no = $2`, g.ID, no)
		wantStatus := "upcoming"
		if no == 1 {
			wantStatus = "collecting"
		}
		if status != wantStatus {
			t.Errorf("status putaran %d = %s, want %s", no, status, wantStatus)
		}
	}
}

func TestCreateScheduleMonthlyClampsMonthEnd(t *testing.T) {
	cases := []struct {
		name  string
		start string
		slots int
		want  []string
	}{
		{"tahun biasa", "2026-01-31", 5, []string{"2026-01-31", "2026-02-28", "2026-03-31", "2026-04-30", "2026-05-31"}},
		{"tahun kabisat", "2028-01-31", 3, []string{"2028-01-31", "2028-02-29", "2028-03-31"}},
		{"tanggal 30", "2026-01-30", 3, []string{"2026-01-30", "2026-02-28", "2026-03-30"}},
		{"lintas tahun", "2026-11-15", 4, []string{"2026-11-15", "2026-12-15", "2027-01-15", "2027-02-15"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := testutil.NewEnv(t)
			g, members := e.NewGroup(testutil.GroupSpec{Slots: tc.slots, Period: "monthly", StartDate: tc.start})
			e.SetTurnOrders(members)
			if err := schedule(e, g.ID); err != nil {
				t.Fatal(err)
			}
			if got := dueDates(t, e, g.ID); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("jatuh tempo = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCreateScheduleCreatesEveryContribution(t *testing.T) {
	e := testutil.NewEnv(t)
	g, members := e.NewGroup(testutil.GroupSpec{Slots: 5, Amount: 250_000})
	e.SetTurnOrders(members)
	if err := schedule(e, g.ID); err != nil {
		t.Fatal(err)
	}

	if n := e.QueryInt(`SELECT count(*) FROM rounds WHERE group_id = $1`, g.ID); n != 5 {
		t.Errorf("putaran = %d, want 5", n)
	}
	if n := e.QueryInt(`SELECT count(*) FROM contributions c JOIN rounds r ON r.id = c.round_id WHERE r.group_id = $1`, g.ID); n != 25 {
		t.Errorf("iuran = %d, want 25 (5 anggota x 5 putaran)", n)
	}
	if n := e.QueryInt(`SELECT count(*) FROM contributions c JOIN rounds r ON r.id = c.round_id
	                     WHERE r.group_id = $1 AND c.amount = 250000 AND c.status = 'unpaid'
	                       AND c.paid_at IS NULL AND c.late_fee = 0`, g.ID); n != 25 {
		t.Errorf("iuran bernominal benar & belum dibayar = %d, want 25", n)
	}
	// tiap anggota punya tepat satu iuran per putaran
	if n := e.QueryInt(`SELECT count(DISTINCT (c.round_id, c.member_id)) FROM contributions c
	                     JOIN rounds r ON r.id = c.round_id WHERE r.group_id = $1`, g.ID); n != 25 {
		t.Errorf("pasangan (putaran, anggota) unik = %d, want 25", n)
	}
}

func TestCreateScheduleIgnoresPendingMembers(t *testing.T) {
	e := testutil.NewEnv(t)
	ctx := context.Background()
	g, members := e.NewGroup(testutil.GroupSpec{Slots: 4, Active: 3})
	if _, err := e.Groups.AddMember(ctx, g.ID, e.NewUser("pending")); err != nil {
		t.Fatal(err)
	}
	e.SetTurnOrders(members)

	if err := schedule(e, g.ID); err != nil {
		t.Fatal(err)
	}
	if n := e.QueryInt(`SELECT count(*) FROM rounds WHERE group_id = $1`, g.ID); n != 3 {
		t.Errorf("putaran = %d, want 3 (anggota pending tidak dihitung)", n)
	}
	if n := e.QueryInt(`SELECT count(*) FROM contributions c JOIN rounds r ON r.id = c.round_id WHERE r.group_id = $1`, g.ID); n != 9 {
		t.Errorf("iuran = %d, want 9", n)
	}
}

func TestCreateScheduleTwiceFailsAndKeepsFirst(t *testing.T) {
	e := testutil.NewEnv(t)
	g, members := e.NewGroup(testutil.GroupSpec{Slots: 3})
	e.SetTurnOrders(members)

	if err := schedule(e, g.ID); err != nil {
		t.Fatal(err)
	}
	if err := schedule(e, g.ID); err == nil {
		t.Fatal("jadwal kedua seharusnya gagal")
	}
	if n := e.QueryInt(`SELECT count(*) FROM rounds WHERE group_id = $1`, g.ID); n != 3 {
		t.Errorf("putaran = %d, want 3 (jadwal pertama tidak boleh rusak)", n)
	}
	if n := e.QueryInt(`SELECT count(*) FROM contributions c JOIN rounds r ON r.id = c.round_id WHERE r.group_id = $1`, g.ID); n != 9 {
		t.Errorf("iuran = %d, want 9", n)
	}
}

func TestCreateScheduleRequiresTurnOrders(t *testing.T) {
	e := testutil.NewEnv(t)
	g, members := e.NewGroup(testutil.GroupSpec{Slots: 3})
	e.Exec(`UPDATE group_members SET turn_order = 1 WHERE id = $1`, members[0].ID) // sisanya kosong

	err := schedule(e, g.ID)
	if !errors.Is(err, ErrTurnOrderMissing) {
		t.Fatalf("err = %v, want ErrTurnOrderMissing", err)
	}
	if n := e.QueryInt(`SELECT count(*) FROM rounds WHERE group_id = $1`, g.ID); n != 0 {
		t.Errorf("putaran = %d, want 0", n)
	}
}

func TestCreateScheduleRollsBackOnLaterFailure(t *testing.T) {
	e := testutil.NewEnv(t)
	g, members := e.NewGroup(testutil.GroupSpec{Slots: 3})
	e.SetTurnOrders(members)

	// transaksi pemanggil gagal setelah jadwal dibuat -> seluruh jadwal harus lenyap
	boom := errors.New("gagal setelah jadwal")
	err := e.InTx(func(tx pgx.Tx) error {
		if err := CreateSchedule(context.Background(), tx, g.ID); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	if n := e.QueryInt(`SELECT count(*) FROM rounds WHERE group_id = $1`, g.ID); n != 0 {
		t.Errorf("putaran tersisa = %d, want 0", n)
	}
}

func TestListByGroup(t *testing.T) {
	e := testutil.NewEnv(t)
	g, members := e.NewGroup(testutil.GroupSpec{Slots: 3, Amount: 200_000})
	e.SetTurnOrders(members)
	if err := schedule(e, g.ID); err != nil {
		t.Fatal(err)
	}
	// satu anggota sudah membayar putaran 1
	e.Exec(`UPDATE contributions SET status = 'paid', paid_at = now(), paid_on_time = true
	         WHERE member_id = $1 AND round_id = (SELECT id FROM rounds WHERE group_id = $2 AND round_no = 1)`,
		members[1].ID, g.ID)

	repo := NewRepository(e.Pool)
	rounds, err := repo.ListByGroup(context.Background(), g.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rounds) != 3 {
		t.Fatalf("len = %d, want 3", len(rounds))
	}
	for i, r := range rounds {
		if r.RoundNo != i+1 {
			t.Errorf("urutan salah: posisi %d punya round_no %d", i, r.RoundNo)
		}
		if r.MemberCount != 3 || r.PotAmount != 600_000 {
			t.Errorf("putaran %d: member_count=%d pot=%d, want 3 dan 600000", r.RoundNo, r.MemberCount, r.PotAmount)
		}
		if r.RecipientName == "" || r.RecipientMemberID != members[i].ID {
			t.Errorf("putaran %d: penerima %q (%s)", r.RoundNo, r.RecipientName, r.RecipientMemberID)
		}
	}
	if rounds[0].PaidCount != 1 || rounds[1].PaidCount != 0 {
		t.Errorf("paid_count = %d/%d, want 1/0", rounds[0].PaidCount, rounds[1].PaidCount)
	}
	if rounds[0].Status != "collecting" || rounds[2].Status != "upcoming" {
		t.Errorf("status = %s/%s", rounds[0].Status, rounds[2].Status)
	}

	// grup tanpa jadwal -> daftar kosong, bukan error/null
	other, _ := e.NewGroup(testutil.GroupSpec{Slots: 2})
	empty, err := repo.ListByGroup(context.Background(), other.ID)
	if err != nil || empty == nil || len(empty) != 0 {
		t.Errorf("grup tanpa jadwal: %v, %v (harus slice kosong non-nil)", empty, err)
	}
}

func payAll(e *testutil.Env, groupID string, roundNo int) {
	e.Exec(`UPDATE contributions SET status = 'paid', paid_at = now(), paid_on_time = true
	         WHERE round_id = (SELECT id FROM rounds WHERE group_id = $1 AND round_no = $2)`, groupID, roundNo)
}

func roundID(e *testutil.Env, groupID string, no int) string {
	return e.QueryString(`SELECT id::text FROM rounds WHERE group_id = $1 AND round_no = $2`, groupID, no)
}

func complete(e *testutil.Env, id string) (done bool, err error) {
	err = e.InTx(func(tx pgx.Tx) error {
		var cerr error
		done, cerr = Complete(context.Background(), tx, id)
		return cerr
	})
	return
}

func TestCompleteAdvancesRoundsAndFinishesGroup(t *testing.T) {
	e := testutil.NewEnv(t)
	g, members := e.NewGroup(testutil.GroupSpec{Slots: 3})
	e.SetTurnOrders(members)
	e.Exec(`UPDATE groups SET status = 'running' WHERE id = $1`, g.ID)
	if err := schedule(e, g.ID); err != nil {
		t.Fatal(err)
	}
	status := func(no int) string {
		return e.QueryString(`SELECT status FROM rounds WHERE group_id = $1 AND round_no = $2`, g.ID, no)
	}

	// belum lunas -> ditolak dan tidak ada yang berubah
	if _, err := complete(e, roundID(e, g.ID, 1)); !errors.Is(err, ErrContributionsPending) {
		t.Fatalf("err = %v, want ErrContributionsPending", err)
	}
	if status(1) != "collecting" || status(2) != "upcoming" {
		t.Errorf("status berubah padahal ditolak: %s/%s", status(1), status(2))
	}

	// putaran upcoming tidak bisa ditutup lebih dulu
	payAll(e, g.ID, 2)
	if _, err := complete(e, roundID(e, g.ID, 2)); !errors.Is(err, ErrRoundNotCollecting) {
		t.Errorf("putaran upcoming: err = %v, want ErrRoundNotCollecting", err)
	}

	// putaran 1 lunas -> selesai, putaran 2 mulai
	payAll(e, g.ID, 1)
	done, err := complete(e, roundID(e, g.ID, 1))
	if err != nil || done {
		t.Fatalf("putaran 1: done=%v err=%v", done, err)
	}
	if status(1) != "paid_out" || status(2) != "collecting" || status(3) != "upcoming" {
		t.Errorf("status = %s/%s/%s", status(1), status(2), status(3))
	}

	// tidak bisa ditutup dua kali
	if _, err := complete(e, roundID(e, g.ID, 1)); !errors.Is(err, ErrRoundNotCollecting) {
		t.Errorf("tutup ulang: err = %v, want ErrRoundNotCollecting", err)
	}

	// putaran 2 lalu 3 (terakhir): grup selesai
	if done, err := complete(e, roundID(e, g.ID, 2)); err != nil || done {
		t.Fatalf("putaran 2: done=%v err=%v", done, err)
	}
	payAll(e, g.ID, 3)
	if e.QueryString(`SELECT status FROM groups WHERE id = $1`, g.ID) != "running" {
		t.Error("grup harus masih running sebelum putaran terakhir ditutup")
	}
	done, err = complete(e, roundID(e, g.ID, 3))
	if err != nil || !done {
		t.Fatalf("putaran terakhir: done=%v err=%v", done, err)
	}
	if got := e.QueryString(`SELECT status FROM groups WHERE id = $1`, g.ID); got != "done" {
		t.Errorf("status grup = %s, want done", got)
	}
}

func TestCompleteUnknownRound(t *testing.T) {
	e := testutil.NewEnv(t)
	_, err := complete(e, "00000000-0000-0000-0000-000000000000")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

// Dua penutupan putaran bersamaan: tepat satu berhasil.
func TestCompleteConcurrent(t *testing.T) {
	e := testutil.NewEnv(t)
	g, members := e.NewGroup(testutil.GroupSpec{Slots: 3})
	e.SetTurnOrders(members)
	if err := schedule(e, g.ID); err != nil {
		t.Fatal(err)
	}
	payAll(e, g.ID, 1)
	id := roundID(e, g.ID, 1)

	const n = 6
	results := make(chan error, n)
	for i := 0; i < n; i++ {
		go func() { _, err := complete(e, id); results <- err }()
	}
	ok := 0
	for i := 0; i < n; i++ {
		switch err := <-results; {
		case err == nil:
			ok++
		case errors.Is(err, ErrRoundNotCollecting):
		default:
			t.Errorf("error tak terduga: %v", err)
		}
	}
	if ok != 1 {
		t.Errorf("%d penutupan berhasil, want tepat 1", ok)
	}
}

// Tes deterministik untuk kunci baris putaran. Transaksi A menutup putaran tetapi belum
// commit; Complete kedua yang dimulai saat itu harus menunggu lalu membaca ulang status
// dan gagal (ErrRoundNotCollecting). Tanpa FOR UPDATE ia membaca status lama
// ("collecting"), lalu menimpa hasil A: putaran berikutnya tidak dimajukan lagi dan
// grup salah dianggap selesai. (Tes balapan di atas bergantung waktu dan bisa lolos.)
func TestCompleteLocksRoundRow(t *testing.T) {
	e := testutil.NewEnv(t)
	g, members := e.NewGroup(testutil.GroupSpec{Slots: 3})
	e.SetTurnOrders(members)
	e.Exec(`UPDATE groups SET status = 'running' WHERE id = $1`, g.ID)
	if err := schedule(e, g.ID); err != nil {
		t.Fatal(err)
	}
	payAll(e, g.ID, 1)
	id := roundID(e, g.ID, 1)

	ctx := context.Background()
	txA, err := e.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = txA.Rollback(ctx) }()
	if _, err := Complete(ctx, txA, id); err != nil { // A menutup putaran 1, belum commit
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() { _, err := complete(e, id); done <- err }()

	select {
	case err := <-done:
		t.Fatalf("Complete kedua selesai (err=%v) padahal A belum commit; seharusnya menunggu", err)
	case <-time.After(300 * time.Millisecond):
	}
	if err := txA.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	select {
	case err := <-done:
		if !errors.Is(err, ErrRoundNotCollecting) {
			t.Errorf("err = %v, want ErrRoundNotCollecting (putaran sudah ditutup A)", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Complete kedua tidak selesai setelah A commit")
	}

	// hasil A harus utuh: putaran 2 collecting, grup masih running
	if got := e.QueryString(`SELECT status FROM rounds WHERE group_id = $1 AND round_no = 2`, g.ID); got != "collecting" {
		t.Errorf("status putaran 2 = %s, want collecting", got)
	}
	if got := e.QueryString(`SELECT status FROM groups WHERE id = $1`, g.ID); got != "running" {
		t.Errorf("status grup = %s, want running", got)
	}
}
