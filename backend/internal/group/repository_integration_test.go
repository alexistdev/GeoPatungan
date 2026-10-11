/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package group

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Tes ini butuh database Postgres dengan semua migrasi sudah dijalankan.
// Dilewati otomatis jika TEST_DATABASE_URL tidak diisi.
//
//	TEST_DATABASE_URL="postgres://geopatungan:geopatungan_dev@localhost:5432/geopatungan?sslmode=disable" go test ./internal/group/
type env struct {
	repo   *Repository
	pool   *pgxpool.Pool
	prefix string
	seq    int
}

func newEnv(t *testing.T) *env {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL tidak diisi; tes integrasi dilewati")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("gagal konek: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping gagal: %v", err)
	}

	e := &env{repo: NewRepository(pool), pool: pool, prefix: fmt.Sprintf("gtest-%d", time.Now().UnixNano())}
	t.Cleanup(func() {
		// grup dihapus dulu (anggotanya ikut via CASCADE), baru user uji.
		_, _ = pool.Exec(ctx, `DELETE FROM groups WHERE owner_id IN (SELECT id FROM users WHERE email LIKE $1)`, e.prefix+"-%")
		_, _ = pool.Exec(ctx, `DELETE FROM group_members WHERE user_id IN (SELECT id FROM users WHERE email LIKE $1)`, e.prefix+"-%")
		_, _ = pool.Exec(ctx, `DELETE FROM users WHERE email LIKE $1`, e.prefix+"-%")
		pool.Close()
	})
	return e
}

func (e *env) newUser(t *testing.T, name string) string {
	t.Helper()
	e.seq++
	var id string
	err := e.pool.QueryRow(context.Background(),
		`INSERT INTO users (name, phone, email, password_hash, referral_code)
		 VALUES ($1, $2, $3, 'x', $4) RETURNING id`,
		name, fmt.Sprintf("%s-%d", e.prefix, e.seq), fmt.Sprintf("%s-%s@example.com", e.prefix, name),
		fmt.Sprintf("%s-%d", e.prefix, e.seq),
	).Scan(&id)
	if err != nil {
		t.Fatalf("buat user %s: %v", name, err)
	}
	return id
}

func (e *env) newGroup(t *testing.T, ownerID string, slots int) *Group {
	t.Helper()
	e.seq++
	g := &Group{
		OwnerID: ownerID, Name: "Grup Uji", ContributionAmount: 100_000, PeriodType: "monthly",
		SlotCount: slots, StartDate: "2026-12-01", LateFee: 5_000, DrawMethod: "random", MinLevelRank: 1,
		InviteCode: fmt.Sprintf("%s-%d", e.prefix, e.seq),
	}
	if err := e.repo.Create(context.Background(), g); err != nil {
		t.Fatalf("Create grup: %v", err)
	}
	return g
}

func TestRepoCreateAndFind(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	owner := e.newUser(t, "owner")
	g := e.newGroup(t, owner, 5)

	if g.ID == "" || g.Status != "open" || g.CreatedAt.IsZero() || g.ActiveMembers != 1 {
		t.Errorf("hasil Create tidak lengkap: %+v", g)
	}

	got, err := e.repo.FindByID(ctx, g.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.StartDate != "2026-12-01" || got.ContributionAmount != 100_000 || got.ActiveMembers != 1 || got.InviteCode != g.InviteCode {
		t.Errorf("FindByID: %+v", got)
	}
	if byCode, err := e.repo.FindByInviteCode(ctx, g.InviteCode); err != nil || byCode.ID != g.ID {
		t.Errorf("FindByInviteCode: %+v, %v", byCode, err)
	}
	if _, err := e.repo.FindByInviteCode(ctx, "tidak-ada"); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}

	// pemilik otomatis jadi anggota aktif
	m, err := e.repo.FindMember(ctx, g.ID, owner)
	if err != nil || m.Status != "active" {
		t.Errorf("keanggotaan pemilik: %+v, %v", m, err)
	}
}

func TestRepoDuplicateInviteCode(t *testing.T) {
	e := newEnv(t)
	owner := e.newUser(t, "owner")
	g := e.newGroup(t, owner, 5)

	dup := *g
	dup.ID = ""
	if err := e.repo.Create(context.Background(), &dup); !errors.Is(err, ErrDuplicate) {
		t.Errorf("err = %v, want ErrDuplicate", err)
	}
}

