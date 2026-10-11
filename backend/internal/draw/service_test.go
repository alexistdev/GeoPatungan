/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package draw

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/alexistdev/geopatungan/internal/group"
)

// ---- fakes di memori -------------------------------------------------------

type fakeGroups struct {
	groups  map[string]*group.Group
	members map[string][]group.Member // groupID -> anggota
}

func (f *fakeGroups) FindByID(_ context.Context, id string) (*group.Group, error) {
	g, ok := f.groups[id]
	if !ok {
		return nil, group.ErrNotFound
	}
	cp := *g
	return &cp, nil
}

func (f *fakeGroups) FindMember(_ context.Context, groupID, userID string) (*group.Member, error) {
	for _, m := range f.members[groupID] {
		if m.UserID == userID {
			cp := m
			return &cp, nil
		}
	}
	return nil, group.ErrNotFound
}

func (f *fakeGroups) ListMembers(_ context.Context, groupID string) ([]group.Member, error) {
	return append([]group.Member{}, f.members[groupID]...), nil
}

type fakeStore struct {
	draws map[string]*Draw // groupID -> draw
	seq   int
}

func (f *fakeStore) Commit(_ context.Context, d *Draw) error {
	if _, exists := f.draws[d.GroupID]; exists {
		return ErrAlreadyCommitted
	}
	f.seq++
	d.ID = fmt.Sprintf("draw-%d", f.seq)
	d.CommittedAt = time.Now()
	cp := *d
	cp.Entries = append([]Entry{}, d.Entries...)
	f.draws[d.GroupID] = &cp
	return nil
}

func (f *fakeStore) Execute(_ context.Context, groupID string, ordered []Entry) error {
	d, ok := f.draws[groupID]
	if !ok {
		return ErrNoDraw
	}
	if d.ExecutedAt != nil {
		return ErrAlreadyExecuted
	}
	now := time.Now()
	d.ExecutedAt = &now
	d.SeedReveal = d.seed
	d.Entries = append([]Entry{}, ordered...)
	return nil
}

func (f *fakeStore) Manual(_ context.Context, d *Draw) error {
	if _, exists := f.draws[d.GroupID]; exists {
		return ErrAlreadyCommitted
	}
	f.seq++
	d.ID = fmt.Sprintf("draw-%d", f.seq)
	now := time.Now()
	d.CommittedAt, d.ExecutedAt = now, &now
	cp := *d
	f.draws[d.GroupID] = &cp
	return nil
}

func (f *fakeStore) Get(_ context.Context, groupID string) (*Draw, error) {
	d, ok := f.draws[groupID]
	if !ok {
		return nil, ErrNoDraw
	}
	cp := *d
	cp.Entries = append([]Entry{}, d.Entries...)
	return &cp, nil
}

// rankByUser memberi level berbeda per pengguna.
type rankByUser map[string]int

func (r rankByUser) Rank(_ context.Context, userID string) (int, error) {
	if v, ok := r[userID]; ok {
		return v, nil
	}
	return 1, nil
}

type fixture struct {
	svc    *Service
	store  *fakeStore
	groups *fakeGroups
	gid    string
	owner  string
	users  []string // owner + peserta lain
}

// newFixture membuat grup berisi `n` anggota aktif (anggota pertama = pemilik).
func newFixture(t *testing.T, n int, method string, ranks rankByUser) *fixture {
	t.Helper()
	gid := "group-1"
	fg := &fakeGroups{groups: map[string]*group.Group{}, members: map[string][]group.Member{}}
	fx := &fixture{store: &fakeStore{draws: map[string]*Draw{}}, groups: fg, gid: gid, owner: "user-0"}
	for i := 0; i < n; i++ {
		uid := fmt.Sprintf("user-%d", i)
		fx.users = append(fx.users, uid)
		fg.members[gid] = append(fg.members[gid], group.Member{
			ID: fmt.Sprintf("member-%d", i), GroupID: gid, UserID: uid,
			UserName: fmt.Sprintf("Nama %d", i), Status: "active",
		})
	}
	fg.groups[gid] = &group.Group{ID: gid, OwnerID: fx.owner, SlotCount: n, Status: "open", DrawMethod: method}
	fx.svc = NewService(fg, fx.store, ranks)
	// seed deterministik untuk tes
	fx.svc.rand = bytes.NewReader(bytes.Repeat([]byte{7}, 32*10))
	return fx
}

// ---- Commit ----------------------------------------------------------------

