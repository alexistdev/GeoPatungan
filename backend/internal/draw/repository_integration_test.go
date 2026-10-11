/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package draw

import (
	"context"
	crand "crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/alexistdev/geopatungan/internal/group"
)

// Tes ini butuh database Postgres dengan semua migrasi sudah dijalankan.
// Dilewati otomatis jika TEST_DATABASE_URL tidak diisi.
//
//	TEST_DATABASE_URL="postgres://geopatungan:geopatungan_dev@localhost:5432/geopatungan?sslmode=disable" go test ./internal/draw/
type env struct {
	repo   *Repository
	groups *group.Repository
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
	e := &env{repo: NewRepository(pool), groups: group.NewRepository(pool), pool: pool,
		prefix: fmt.Sprintf("dtest-%d", time.Now().UnixNano())}
	t.Cleanup(func() {
		like := e.prefix + "-%"
		_, _ = pool.Exec(ctx, `DELETE FROM groups WHERE owner_id IN (SELECT id FROM users WHERE email LIKE $1)`, like)
		_, _ = pool.Exec(ctx, `DELETE FROM group_members WHERE user_id IN (SELECT id FROM users WHERE email LIKE $1)`, like)
		_, _ = pool.Exec(ctx, `DELETE FROM users WHERE email LIKE $1`, like)
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
		fmt.Sprintf("%s-%d", e.prefix, e.seq)).Scan(&id)
	if err != nil {
		t.Fatalf("buat user: %v", err)
	}
	return id
}

// newGroup membuat grup dengan `slots` slot, `active` di antaranya anggota aktif
// (pemilik + peserta yang disetujui), sisanya kosong. Mengembalikan grup dan anggota aktif.
func (e *env) newGroup(t *testing.T, slots, active int, method string) (*group.Group, []group.Member) {
	t.Helper()
	ctx := context.Background()
	e.seq++
	owner := e.newUser(t, fmt.Sprintf("owner%d", e.seq))
	g := &group.Group{
		OwnerID: owner, Name: "Grup Kocokan", ContributionAmount: 100_000, PeriodType: "monthly",
		SlotCount: slots, StartDate: "2026-12-01", DrawMethod: method, MinLevelRank: 1,
		InviteCode: fmt.Sprintf("%s-g%d", e.prefix, e.seq),
	}
	if err := e.groups.Create(ctx, g); err != nil {
		t.Fatalf("buat grup: %v", err)
	}
	for i := 1; i < active; i++ {
		uid := e.newUser(t, fmt.Sprintf("m%d-%d", e.seq, i))
		m, err := e.groups.AddMember(ctx, g.ID, uid)
		if err != nil {
			t.Fatal(err)
		}
		if err := e.groups.ApproveMember(ctx, g.ID, m.ID); err != nil {
			t.Fatal(err)
		}
	}
	members, err := e.groups.ListMembers(ctx, g.ID)
	if err != nil {
		t.Fatal(err)
	}
	return g, members
}

func entriesFor(members []group.Member, weights ...float64) []Entry {
	out := make([]Entry, len(members))
	for i, m := range members {
		w := 1.0
		if i < len(weights) {
			w = weights[i]
		}
		out[i] = Entry{MemberID: m.ID, UserName: m.UserName, Weight: w}
	}
	return out
}

func newDraw(t *testing.T, groupID string, entries []Entry) (*Draw, []byte) {
	t.Helper()
	seed, err := NewSeed(testRand())
	if err != nil {
		t.Fatal(err)
	}
	return &Draw{GroupID: groupID, Algorithm: Algorithm, SeedCommit: CommitOf(seed),
		Entries: entries, seed: hex.EncodeToString(seed)}, seed
}

func (e *env) groupStatus(t *testing.T, id string) string {
	t.Helper()
	var s string
	if err := e.pool.QueryRow(context.Background(), `SELECT status FROM groups WHERE id = $1`, id).Scan(&s); err != nil {
		t.Fatal(err)
	}
	return s
}

func (e *env) turnOrders(t *testing.T, groupID string) map[string]int {
	t.Helper()
	rows, err := e.pool.Query(context.Background(),
		`SELECT id, COALESCE(turn_order, 0) FROM group_members WHERE group_id = $1 AND status = 'active'`, groupID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var id string
		var o int
		if err := rows.Scan(&id, &o); err != nil {
			t.Fatal(err)
		}
		out[id] = o
	}
	return out
}

func TestRepoCommitExecuteFlow(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	g, members := e.newGroup(t, 4, 4, "weighted")

	d, seed := newDraw(t, g.ID, entriesFor(members, 1, 2, 3, 1))
	if err := e.repo.Commit(ctx, d); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if d.ID == "" || d.CommittedAt.IsZero() {
		t.Errorf("Commit tidak mengisi ID/CommittedAt: %+v", d)
	}

	// setelah commit: komitmen dan bobot terlihat, seed tertutup, belum ada urutan
	got, err := e.repo.Get(ctx, g.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.SeedCommit != d.SeedCommit || got.SeedReveal != "" || got.ExecutedAt != nil {
		t.Errorf("setelah commit: %+v", got)
	}
	if len(got.Entries) != 4 {
		t.Fatalf("len(entries) = %d, want 4", len(got.Entries))
	}
	for _, en := range got.Entries {
		if en.Order != nil || en.Score != nil {
			t.Error("urutan/skor belum boleh terisi sebelum execute")
		}
	}
	for _, m := range e.turnOrders(t, g.ID) {
		if m != 0 {
			t.Error("turn_order belum boleh terisi sebelum execute")
		}
	}
	if e.groupStatus(t, g.ID) != "open" {
		t.Error("grup masih harus open setelah commit")
	}

	// execute
	ordered, err := ComputeOrder(seed, got.Entries)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.repo.Execute(ctx, g.ID, ordered); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	done, err := e.repo.Get(ctx, g.ID)
	if err != nil {
		t.Fatal(err)
	}
	if done.ExecutedAt == nil {
		t.Error("ExecutedAt harus terisi")
	}
	if done.SeedReveal == "" || !VerifyCommit(done.SeedReveal, done.SeedCommit) {
		t.Error("seed harus terbuka dan cocok dengan komitmen")
	}
	if e.groupStatus(t, g.ID) != "running" {
		t.Errorf("status grup = %s, want running", e.groupStatus(t, g.ID))
	}

	orders := e.turnOrders(t, g.ID)
	seen := map[int]bool{}
	for i, en := range done.Entries {
		if en.Order == nil || *en.Order != i+1 || en.Score == nil {
			t.Errorf("entri %d: order=%v score=%v", i, en.Order, en.Score)
		}
		if orders[en.MemberID] != i+1 {
			t.Errorf("turn_order %s = %d, want %d", en.MemberID, orders[en.MemberID], i+1)
		}
		seen[orders[en.MemberID]] = true
	}
	if len(seen) != 4 {
		t.Errorf("turn_order harus 4 nilai berbeda, got %v", orders)
	}

	// hasil di DB sama dengan perhitungan ulang dari seed yang dibuka
	revealed, _ := hex.DecodeString(done.SeedReveal)
	again, _ := ComputeOrder(revealed, entriesFor(members, 1, 2, 3, 1))
	for i := range again {
		if done.Entries[i].MemberID != again[i].MemberID {
			t.Errorf("verifikasi ulang berbeda di posisi %d", i)
		}
	}

	// tidak bisa diulang
	if err := e.repo.Execute(ctx, g.ID, ordered); !errors.Is(err, ErrAlreadyExecuted) {
		t.Errorf("execute ulang: err = %v, want ErrAlreadyExecuted", err)
	}
}

func TestRepoCommitRejections(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	t.Run("slot belum penuh", func(t *testing.T) {
		g, members := e.newGroup(t, 4, 3, "random")
		d, _ := newDraw(t, g.ID, entriesFor(members))
		if err := e.repo.Commit(ctx, d); !errors.Is(err, ErrGroupNotFull) {
			t.Errorf("err = %v, want ErrGroupNotFull", err)
		}
		if _, err := e.repo.Get(ctx, g.ID); !errors.Is(err, ErrNoDraw) {
			t.Error("commit gagal tidak boleh meninggalkan data")
		}
	})

	t.Run("entri tidak sama dengan anggota aktif", func(t *testing.T) {
		g, members := e.newGroup(t, 3, 3, "random")
		entries := entriesFor(members)
		entries[2].MemberID = "00000000-0000-0000-0000-000000000000"
		d, _ := newDraw(t, g.ID, entries)
		if err := e.repo.Commit(ctx, d); !errors.Is(err, ErrMembersChanged) {
			t.Errorf("err = %v, want ErrMembersChanged", err)
		}
	})

	t.Run("commit dua kali", func(t *testing.T) {
		g, members := e.newGroup(t, 3, 3, "random")
		d1, _ := newDraw(t, g.ID, entriesFor(members))
		if err := e.repo.Commit(ctx, d1); err != nil {
			t.Fatal(err)
		}
		d2, _ := newDraw(t, g.ID, entriesFor(members))
		if err := e.repo.Commit(ctx, d2); !errors.Is(err, ErrAlreadyCommitted) {
			t.Errorf("err = %v, want ErrAlreadyCommitted", err)
		}
		// komitmen pertama tidak boleh tertimpa
		got, _ := e.repo.Get(ctx, g.ID)
		if got.SeedCommit != d1.SeedCommit {
			t.Error("komitmen pertama tertimpa oleh commit kedua")
		}
	})

	t.Run("grup tidak ada", func(t *testing.T) {
		d, _ := newDraw(t, "00000000-0000-0000-0000-000000000000", nil)
		if err := e.repo.Commit(ctx, d); !errors.Is(err, group.ErrNotFound) {
			t.Errorf("err = %v, want group.ErrNotFound", err)
		}
	})
}

func TestRepoExecuteWithoutCommit(t *testing.T) {
	e := newEnv(t)
	g, _ := e.newGroup(t, 2, 2, "random")
	if err := e.repo.Execute(context.Background(), g.ID, nil); !errors.Is(err, ErrNoDraw) {
		t.Errorf("err = %v, want ErrNoDraw", err)
	}
}

// Execute yang gagal di tengah jalan tidak boleh meninggalkan hasil setengah jadi.
func TestRepoExecuteIsAtomic(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	g, members := e.newGroup(t, 3, 3, "random")
	d, seed := newDraw(t, g.ID, entriesFor(members))
	if err := e.repo.Commit(ctx, d); err != nil {
		t.Fatal(err)
	}
	got, _ := e.repo.Get(ctx, g.ID)
	ordered, _ := ComputeOrder(seed, got.Entries)

	// entri terakhir menunjuk anggota yang tidak ada di kocokan -> gagal setelah dua update berhasil
	broken := append([]Entry{}, ordered...)
	broken[2].MemberID = "00000000-0000-0000-0000-000000000000"
	if err := e.repo.Execute(ctx, g.ID, broken); !errors.Is(err, ErrMembersChanged) {
		t.Fatalf("err = %v, want ErrMembersChanged", err)
	}

	after, _ := e.repo.Get(ctx, g.ID)
	if after.ExecutedAt != nil || after.SeedReveal != "" {
		t.Error("kocokan tidak boleh tercatat selesai")
	}
	for _, en := range after.Entries {
		if en.Order != nil || en.Score != nil {
			t.Error("urutan tidak boleh tersimpan sebagian")
		}
	}
	for id, o := range e.turnOrders(t, g.ID) {
		if o != 0 {
			t.Errorf("turn_order %s = %d, seharusnya masih kosong", id, o)
		}
	}
	if e.groupStatus(t, g.ID) != "open" {
		t.Error("grup harus tetap open")
	}

	// dan eksekusi yang benar tetap bisa dilakukan sesudahnya
	if err := e.repo.Execute(ctx, g.ID, ordered); err != nil {
		t.Errorf("execute yang benar setelah kegagalan: %v", err)
	}
}

// Beberapa execute bersamaan: tepat satu menang, sisanya mendapat ErrAlreadyExecuted.
func TestRepoConcurrentExecute(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	g, members := e.newGroup(t, 6, 6, "random")
	d, seed := newDraw(t, g.ID, entriesFor(members))
	if err := e.repo.Commit(ctx, d); err != nil {
		t.Fatal(err)
	}
	got, _ := e.repo.Get(ctx, g.ID)
	ordered, _ := ComputeOrder(seed, got.Entries)

	const n = 8
	results := make([]error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = e.repo.Execute(ctx, g.ID, ordered)
		}()
	}
	wg.Wait()

	ok := 0
	for _, err := range results {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, ErrAlreadyExecuted):
		default:
			t.Errorf("error tak terduga: %v", err)
		}
	}
	if ok != 1 {
		t.Errorf("%d execute berhasil, want tepat 1", ok)
	}
}

