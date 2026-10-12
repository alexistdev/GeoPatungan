/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package reputation

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/alexistdev/geopatungan/internal/draw"
	"github.com/alexistdev/geopatungan/internal/group"
	"github.com/alexistdev/geopatungan/internal/round"
	"github.com/alexistdev/geopatungan/internal/testutil"
)

// Tes ini butuh database Postgres dengan semua migrasi; dilewati jika TEST_DATABASE_URL kosong.
//
// reputation_levels adalah tabel GLOBAL milik aplikasi: tes tidak mengubahnya. Definisi level
// yang berlaku (bisa saja sudah diubah admin) dibaca lebih dulu, dan asumsi tes tentang ambang
// bawaan diperiksa agar tes berhenti jelas jika definisinya berbeda.

type fx struct {
	e    *testutil.Env
	repo *Repository
	svc  *Service
	ctx  context.Context
}

func newFx(t *testing.T) *fx {
	t.Helper()
	e := testutil.NewEnv(t)
	repo := NewRepository(e.Pool)
	f := &fx{e: e, repo: repo, svc: NewService(repo), ctx: context.Background()}

	got, err := repo.Levels(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := defaultLevels()
	if len(got) != 3 {
		t.Fatalf("level di database = %d, want 3", len(got))
	}
	for i := range want {
		if got[i].RequireVerified != want[i].RequireVerified || got[i].MinCompletedCycles != want[i].MinCompletedCycles ||
			(got[i].MaxLateCount == nil) != (want[i].MaxLateCount == nil) || got[i].DrawWeight != want[i].DrawWeight {
			t.Skipf("definisi level di database berbeda dari seed bawaan (diubah admin?): level %d = %+v", i+1, got[i])
		}
	}
	return f
}

func (f *fx) setKYC(userID, status string) {
	f.e.T.Helper()
	f.e.Exec(`UPDATE users SET kyc_status = $2 WHERE id = $1`, userID, status)
}

// doneGroups membuat n grup berstatus done yang diikuti userID sebagai anggota aktif.
func (f *fx) doneGroups(userID string, n int) {
	f.e.T.Helper()
	for i := 0; i < n; i++ {
		g, _ := f.e.NewGroupFor(testutil.GroupSpec{Slots: 2}, userID, f.e.NewUser("lawan"))
		f.e.Exec(`UPDATE groups SET status = 'done' WHERE id = $1`, g.ID)
	}
}

// runningGroup membuat grup 2 anggota yang sudah berjalan (jadwal dibuat); userID adalah anggota pertama.
func (f *fx) runningGroup(userID string) (*group.Group, []group.Member) {
	f.e.T.Helper()
	g, members := f.e.NewGroupFor(testutil.GroupSpec{Slots: 2}, userID, f.e.NewUser("lawan"))
	f.e.SetTurnOrders(members)
	f.e.Exec(`UPDATE groups SET status = 'running' WHERE id = $1`, g.ID)
	err := f.e.InTx(func(tx pgx.Tx) error { return round.CreateSchedule(context.Background(), tx, g.ID) })
	if err != nil {
		f.e.T.Fatal(err)
	}
	return g, members
}

func (f *fx) contribution(g *group.Group, roundNo int, memberID string) string {
	f.e.T.Helper()
	return f.e.QueryString(`SELECT c.id::text FROM contributions c JOIN rounds r ON r.id = c.round_id
	                         WHERE r.group_id = $1 AND r.round_no = $2 AND c.member_id = $3`, g.ID, roundNo, memberID)
}

func (f *fx) markPaid(contributionID string, onTime bool) {
	f.e.T.Helper()
	f.e.Exec(`UPDATE contributions SET status = 'paid', paid_at = now(), paid_on_time = $2 WHERE id = $1`, contributionID, onTime)
}

func (f *fx) stats(userID string) Stats {
	f.e.T.Helper()
	st, err := f.repo.Stats(f.ctx, userID)
	if err != nil {
		f.e.T.Fatalf("Stats: %v", err)
	}
	return st
}

func (f *fx) rank(userID string) int {
	f.e.T.Helper()
	p, err := f.svc.Profile(f.ctx, userID)
	if err != nil {
		f.e.T.Fatal(err)
	}
	return p.Level.Rank
}

// ---- Levels ----------------------------------------------------------------

func TestLevelsAreOrderedAndNullable(t *testing.T) {
	f := newFx(t)
	levels, err := f.repo.Levels(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	for i, l := range levels {
		if l.Rank != i+1 || l.Code == "" || l.Name == "" {
			t.Errorf("level %d = %+v", i, l)
		}
	}
	if levels[0].MaxLateCount != nil || levels[0].MaxGroupValue == nil || *levels[0].MaxGroupValue != 500_000 {
		t.Errorf("level 1: %+v", levels[0])
	}
	if levels[2].MaxGroupValue != nil {
		t.Errorf("level 3 tanpa batas nilai harus nil, got %v", *levels[2].MaxGroupValue)
	}
	if err := ValidateLevels(levels); err != nil {
		t.Errorf("definisi di database harus lolos validasi: %v", err)
	}
}

// ---- Stats -----------------------------------------------------------------

func TestStatsForNewUser(t *testing.T) {
	f := newFx(t)
	st := f.stats(f.e.NewUser("baru"))
	if st.KYCStatus != "none" || st.Verified || st.CompletedCycles != 0 || st.OnTimePayments != 0 ||
		st.LatePayments != 0 || st.OverdueUnpaid != 0 || st.LateTotal != 0 {
		t.Errorf("stats = %+v", st)
	}
	if _, err := f.repo.Stats(f.ctx, "00000000-0000-0000-0000-000000000000"); !errors.Is(err, ErrNotFound) {
		t.Errorf("pengguna tidak ada: err = %v, want ErrNotFound", err)
	}
}

func TestStatsOnlyVerifiedKYCCounts(t *testing.T) {
	f := newFx(t)
	u := f.e.NewUser("a")
	for status, want := range map[string]bool{"none": false, "pending": false, "rejected": false, "verified": true} {
		f.setKYC(u, status)
		st := f.stats(u)
		if st.Verified != want || st.KYCStatus != status {
			t.Errorf("kyc %s: verified = %v (status %q), want %v", status, st.Verified, st.KYCStatus, want)
		}
	}
}

func TestStatsCountsOnlyFinishedGroupsWhereMemberIsActive(t *testing.T) {
	f := newFx(t)
	u := f.e.NewUser("a")

	f.doneGroups(u, 2)
	// grup yang belum tuntas tidak dihitung
	for _, status := range []string{"open", "running", "cancelled"} {
		g, _ := f.e.NewGroupFor(testutil.GroupSpec{Slots: 2}, u, f.e.NewUser("x"))
		f.e.Exec(`UPDATE groups SET status = $2 WHERE id = $1`, g.ID, status)
	}
	// anggota yang bukan aktif pada grup done tidak dihitung
	g, _ := f.e.NewGroupFor(testutil.GroupSpec{Slots: 2}, f.e.NewUser("owner"), u)
	f.e.Exec(`UPDATE groups SET status = 'done' WHERE id = $1`, g.ID)
	f.e.Exec(`UPDATE group_members SET status = 'removed' WHERE group_id = $1 AND user_id = $2`, g.ID, u)

	if got := f.stats(u).CompletedCycles; got != 2 {
		t.Errorf("CompletedCycles = %d, want 2", got)
	}
}

func TestStatsCountsOnTimeAndLatePayments(t *testing.T) {
	f := newFx(t)
	u := f.e.NewUser("a")
	g, members := f.runningGroup(u)

	f.markPaid(f.contribution(g, 1, members[0].ID), true)
	f.markPaid(f.contribution(g, 2, members[0].ID), false)
	// iuran milik lawan tidak ikut dihitung
	f.markPaid(f.contribution(g, 1, members[1].ID), false)

	st := f.stats(u)
	if st.OnTimePayments != 1 || st.LatePayments != 1 || st.OverdueUnpaid != 0 || st.LateTotal != 1 {
		t.Errorf("stats = %+v, want ontime 1, late 1, overdue 0, total 1", st)
	}
}

func TestStatsOverdueOnlyForCollectingRoundsPastDue(t *testing.T) {
	f := newFx(t)
	u := f.e.NewUser("a")
	g, members := f.runningGroup(u) // putaran 1 collecting, putaran 2 upcoming

	today := f.e.QueryString(`SELECT (now() AT TIME ZONE 'Asia/Jakarta')::date::text`)

	// jatuh tempo hari ini (WIB): belum menunggak
	f.e.Exec(`UPDATE rounds SET due_date = $2::date WHERE group_id = $1 AND round_no = 1`, g.ID, today)
	if got := f.stats(u).OverdueUnpaid; got != 0 {
		t.Errorf("jatuh tempo hari ini: OverdueUnpaid = %d, want 0", got)
	}
	// kemarin: menunggak
	f.e.Exec(`UPDATE rounds SET due_date = ($2::date - 1) WHERE group_id = $1 AND round_no = 1`, g.ID, today)
	if got := f.stats(u).OverdueUnpaid; got != 1 {
		t.Errorf("jatuh tempo kemarin: OverdueUnpaid = %d, want 1", got)
	}
	// putaran upcoming yang tanggalnya lewat tidak dihitung (belum bisa dibayar)
	f.e.Exec(`UPDATE rounds SET due_date = ($2::date - 30) WHERE group_id = $1 AND round_no = 2`, g.ID, today)
	if got := f.stats(u).OverdueUnpaid; got != 1 {
		t.Errorf("putaran upcoming ikut dihitung: OverdueUnpaid = %d, want 1", got)
	}
	// setelah dibayar (terlambat), tidak lagi menunggak tetapi tercatat terlambat
	f.markPaid(f.contribution(g, 1, members[0].ID), false)
	st := f.stats(u)
	if st.OverdueUnpaid != 0 || st.LatePayments != 1 || st.LateTotal != 1 {
		t.Errorf("setelah dibayar terlambat: %+v", st)
	}
}

// ---- level dari data nyata -------------------------------------------------

func TestLevelProgressionFromRealData(t *testing.T) {
	f := newFx(t)
	u := f.e.NewUser("a")

	if r := f.rank(u); r != 1 {
		t.Fatalf("pengguna baru: level %d, want 1", r)
	}

	f.doneGroups(u, 1)
	if r := f.rank(u); r != 1 {
		t.Errorf("1 arisan tuntas tetapi belum terverifikasi: level %d, want 1", r)
	}

	f.setKYC(u, "pending")
	if r := f.rank(u); r != 1 {
		t.Errorf("KYC masih pending: level %d, want 1", r)
	}
	f.setKYC(u, "verified")
	if r := f.rank(u); r != 2 {
		t.Errorf("terverifikasi + 1 arisan tuntas: level %d, want 2", r)
	}

	f.doneGroups(u, 2)
	if r := f.rank(u); r != 3 {
		t.Errorf("terverifikasi + 3 arisan tuntas: level %d, want 3", r)
	}

	// satu keterlambatan menurunkan SEMUA level di atas dasar (syarat: tanpa keterlambatan)
	g, members := f.runningGroup(u)
	f.markPaid(f.contribution(g, 1, members[0].ID), false)
	if r := f.rank(u); r != 1 {
		t.Errorf("pernah terlambat: level %d, want 1", r)
	}
}

func TestOverdueUnpaidDropsLevelImmediately(t *testing.T) {
	f := newFx(t)
	u := f.e.NewUser("a")
	f.setKYC(u, "verified")
	f.doneGroups(u, 3)
	if r := f.rank(u); r != 3 {
		t.Fatalf("level awal %d, want 3", r)
	}

	g, _ := f.runningGroup(u)
	today := f.e.QueryString(`SELECT (now() AT TIME ZONE 'Asia/Jakarta')::date::text`)
	f.e.Exec(`UPDATE rounds SET due_date = ($2::date - 1) WHERE group_id = $1 AND round_no = 1`, g.ID, today)

	if r := f.rank(u); r != 1 {
		t.Errorf("menunggak: level %d, want 1 (turun seketika, tanpa menunggu dibayar terlambat)", r)
	}
}

func TestPolicyMatchesLevel(t *testing.T) {
	f := newFx(t)
	u := f.e.NewUser("a")

	pol, err := f.svc.Policy(f.ctx, u)
	if err != nil {
		t.Fatal(err)
	}
	if pol.Rank != 1 || pol.DrawWeight != 1 || !pol.CanCreateGroup || pol.MaxGroupValue != 500_000 {
		t.Errorf("level 1: %+v", pol)
	}

	f.setKYC(u, "verified")
	f.doneGroups(u, 3)
	pol, _ = f.svc.Policy(f.ctx, u)
	if pol.Rank != 3 || pol.DrawWeight != 3 || pol.MaxGroupValue != 0 {
		t.Errorf("level 3: %+v (MaxGroupValue 0 = tanpa batas)", pol)
	}
}

// ---- pengaturan level oleh admin -------------------------------------------

// Dijalankan di transaksi yang SELALU di-rollback: definisi level adalah data global aplikasi.
func TestSaveLevelsWritesAuditAndRespectsDatabaseChecks(t *testing.T) {
	f := newFx(t)
	admin := f.e.NewAdmin("admin")
	ctx := f.ctx

	inTx := func(fn func(tx pgx.Tx)) {
		tx, err := f.e.Pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback(ctx) }()
		fn(tx)
	}

	inTx(func(tx pgx.Tx) {
		levels := defaultLevels()
		levels[1].MaxLateCount = ip(3)
		levels[1].MinCompletedCycles = 2
		levels[2].MaxGroupValue = i64p(20_000_000)
		if err := saveLevels(ctx, tx, admin, levels); err != nil {
			t.Fatalf("saveLevels: %v", err)
		}
		var late, cycles int
		var max *int64
		if err := tx.QueryRow(ctx, `SELECT max_late_count, min_completed_cycles FROM reputation_levels WHERE rank = 2`).Scan(&late, &cycles); err != nil {
			t.Fatal(err)
		}
		if late != 3 || cycles != 2 {
			t.Errorf("level 2 = late %d cycles %d, want 3 dan 2", late, cycles)
		}
		if err := tx.QueryRow(ctx, `SELECT max_group_value FROM reputation_levels WHERE rank = 3`).Scan(&max); err != nil || max == nil || *max != 20_000_000 {
			t.Errorf("max_group_value level 3 = %v, %v", max, err)
		}
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE actor_id = $1 AND action = 'reputation.levels.update'`, admin).Scan(&n); err != nil || n != 1 {
			t.Errorf("audit = %d, want 1 (%v)", n, err)
		}
	})

	// batasan di database (lapisan terakhir, di luar validasi service)
	exec := func(sql string) error {
		var err error
		inTx(func(tx pgx.Tx) { _, err = tx.Exec(ctx, sql) })
		return err
	}
	if err := exec(`UPDATE reputation_levels SET require_verified = true WHERE rank = 1`); err == nil {
		t.Error("level 1 dengan syarat harus ditolak CHECK")
	}
	if err := exec(`UPDATE reputation_levels SET min_completed_cycles = 1 WHERE rank = 1`); err == nil {
		t.Error("level 1 dengan syarat arisan harus ditolak CHECK")
	}
	if err := exec(`UPDATE reputation_levels SET draw_weight = 3.5 WHERE rank = 3`); err == nil {
		t.Error("bobot di atas 3 (melanggar rasio 3:1) harus ditolak CHECK")
	}
	if err := exec(`UPDATE reputation_levels SET draw_weight = 0.9 WHERE rank = 2`); err == nil {
		t.Error("bobot di bawah 1 harus ditolak CHECK")
	}
	if err := exec(`UPDATE reputation_levels SET max_group_value = 0 WHERE rank = 1`); err == nil {
		t.Error("batas nilai 0 harus ditolak CHECK")
	}
	if err := exec(`UPDATE reputation_levels SET code = 'Baru' WHERE rank = 1`); err == nil {
		t.Error("kode huruf besar harus ditolak CHECK")
	}
	if err := exec(`UPDATE reputation_levels SET code = 'baru' WHERE rank = 2`); err == nil {
		t.Error("kode kembar harus ditolak UNIQUE")
	}
	if err := exec(`INSERT INTO reputation_levels (rank, code, name, draw_weight) VALUES (4, 'x', 'X', 1)`); err == nil {
		t.Error("rank 4 harus ditolak CHECK")
	}

	// setelah semua rollback, definisi tidak berubah
	levels, _ := f.repo.Levels(ctx)
	if levels[1].MinCompletedCycles != 1 || *levels[1].MaxLateCount != 0 {
		t.Errorf("rollback bocor: level 2 = %+v", levels[1])
	}
}

func TestGroupMinLevelRankMustReferenceExistingLevel(t *testing.T) {
	f := newFx(t)
	owner := f.e.NewUser("o")
	g, _ := f.e.NewGroupFor(testutil.GroupSpec{Slots: 2}, owner, f.e.NewUser("m"))
	// CHECK 1..3 sudah menolak 4; FK memastikan rank 2 tidak bisa dihapus selagi dipakai grup
	f.e.Exec(`UPDATE groups SET min_level_rank = 2 WHERE id = $1`, g.ID)
	tx, err := f.e.Pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(f.ctx) }()
	if _, err := tx.Exec(f.ctx, `DELETE FROM reputation_levels WHERE rank = 2`); err == nil {
		t.Error("level yang dipakai grup tidak boleh bisa dihapus (FK)")
	}
}

// ---- efek pada modul grup dan kocokan --------------------------------------

func TestGroupCreationAndJoiningFollowReputation(t *testing.T) {
	f := newFx(t)
	ctx := f.ctx
	gsvc := group.NewService(group.NewRepository(f.e.Pool), f.svc)
	input := func(amount int64, minLevel int) group.CreateInput {
		return group.CreateInput{Name: "Arisan Uji", ContributionAmount: amount, PeriodType: "monthly",
			SlotCount: 5, StartDate: time.Now().AddDate(0, 0, 3).Format("2006-01-02"), MinLevelRank: minLevel}
	}

	newbie := f.e.NewUser("newbie")
	if _, err := gsvc.Create(ctx, newbie, input(500_001, 0)); !errors.Is(err, group.ErrGroupTooExpensive) {
		t.Errorf("pengguna baru membuat grup Rp500.001: err = %v, want ErrGroupTooExpensive", err)
	}
	if _, err := gsvc.Create(ctx, newbie, input(500_000, 0)); err != nil {
		t.Errorf("tepat di batas level 1: %v", err)
	}

	// pengguna level 2 boleh membuat grup lebih besar dan mensyaratkan level 2
	l2 := f.e.NewUser("l2")
	f.setKYC(l2, "verified")
	f.doneGroups(l2, 1)
	big, err := gsvc.Create(ctx, l2, input(2_000_000, 2))
	if err != nil {
		t.Fatalf("level 2 membuat grup Rp2.000.000: %v", err)
	}
	if _, err := gsvc.Create(ctx, l2, input(5_000_001, 0)); !errors.Is(err, group.ErrGroupTooExpensive) {
		t.Errorf("level 2 membuat grup di atas Rp5.000.000: err = %v, want ErrGroupTooExpensive", err)
	}

	// bergabung: level 1 ditolak karena syarat level DAN nilai iuran
	if _, err := gsvc.Join(ctx, newbie, big.InviteCode); !errors.Is(err, group.ErrLevelTooLow) {
		t.Errorf("level 1 bergabung ke grup min level 2: err = %v, want ErrLevelTooLow", err)
	}
	// grup tanpa syarat level tetapi bernilai besar: ditolak oleh batas nilai
	open, err := gsvc.Create(ctx, l2, input(3_000_000, 1))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := gsvc.Join(ctx, newbie, open.InviteCode); !errors.Is(err, group.ErrGroupTooExpensive) {
		t.Errorf("level 1 bergabung ke grup Rp3.000.000: err = %v, want ErrGroupTooExpensive", err)
	}

	// pengguna level 2 lain boleh bergabung
	other := f.e.NewUser("other2")
	f.setKYC(other, "verified")
	f.doneGroups(other, 1)
	if _, err := gsvc.Join(ctx, other, big.InviteCode); err != nil {
		t.Errorf("level 2 bergabung: %v", err)
	}

	// keterlambatan menurunkan level, dan haknya ikut hilang seketika
	g, members := f.runningGroup(other)
	f.markPaid(f.contribution(g, 1, members[0].ID), false)
	if _, err := gsvc.Create(ctx, other, input(2_000_000, 0)); !errors.Is(err, group.ErrGroupTooExpensive) {
		t.Errorf("setelah terlambat (turun ke level 1): err = %v, want ErrGroupTooExpensive", err)
	}
}

// Bobot kocokan berbobot benar-benar berasal dari level reputasi yang dihitung dari data nyata.
func TestWeightedDrawUsesRealReputationWeights(t *testing.T) {
	f := newFx(t)
	ctx := f.ctx

	l1 := f.e.NewUser("l1")
	l2 := f.e.NewUser("l2")
	f.setKYC(l2, "verified")
	f.doneGroups(l2, 1)
	l3 := f.e.NewUser("l3")
	f.setKYC(l3, "verified")
	f.doneGroups(l3, 3)

	g, members := f.e.NewGroupFor(testutil.GroupSpec{Slots: 3, DrawMethod: "weighted"}, l1, l2, l3)
	groupRepo := group.NewRepository(f.e.Pool)
	svc := draw.NewService(groupRepo, draw.NewRepository(f.e.Pool), f.svc)

	d, err := svc.Commit(ctx, l1, g.ID)
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	weight := map[string]float64{}
	for _, e := range d.Entries {
		weight[e.MemberID] = e.Weight
	}
	for i, want := range []float64{1, 2, 3} {
		if got := weight[members[i].ID]; got != want {
			t.Errorf("bobot anggota level %d = %v, want %v", i+1, got, want)
		}
	}
}

// Kocokan acak (random) mengabaikan reputasi: semua bobot setara.
func TestRandomDrawIgnoresReputation(t *testing.T) {
	f := newFx(t)
	l1, l3 := f.e.NewUser("l1"), f.e.NewUser("l3")
	f.setKYC(l3, "verified")
	f.doneGroups(l3, 3)
	g, _ := f.e.NewGroupFor(testutil.GroupSpec{Slots: 2, DrawMethod: "random"}, l1, l3)

	d, err := draw.NewService(group.NewRepository(f.e.Pool), draw.NewRepository(f.e.Pool), f.svc).Commit(f.ctx, l1, g.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range d.Entries {
		if e.Weight != 1 {
			t.Errorf("bobot random = %v, want 1", e.Weight)
		}
	}
}