func TestCommit(t *testing.T) {
	fx := newFixture(t, 4, "random", nil)

	d, err := fx.svc.Commit(context.Background(), fx.owner, fx.gid)
	if err != nil {
		t.Fatalf("Commit error: %v", err)
	}
	wantSeed := bytes.Repeat([]byte{7}, 32)
	if d.SeedCommit != CommitOf(wantSeed) {
		t.Errorf("SeedCommit = %s, want hash dari seed uji", d.SeedCommit)
	}
	if d.SeedReveal != "" {
		t.Error("seed tidak boleh dibuka saat commit")
	}
	if len(d.Entries) != 4 {
		t.Fatalf("len(entries) = %d, want 4", len(d.Entries))
	}
	for _, e := range d.Entries {
		if e.Weight != 1 {
			t.Errorf("random: bobot %v, want 1", e.Weight)
		}
		if e.Order != nil || e.Score != nil {
			t.Error("urutan/skor belum boleh ada sebelum dijalankan")
		}
	}

	// seed rahasia tidak boleh bocor lewat JSON
	body, _ := json.Marshal(d)
	if strings.Contains(string(body), hex.EncodeToString(wantSeed)) {
		t.Error("seed bocor di JSON respons commit")
	}
}

func TestCommitWeightsFollowReputation(t *testing.T) {
	fx := newFixture(t, 3, "weighted", rankByUser{"user-0": 1, "user-1": 2, "user-2": 3})
	d, err := fx.svc.Commit(context.Background(), fx.owner, fx.gid)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]float64{}
	for _, e := range d.Entries {
		got[e.MemberID] = e.Weight
	}
	want := map[string]float64{"member-0": 1, "member-1": 2, "member-2": 3}
	for id, w := range want {
		if got[id] != w {
			t.Errorf("bobot %s = %v, want %v", id, got[id], w)
		}
	}
}

func TestCommitRejections(t *testing.T) {
	ctx := context.Background()

	t.Run("orang luar -> tidak ditemukan", func(t *testing.T) {
		fx := newFixture(t, 3, "random", nil)
		if _, err := fx.svc.Commit(ctx, "stranger", fx.gid); !errors.Is(err, group.ErrNotFound) {
			t.Errorf("err = %v, want ErrNotFound", err)
		}
	})
	t.Run("anggota biasa -> terlarang", func(t *testing.T) {
		fx := newFixture(t, 3, "random", nil)
		if _, err := fx.svc.Commit(ctx, "user-1", fx.gid); !errors.Is(err, group.ErrForbidden) {
			t.Errorf("err = %v, want ErrForbidden", err)
		}
	})
	t.Run("grup tidak ada", func(t *testing.T) {
		fx := newFixture(t, 3, "random", nil)
		if _, err := fx.svc.Commit(ctx, fx.owner, "tidak-ada"); !errors.Is(err, group.ErrNotFound) {
			t.Errorf("err = %v, want ErrNotFound", err)
		}
	})
	t.Run("grup sudah running", func(t *testing.T) {
		fx := newFixture(t, 3, "random", nil)
		fx.groups.groups[fx.gid].Status = "running"
		if _, err := fx.svc.Commit(ctx, fx.owner, fx.gid); !errors.Is(err, ErrGroupStarted) {
			t.Errorf("err = %v, want ErrGroupStarted", err)
		}
	})
	t.Run("slot belum penuh", func(t *testing.T) {
		fx := newFixture(t, 3, "random", nil)
		fx.groups.groups[fx.gid].SlotCount = 5
		if _, err := fx.svc.Commit(ctx, fx.owner, fx.gid); !errors.Is(err, ErrGroupNotFull) {
			t.Errorf("err = %v, want ErrGroupNotFull", err)
		}
	})
	t.Run("anggota pending tidak dihitung", func(t *testing.T) {
		fx := newFixture(t, 3, "random", nil)
		fx.groups.members[fx.gid][2].Status = "pending"
		if _, err := fx.svc.Commit(ctx, fx.owner, fx.gid); !errors.Is(err, ErrGroupNotFull) {
			t.Errorf("err = %v, want ErrGroupNotFull", err)
		}
	})
	t.Run("grup manual tidak bisa di-commit", func(t *testing.T) {
		fx := newFixture(t, 3, "manual", nil)
		if _, err := fx.svc.Commit(ctx, fx.owner, fx.gid); !errors.Is(err, ErrWrongMethod) {
			t.Errorf("err = %v, want ErrWrongMethod", err)
		}
	})
	t.Run("commit dua kali", func(t *testing.T) {
		fx := newFixture(t, 3, "random", nil)
		if _, err := fx.svc.Commit(ctx, fx.owner, fx.gid); err != nil {
			t.Fatal(err)
		}
		if _, err := fx.svc.Commit(ctx, fx.owner, fx.gid); !errors.Is(err, ErrAlreadyCommitted) {
			t.Errorf("err = %v, want ErrAlreadyCommitted", err)
		}
	})
}