func TestRepoManual(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	g, members := e.newGroup(t, 3, 3, "manual")

	entries := entriesFor(members)
	for i := range entries {
		pos := len(entries) - i // urutan dibalik
		entries[i].Order = &pos
	}
	d := &Draw{GroupID: g.ID, Algorithm: "manual", Entries: entries}
	if err := e.repo.Manual(ctx, d); err != nil {
		t.Fatalf("Manual: %v", err)
	}

	got, err := e.repo.Get(ctx, g.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Algorithm != "manual" || got.ExecutedAt == nil || got.SeedCommit != "" || got.SeedReveal != "" {
		t.Errorf("draw manual: %+v", got)
	}
	orders := e.turnOrders(t, g.ID)
	for _, en := range entries {
		if orders[en.MemberID] != *en.Order {
			t.Errorf("turn_order %s = %d, want %d", en.MemberID, orders[en.MemberID], *en.Order)
		}
	}
	if e.groupStatus(t, g.ID) != "running" {
		t.Error("grup harus running setelah urutan manual")
	}

	d2 := &Draw{GroupID: g.ID, Algorithm: "manual", Entries: entries}
	if err := e.repo.Manual(ctx, d2); !errors.Is(err, ErrGroupStarted) {
		t.Errorf("manual kedua: err = %v, want ErrGroupStarted", err)
	}
}

func TestRepoGetNoDraw(t *testing.T) {
	e := newEnv(t)
	g, _ := e.newGroup(t, 2, 2, "random")
	if _, err := e.repo.Get(context.Background(), g.ID); !errors.Is(err, ErrNoDraw) {
		t.Errorf("err = %v, want ErrNoDraw", err)
	}
}

// testRand mengembalikan sumber acak kriptografis untuk membuat seed di tes.
func testRand() io.Reader { return crand.Reader }
