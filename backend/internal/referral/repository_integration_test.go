/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package referral

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/alexistdev/geopatungan/internal/round"
	"github.com/alexistdev/geopatungan/internal/testutil"
)

// Tes ini butuh database Postgres dengan semua migrasi; dilewati jika TEST_DATABASE_URL kosong.

func chain(e *testutil.Env) []string {
	ids := []string{e.NewUser("a")}
	for _, n := range []string{"b", "c", "d", "e"} {
		ids = append(ids, e.NewReferredUser(n, ids[len(ids)-1]))
	}
	return ids
}

func TestRegistrationBuildsClosureOfFourLevels(t *testing.T) {
	e := testutil.NewEnv(t)
	ch := chain(e) // A B C D E
	for i, want := range []int{1, 2, 3, 4} {
		// ancestor level `want` dari E
		got := e.QueryString(`SELECT ancestor_id::text FROM referrals WHERE descendant_id = $1 AND level = $2`, ch[4], want)
		if got != ch[4-want] {
			t.Errorf("level %d (%d): ancestor = %s, want %s", want, i, got, ch[4-want])
		}
	}
	if n := e.QueryInt(`SELECT count(*) FROM referrals WHERE descendant_id = $1`, ch[4]); n != 4 {
		t.Errorf("E punya %d ancestor, want 4", n)
	}
	// kedalaman maksimal 4: F di bawah E hanya melihat 4 ancestor (B..E), A tidak termasuk
	f := e.NewReferredUser("f", ch[4])
	if n := e.QueryInt(`SELECT count(*) FROM referrals WHERE descendant_id = $1`, f); n != 4 {
		t.Errorf("F punya %d ancestor, want 4", n)
	}
	if n := e.QueryInt(`SELECT count(*) FROM referrals WHERE descendant_id = $1 AND ancestor_id = $2`, f, ch[0]); n != 0 {
		t.Error("A (level 5 dari F) tidak boleh tercatat")
	}
}

func TestPlanOnlyVerifiedAndPositiveShares(t *testing.T) {
	e := testutil.NewEnv(t)
	ch := chain(e)
	ctx := context.Background()

	plan, err := Plan(ctx, e.Pool, ch[4])
	if err != nil || len(plan) != 0 {
		t.Fatalf("tanpa upline terverifikasi: plan = %+v, %v", plan, err)
	}
	e.Verify(ch[3], ch[1]) // D (level 1) dan B (level 3)
	plan, err = Plan(ctx, e.Pool, ch[4])
	if err != nil || len(plan) != 2 {
		t.Fatalf("plan = %+v, %v", plan, err)
	}
	if plan[0] != (Beneficiary{Level: 1, UserID: ch[3], ShareBps: 4000}) || plan[1] != (Beneficiary{Level: 3, UserID: ch[1], ShareBps: 1500}) {
		t.Errorf("plan = %+v", plan)
	}
	// pengguna tanpa upline
	if p, err := Plan(ctx, e.Pool, ch[0]); err != nil || len(p) != 0 {
		t.Errorf("A: plan = %+v, %v", p, err)
	}
}