// ---- Execute ---------------------------------------------------------------

func TestExecute(t *testing.T) {
	ctx := context.Background()
	fx := newFixture(t, 5, "weighted", rankByUser{"user-1": 2, "user-2": 3})

	committed, err := fx.svc.Commit(ctx, fx.owner, fx.gid)
	if err != nil {
		t.Fatal(err)
	}
	d, err := fx.svc.Execute(ctx, fx.owner, fx.gid)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	if d.ExecutedAt == nil {
		t.Error("ExecutedAt seharusnya terisi")
	}
	if !VerifyCommit(d.SeedReveal, d.SeedCommit) {
		t.Error("seed yang dibuka harus cocok dengan komitmen")
	}
	if d.SeedCommit != committed.SeedCommit {
		t.Error("komitmen tidak boleh berubah antara commit dan execute")
	}

	// verifikasi mandiri, seperti yang akan dilakukan anggota:
	seed, _ := hex.DecodeString(d.SeedReveal)
	in := make([]Entry, len(committed.Entries))
	copy(in, committed.Entries)
	want, err := ComputeOrder(seed, in)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Entries) != len(want) {
		t.Fatalf("len = %d, want %d", len(d.Entries), len(want))
	}
	for i := range want {
		if d.Entries[i].MemberID != want[i].MemberID || *d.Entries[i].Order != i+1 {
			t.Errorf("posisi %d: %s (order %v), verifikasi mandiri menghasilkan %s",
				i, d.Entries[i].MemberID, d.Entries[i].Order, want[i].MemberID)
		}
	}
}

func TestExecuteRejections(t *testing.T) {
	ctx := context.Background()

	t.Run("belum ada commit", func(t *testing.T) {
		fx := newFixture(t, 3, "random", nil)
		if _, err := fx.svc.Execute(ctx, fx.owner, fx.gid); !errors.Is(err, ErrNoDraw) {
			t.Errorf("err = %v, want ErrNoDraw", err)
		}
	})
	t.Run("anggota biasa", func(t *testing.T) {
		fx := newFixture(t, 3, "random", nil)
		fx.svc.Commit(ctx, fx.owner, fx.gid)
		if _, err := fx.svc.Execute(ctx, "user-1", fx.gid); !errors.Is(err, group.ErrForbidden) {
			t.Errorf("err = %v, want ErrForbidden", err)
		}
	})
	t.Run("orang luar", func(t *testing.T) {
		fx := newFixture(t, 3, "random", nil)
		fx.svc.Commit(ctx, fx.owner, fx.gid)
		if _, err := fx.svc.Execute(ctx, "stranger", fx.gid); !errors.Is(err, group.ErrNotFound) {
			t.Errorf("err = %v, want ErrNotFound", err)
		}
	})
	t.Run("dua kali", func(t *testing.T) {
		fx := newFixture(t, 3, "random", nil)
		fx.svc.Commit(ctx, fx.owner, fx.gid)
		if _, err := fx.svc.Execute(ctx, fx.owner, fx.gid); err != nil {
			t.Fatal(err)
		}
		if _, err := fx.svc.Execute(ctx, fx.owner, fx.gid); !errors.Is(err, ErrAlreadyExecuted) {
			t.Errorf("err = %v, want ErrAlreadyExecuted", err)
		}
	})
	t.Run("kocokan manual tidak bisa di-execute", func(t *testing.T) {
		fx := newFixture(t, 2, "manual", nil)
		order := []string{"member-1", "member-0"}
		if _, err := fx.svc.Manual(ctx, fx.owner, fx.gid, order); err != nil {
			t.Fatal(err)
		}
		if _, err := fx.svc.Execute(ctx, fx.owner, fx.gid); !errors.Is(err, ErrAlreadyExecuted) {
			t.Errorf("err = %v, want ErrAlreadyExecuted", err)
		}
	})
}

// ---- Manual ----------------------------------------------------------------

func TestManual(t *testing.T) {
	ctx := context.Background()
	fx := newFixture(t, 3, "manual", nil)

	d, err := fx.svc.Manual(ctx, fx.owner, fx.gid, []string{"member-2", "member-0", "member-1"})
	if err != nil {
		t.Fatalf("Manual error: %v", err)
	}
	if d.Algorithm != "manual" || d.ExecutedAt == nil || d.SeedCommit != "" || d.SeedReveal != "" {
		t.Errorf("draw manual tidak sesuai: %+v", d)
	}
	for i, id := range []string{"member-2", "member-0", "member-1"} {
		if d.Entries[i].MemberID != id || *d.Entries[i].Order != i+1 {
			t.Errorf("posisi %d: %s order %v, want %s order %d", i, d.Entries[i].MemberID, d.Entries[i].Order, id, i+1)
		}
	}
}