func TestRepoJoinApproveReject(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	owner := e.newUser(t, "owner")
	a, b := e.newUser(t, "a"), e.newUser(t, "b")
	g := e.newGroup(t, owner, 5)

	ma, err := e.repo.AddMember(ctx, g.ID, a)
	if err != nil || ma.Status != "pending" {
		t.Fatalf("AddMember: %+v, %v", ma, err)
	}
	if _, err := e.repo.AddMember(ctx, g.ID, a); !errors.Is(err, ErrAlreadyMember) {
		t.Errorf("join dua kali: err = %v, want ErrAlreadyMember", err)
	}
	mb, _ := e.repo.AddMember(ctx, g.ID, b)

	if err := e.repo.ApproveMember(ctx, g.ID, ma.ID); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if err := e.repo.ApproveMember(ctx, g.ID, ma.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("approve ulang: err = %v, want ErrNotFound", err)
	}
	if err := e.repo.RejectMember(ctx, g.ID, mb.ID); err != nil {
		t.Fatalf("Reject: %v", err)
	}
	if err := e.repo.RejectMember(ctx, g.ID, mb.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("reject ulang: err = %v, want ErrNotFound", err)
	}

	// member id dari grup lain tidak boleh bisa disetujui lewat grup ini
	other := e.newGroup(t, owner, 5)
	if err := e.repo.ApproveMember(ctx, other.ID, ma.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("approve lintas grup: err = %v, want ErrNotFound", err)
	}

	members, err := e.repo.ListMembers(ctx, g.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 2 { // pemilik + a; b ditolak (removed) tidak ditampilkan
		t.Errorf("len(members) = %d, want 2", len(members))
	}

	mine, err := e.repo.ListByUser(ctx, a)
	if err != nil || len(mine) != 1 || mine[0].MyStatus != "active" || mine[0].ActiveMembers != 2 {
		t.Errorf("ListByUser(a): %+v, %v", mine, err)
	}
	if rejected, _ := e.repo.ListByUser(ctx, b); len(rejected) != 0 {
		t.Errorf("user yang ditolak seharusnya tidak melihat grup, got %d", len(rejected))
	}
}

func TestRepoApproveRespectsSlotLimit(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	owner := e.newUser(t, "owner")
	g := e.newGroup(t, owner, 2) // pemilik + 1 slot

	var pending []*Member
	for _, name := range []string{"a", "b", "c"} {
		m, err := e.repo.AddMember(ctx, g.ID, e.newUser(t, name))
		if err != nil {
			t.Fatal(err)
		}
		pending = append(pending, m)
	}

	if err := e.repo.ApproveMember(ctx, g.ID, pending[0].ID); err != nil {
		t.Fatalf("approve pertama: %v", err)
	}
	if err := e.repo.ApproveMember(ctx, g.ID, pending[1].ID); !errors.Is(err, ErrGroupFull) {
		t.Errorf("approve saat penuh: err = %v, want ErrGroupFull", err)
	}
}

// Dua persetujuan bersamaan untuk slot terakhir: tepat satu yang boleh menang.
func TestRepoConcurrentApproveCannotExceedSlots(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	owner := e.newUser(t, "owner")
	g := e.newGroup(t, owner, 2) // tersisa 1 slot

	var members []*Member
	for i := 0; i < 8; i++ {
		m, err := e.repo.AddMember(ctx, g.ID, e.newUser(t, fmt.Sprintf("c%d", i)))
		if err != nil {
			t.Fatal(err)
		}
		members = append(members, m)
	}

	var wg sync.WaitGroup
	results := make([]error, len(members))
	for i, m := range members {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = e.repo.ApproveMember(ctx, g.ID, m.ID)
		}()
	}
	wg.Wait()

	ok := 0
	for _, err := range results {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, ErrGroupFull):
		default:
			t.Errorf("error tak terduga: %v", err)
		}
	}
	if ok != 1 {
		t.Errorf("%d persetujuan berhasil, want tepat 1", ok)
	}
	got, _ := e.repo.FindByID(ctx, g.ID)
	if got.ActiveMembers != 2 {
		t.Errorf("ActiveMembers = %d, want 2 (tidak boleh melebihi slot)", got.ActiveMembers)
	}
}

// Tes deterministik untuk kunci baris: selama transaksi lain memegang
// FOR UPDATE pada grup, ApproveMember harus menunggu, bukan lanjut begitu saja.
// (Tes balapan di atas bergantung pada waktu dan bisa lolos walau kunci hilang.)
func TestRepoApproveLocksGroupRow(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	owner := e.newUser(t, "owner")
	g := e.newGroup(t, owner, 3)
	m, err := e.repo.AddMember(ctx, g.ID, e.newUser(t, "a"))
	if err != nil {
		t.Fatal(err)
	}

	tx, err := e.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT 1 FROM groups WHERE id = $1 FOR UPDATE`, g.ID); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() { done <- e.repo.ApproveMember(ctx, g.ID, m.ID) }()

	select {
	case err := <-done:
		t.Fatalf("ApproveMember selesai (err=%v) padahal baris grup terkunci; seharusnya menunggu", err)
	case <-time.After(300 * time.Millisecond):
	}

	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("ApproveMember setelah kunci dilepas: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ApproveMember tidak selesai setelah kunci dilepas")
	}
}