func TestSummaryAndCommissionPaging(t *testing.T) {
	e := testutil.NewEnv(t)
	ch := chain(e)
	e.Verify(ch[0])
	ctx := context.Background()
	repo := NewRepository(e.Pool)

	s, err := repo.Summary(ctx, ch[0])
	if err != nil {
		t.Fatal(err)
	}
	if s.ReferralCode == "" || !s.Eligible {
		t.Errorf("summary = %+v", s)
	}
	if d := s.Downline; d.Level1 != 1 || d.Level2 != 1 || d.Level3 != 1 || d.Level4 != 1 || d.Total != 4 {
		t.Errorf("downline = %+v", d)
	}
	if s.Earned.Total != 0 || len(s.Earned.ByLevel) != 4 {
		t.Errorf("earned = %+v", s.Earned)
	}
	if len(s.Rules) != 4 || s.Rules[0].ShareBps != 4000 {
		t.Errorf("rules = %+v", s.Rules)
	}
	if bs, _ := repo.Summary(ctx, ch[1]); bs.Eligible {
		t.Error("B belum terverifikasi, tidak boleh eligible")
	}
	if _, err := repo.Summary(ctx, "00000000-0000-0000-0000-000000000000"); err != ErrNotFound {
		t.Errorf("err = %v, want ErrNotFound", err)
	}

	// riwayat komisi: tiga iuran nyata dari grup berisi E, lalu baris komisi ke A
	g, members := e.NewGroupFor(testutil.GroupSpec{Amount: 100_000}, ch[4], e.NewUser("p"), e.NewUser("q"))
	e.SetTurnOrders(members)
	e.Exec(`UPDATE groups SET status = 'running' WHERE id = $1`, g.ID)
	if err := e.InTx(func(tx pgx.Tx) error { return round.CreateSchedule(ctx, tx, g.ID) }); err != nil {
		t.Fatal(err)
	}
	rows, err := e.Pool.Query(ctx, `SELECT c.id::text FROM contributions c JOIN rounds r ON r.id = c.round_id
	                                 WHERE r.group_id = $1 AND r.round_no = 1 ORDER BY c.id`, g.ID)
	if err != nil {
		t.Fatal(err)
	}
	var cids []string
	for rows.Next() {
		var id string
		_ = rows.Scan(&id)
		cids = append(cids, id)
	}
	rows.Close()
	if len(cids) != 3 {
		t.Fatalf("iuran = %d", len(cids))
	}
	for i, cid := range cids {
		if err := e.InTx(func(tx pgx.Tx) error {
			return Record(ctx, tx, cid, ch[4], []Commission{{Level: 4, UserID: ch[0], ShareBps: 1000, Amount: int64(100 * (i + 1))}})
		}); err != nil {
			t.Fatal(err)
		}
	}
	s, err = repo.Summary(ctx, ch[0])
	if err != nil || s.Earned.Total != 600 || s.Earned.ByLevel["4"] != 600 || s.Earned.ByLevel["1"] != 0 {
		t.Fatalf("earned = %+v, %v", s.Earned, err)
	}

	// paging dengan limit 2: 3 entri -> 2 + 1, terbaru dulu, tanpa ganda
	svc := NewService(repo)
	p1, err := svc.Commissions(ctx, ch[0], 2, 0)
	if err != nil || len(p1.Commissions) != 2 || p1.NextBefore == nil {
		t.Fatalf("halaman 1 = %+v, %v", p1, err)
	}
	p2, err := svc.Commissions(ctx, ch[0], 2, *p1.NextBefore)
	if err != nil || len(p2.Commissions) != 1 || p2.NextBefore != nil {
		t.Fatalf("halaman 2 = %+v, %v", p2, err)
	}
	if p1.Commissions[0].ID <= p1.Commissions[1].ID || p1.Commissions[1].ID <= p2.Commissions[0].ID {
		t.Error("urutan harus terbaru dulu")
	}
	for _, c := range append(p1.Commissions, p2.Commissions...) {
		if strings.Contains(c.SourceName, "e-") || !strings.HasSuffix(c.SourceName, "***") {
			t.Errorf("nama sumber tidak disamarkan: %q", c.SourceName)
		}
	}
	// pengguna lain tidak melihat komisi A
	if other, err := svc.Commissions(ctx, ch[1], 10, 0); err != nil || len(other.Commissions) != 0 {
		t.Errorf("komisi bocor ke pengguna lain: %+v, %v", other, err)
	}
}