func TestManualRejections(t *testing.T) {
	ctx := context.Background()
	valid := []string{"member-0", "member-1", "member-2"}

	invalid := map[string][]string{
		"kosong":         {},
		"kurang satu":    {"member-0", "member-1"},
		"kelebihan":      {"member-0", "member-1", "member-2", "member-3"},
		"duplikat":       {"member-0", "member-0", "member-1"},
		"id tak dikenal": {"member-0", "member-1", "hantu"},
	}
	for name, order := range invalid {
		t.Run(name, func(t *testing.T) {
			fx := newFixture(t, 3, "manual", nil)
			_, err := fx.svc.Manual(ctx, fx.owner, fx.gid, order)
			var verr group.ValidationError
			if !errors.As(err, &verr) {
				t.Errorf("err = %v, want ValidationError", err)
			}
			if len(fx.store.draws) != 0 {
				t.Error("urutan tidak valid tidak boleh tersimpan")
			}
		})
	}

	t.Run("metode bukan manual", func(t *testing.T) {
		fx := newFixture(t, 3, "random", nil)
		if _, err := fx.svc.Manual(ctx, fx.owner, fx.gid, valid); !errors.Is(err, ErrWrongMethod) {
			t.Errorf("err = %v, want ErrWrongMethod", err)
		}
	})
	t.Run("bukan pemilik", func(t *testing.T) {
		fx := newFixture(t, 3, "manual", nil)
		if _, err := fx.svc.Manual(ctx, "user-1", fx.gid, valid); !errors.Is(err, group.ErrForbidden) {
			t.Errorf("err = %v, want ErrForbidden", err)
		}
	})
	t.Run("slot belum penuh", func(t *testing.T) {
		fx := newFixture(t, 3, "manual", nil)
		fx.groups.groups[fx.gid].SlotCount = 4
		if _, err := fx.svc.Manual(ctx, fx.owner, fx.gid, valid); !errors.Is(err, ErrGroupNotFull) {
			t.Errorf("err = %v, want ErrGroupNotFull", err)
		}
	})
}

// ---- Get -------------------------------------------------------------------

func TestGetAccess(t *testing.T) {
	ctx := context.Background()
	fx := newFixture(t, 3, "random", nil)
	fx.groups.members[fx.gid] = append(fx.groups.members[fx.gid],
		group.Member{ID: "member-p", GroupID: fx.gid, UserID: "pending-user", Status: "pending"},
		group.Member{ID: "member-r", GroupID: fx.gid, UserID: "removed-user", Status: "removed"},
	)

	if _, err := fx.svc.Get(ctx, "user-1", fx.gid); !errors.Is(err, ErrNoDraw) {
		t.Errorf("sebelum commit: err = %v, want ErrNoDraw", err)
	}
	if _, err := fx.svc.Commit(ctx, fx.owner, fx.gid); err != nil {
		t.Fatal(err)
	}

	if d, err := fx.svc.Get(ctx, "user-1", fx.gid); err != nil || d.SeedReveal != "" {
		t.Errorf("anggota aktif: %+v, %v (seed tidak boleh terbuka sebelum execute)", d, err)
	}
	if _, err := fx.svc.Get(ctx, "pending-user", fx.gid); !errors.Is(err, group.ErrForbidden) {
		t.Errorf("pending: err = %v, want ErrForbidden", err)
	}
	if _, err := fx.svc.Get(ctx, "removed-user", fx.gid); !errors.Is(err, group.ErrNotFound) {
		t.Errorf("removed: err = %v, want ErrNotFound", err)
	}
	if _, err := fx.svc.Get(ctx, "stranger", fx.gid); !errors.Is(err, group.ErrNotFound) {
		t.Errorf("orang luar: err = %v, want ErrNotFound", err)
	}
}

// Seed tidak boleh terlihat lewat Get sebelum dijalankan, dan harus terlihat sesudahnya.
func TestSeedRevealedOnlyAfterExecute(t *testing.T) {
	ctx := context.Background()
	fx := newFixture(t, 3, "random", nil)
	fx.svc.Commit(ctx, fx.owner, fx.gid)

	before, _ := fx.svc.Get(ctx, fx.owner, fx.gid)
	if before.SeedReveal != "" {
		t.Error("seed terbuka sebelum execute")
	}
	if _, err := fx.svc.Execute(ctx, fx.owner, fx.gid); err != nil {
		t.Fatal(err)
	}
	after, _ := fx.svc.Get(ctx, "user-2", fx.gid)
	if after.SeedReveal == "" || !VerifyCommit(after.SeedReveal, after.SeedCommit) {
		t.Error("setelah execute, seed harus terbuka dan cocok dengan komitmen")
	}
}