func TestRecordRejectsDuplicateLevelPerContribution(t *testing.T) {
	e := testutil.NewEnv(t)
	ch := chain(e)
	ctx := context.Background()
	g, members := e.NewGroupFor(testutil.GroupSpec{Amount: 100_000}, ch[4], e.NewUser("p"))
	e.SetTurnOrders(members)
	e.Exec(`UPDATE groups SET status = 'running' WHERE id = $1`, g.ID)
	if err := e.InTx(func(tx pgx.Tx) error { return round.CreateSchedule(ctx, tx, g.ID) }); err != nil {
		t.Fatal(err)
	}
	cid := e.QueryString(`SELECT c.id::text FROM contributions c JOIN rounds r ON r.id = c.round_id
	                       WHERE r.group_id = $1 AND r.round_no = 1 LIMIT 1`, g.ID)
	cs := []Commission{{Level: 1, UserID: ch[3], ShareBps: 4000, Amount: 800}}
	if err := e.InTx(func(tx pgx.Tx) error { return Record(ctx, tx, cid, ch[4], cs) }); err != nil {
		t.Fatal(err)
	}
	if err := e.InTx(func(tx pgx.Tx) error { return Record(ctx, tx, cid, ch[4], cs) }); err == nil {
		t.Error("komisi ganda untuk iuran+level yang sama harus ditolak")
	}
	// pengguna tidak boleh menjadi penerima komisi dari dirinya sendiri
	self := []Commission{{Level: 2, UserID: ch[4], ShareBps: 2500, Amount: 500}}
	if err := e.InTx(func(tx pgx.Tx) error { return Record(ctx, tx, cid, ch[4], self) }); err == nil {
		t.Error("komisi ke diri sendiri harus ditolak")
	}
	// jumlah nol/negatif ditolak
	for _, amt := range []int64{0, -5} {
		bad := []Commission{{Level: 3, UserID: ch[2], ShareBps: 1500, Amount: amt}}
		if err := e.InTx(func(tx pgx.Tx) error { return Record(ctx, tx, cid, ch[4], bad) }); err == nil {
			t.Errorf("jumlah %d harus ditolak", amt)
		}
	}
}

func TestRuleUpdateIsAuditedAndTriggerGuardsTotal(t *testing.T) {
	e := testutil.NewEnv(t)
	admin := e.NewAdmin("adm")
	ctx := context.Background()
	repo := NewRepository(e.Pool)

	// perubahan sah: tercatat di audit; dibatalkan supaya aturan sungguhan tidak berubah
	good := []Rule{{1, 3000}, {2, 2000}, {3, 1000}, {4, 500}}
	tx, err := e.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := saveRules(ctx, tx, admin, good); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE actor_id = $1 AND action = 'referral.rules.update'`, admin).Scan(&n); err != nil || n != 1 {
		t.Errorf("audit = %d, %v", n, err)
	}
	_ = tx.Rollback(ctx)

	// trigger: total > 100% ditolak walau lolos service (lapisan pertahanan terakhir)
	over := []Rule{{1, 9000}, {2, 9000}, {3, 0}, {4, 0}}
	err = repo.UpdateRules(ctx, admin, over)
	if err == nil || !strings.Contains(err.Error(), "100%") {
		t.Errorf("err = %v, want penolakan total > 100%%", err)
	}
	rules, err := repo.Rules(ctx)
	if err != nil || len(rules) != 4 || rules[0].ShareBps != 4000 || rules[3].ShareBps != 1000 {
		t.Errorf("aturan berubah setelah penolakan: %+v, %v", rules, err)
	}
	if c := e.QueryInt(`SELECT count(*) FROM audit_logs WHERE actor_id = $1`, admin); c != 0 {
		t.Errorf("audit tersimpan (%d) padahal perubahan gagal", c)
	}

	// batas tepat 100% diperbolehkan (dibatalkan, tidak disimpan)
	tx, _ = e.Pool.Begin(ctx)
	exact := []Rule{{1, 5000}, {2, 3000}, {3, 1500}, {4, 500}}
	if err := saveRules(ctx, tx, admin, exact); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `SET CONSTRAINTS ALL IMMEDIATE`); err != nil {
		t.Errorf("tepat 100%% harus lolos: %v", err)
	}
	_ = tx.Rollback(ctx)

	// bagian di luar rentang ditolak CHECK
	tx, _ = e.Pool.Begin(ctx)
	if _, err := tx.Exec(ctx, `UPDATE commission_rules SET share_bps = 10001 WHERE level = 1`); err == nil {
		t.Error("share_bps 10001 harus ditolak")
	}
	_ = tx.Rollback(ctx)
	tx, _ = e.Pool.Begin(ctx)
	if _, err := tx.Exec(ctx, `INSERT INTO commission_rules (level, share_bps) VALUES (5, 100)`); err == nil {
		t.Error("level 5 harus ditolak")
	}
	_ = tx.Rollback(ctx)
}
