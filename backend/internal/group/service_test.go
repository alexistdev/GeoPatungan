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
	"strings"
	"testing"
	"time"
)

// fakeStore adalah Store di memori; perilaku pentingnya (slot, status pending)
// meniru Repository asli yang diuji terpisah di tes integrasi.
type fakeStore struct {
	groups      map[string]*Group
	members     []*Member
	seq         int
	dupInvites  int // berapa kali Create pertama dibuat gagal ErrDuplicate
	createCalls int
}

func newFakeStore() *fakeStore { return &fakeStore{groups: map[string]*Group{}} }

func (f *fakeStore) nextID() string {
	f.seq++
	return fmt.Sprintf("00000000-0000-0000-0000-%012d", f.seq)
}

func (f *fakeStore) Create(_ context.Context, g *Group) error {
	f.createCalls++
	if f.dupInvites > 0 {
		f.dupInvites--
		return ErrDuplicate
	}
	g.ID = f.nextID()
	g.Status = "open"
	g.CreatedAt = time.Now()
	g.ActiveMembers = 1
	cp := *g
	f.groups[g.ID] = &cp
	f.members = append(f.members, &Member{ID: f.nextID(), GroupID: g.ID, UserID: g.OwnerID, Status: "active"})
	return nil
}

func (f *fakeStore) FindByID(_ context.Context, id string) (*Group, error) {
	g, ok := f.groups[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *g
	return &cp, nil
}

func (f *fakeStore) FindByInviteCode(_ context.Context, code string) (*Group, error) {
	for _, g := range f.groups {
		if g.InviteCode == code {
			cp := *g
			return &cp, nil
		}
	}
	return nil, ErrNotFound
}

func (f *fakeStore) ListByUser(_ context.Context, userID string) ([]Group, error) {
	out := []Group{}
	for _, m := range f.members {
		if m.UserID == userID && m.Status != "removed" {
			g := *f.groups[m.GroupID]
			g.MyStatus = m.Status
			out = append(out, g)
		}
	}
	return out, nil
}

func (f *fakeStore) AddMember(_ context.Context, groupID, userID string) (*Member, error) {
	for _, m := range f.members {
		if m.GroupID == groupID && m.UserID == userID {
			return nil, ErrAlreadyMember
		}
	}
	m := &Member{ID: f.nextID(), GroupID: groupID, UserID: userID, Status: "pending"}
	f.members = append(f.members, m)
	cp := *m
	return &cp, nil
}

func (f *fakeStore) FindMember(_ context.Context, groupID, userID string) (*Member, error) {
	for _, m := range f.members {
		if m.GroupID == groupID && m.UserID == userID {
			cp := *m
			return &cp, nil
		}
	}
	return nil, ErrNotFound
}

func (f *fakeStore) ListMembers(_ context.Context, groupID string) ([]Member, error) {
	out := []Member{}
	for _, m := range f.members {
		if m.GroupID == groupID && m.Status != "removed" {
			out = append(out, *m)
		}
	}
	return out, nil
}

func (f *fakeStore) setStatus(groupID, memberID, from, to string) error {
	for _, m := range f.members {
		if m.ID == memberID && m.GroupID == groupID && m.Status == from {
			m.Status = to
			return nil
		}
	}
	return ErrNotFound
}

func (f *fakeStore) ApproveMember(_ context.Context, groupID, memberID string) error {
	return f.setStatus(groupID, memberID, "pending", "active")
}

func (f *fakeStore) RejectMember(_ context.Context, groupID, memberID string) error {
	return f.setStatus(groupID, memberID, "pending", "removed")
}

type fixedLevel int

func (l fixedLevel) Rank(context.Context, string) (int, error) { return int(l), nil }

func newTestService(store Store, level int) *Service {
	s := NewService(store, fixedLevel(level))
	s.now = func() time.Time { return time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC) }
	return s
}

func validInput() CreateInput {
	return CreateInput{
		Name:               "Arisan Kantor",
		ContributionAmount: 500_000,
		PeriodType:         "monthly",
		SlotCount:          10,
		StartDate:          "2026-11-01",
		LateFee:            10_000,
	}
}

func TestCreate(t *testing.T) {
	store := newFakeStore()
	svc := newTestService(store, 1)

	g, err := svc.Create(context.Background(), "owner-1", validInput())
	if err != nil {
		t.Fatalf("Create error: %v", err)
	}
	if g.ID == "" || g.OwnerID != "owner-1" || g.Status != "open" {
		t.Errorf("hasil tidak sesuai: %+v", g)
	}
	if len(g.InviteCode) != 8 {
		t.Errorf("panjang invite code = %d, want 8", len(g.InviteCode))
	}
	if g.DrawMethod != "random" || g.MinLevelRank != 1 {
		t.Errorf("default salah: draw=%q level=%d", g.DrawMethod, g.MinLevelRank)
	}
	if g.ActiveMembers != 1 {
		t.Errorf("pemilik seharusnya langsung jadi anggota aktif, got %d", g.ActiveMembers)
	}
}

func TestCreateValidation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*CreateInput)
	}{
		{"nama terlalu pendek", func(in *CreateInput) { in.Name = "ab" }},
		{"nama hanya spasi", func(in *CreateInput) { in.Name = "     " }},
		{"nama terlalu panjang", func(in *CreateInput) { in.Name = strings.Repeat("a", 101) }},
		{"iuran terlalu kecil", func(in *CreateInput) { in.ContributionAmount = 9_999 }},
		{"iuran terlalu besar", func(in *CreateInput) { in.ContributionAmount = 100_000_001 }},
		{"periode salah", func(in *CreateInput) { in.PeriodType = "daily" }},
		{"slot 1", func(in *CreateInput) { in.SlotCount = 1 }},
		{"slot 51", func(in *CreateInput) { in.SlotCount = 51 }},
		{"tanggal bukan format", func(in *CreateInput) { in.StartDate = "01-11-2026" }},
		{"tanggal tidak ada", func(in *CreateInput) { in.StartDate = "2026-02-30" }},
		{"tanggal masa lalu", func(in *CreateInput) { in.StartDate = "2026-10-10" }},
		{"denda negatif", func(in *CreateInput) { in.LateFee = -1 }},
		{"denda melebihi iuran", func(in *CreateInput) { in.LateFee = 500_001 }},
		{"draw method salah", func(in *CreateInput) { in.DrawMethod = "lotre" }},
		{"level 4", func(in *CreateInput) { in.MinLevelRank = 4 }},
		{"level negatif", func(in *CreateInput) { in.MinLevelRank = -1 }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := newFakeStore()
			in := validInput()
			tc.mutate(&in)

			_, err := newTestService(store, 1).Create(context.Background(), "owner-1", in)

			var verr ValidationError
			if !errors.As(err, &verr) {
				t.Fatalf("err = %v, want ValidationError", err)
			}
			if store.createCalls != 0 {
				t.Error("input tidak valid seharusnya tidak sampai ke database")
			}
		})
	}
}

func TestCreateAcceptsBoundaryValues(t *testing.T) {
	cases := map[string]func(*CreateInput){
		"tanggal hari ini":   func(in *CreateInput) { in.StartDate = "2026-10-11" },
		"iuran minimum":      func(in *CreateInput) { in.ContributionAmount = 10_000; in.LateFee = 0 },
		"iuran maksimum":     func(in *CreateInput) { in.ContributionAmount = 100_000_000 },
		"slot minimum":       func(in *CreateInput) { in.SlotCount = 2 },
		"slot maksimum":      func(in *CreateInput) { in.SlotCount = 50 },
		"denda = iuran":      func(in *CreateInput) { in.LateFee = in.ContributionAmount },
		"draw weighted":      func(in *CreateInput) { in.DrawMethod = "weighted" },
		"level minimal 3":    func(in *CreateInput) { in.MinLevelRank = 3 },
		"periode mingguan":   func(in *CreateInput) { in.PeriodType = "weekly" },
		"nama 3 karakter":    func(in *CreateInput) { in.Name = "abc" },
		"nama 100 karakter":  func(in *CreateInput) { in.Name = strings.Repeat("a", 100) },
		"nama dengan spasi ": func(in *CreateInput) { in.Name = "  Arisan RT  " },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			in := validInput()
			mutate(&in)
			if _, err := newTestService(newFakeStore(), 1).Create(context.Background(), "o", in); err != nil {
				t.Errorf("seharusnya valid: %v", err)
			}
		})
	}
}

func TestCreateRetriesOnInviteCodeCollision(t *testing.T) {
	store := newFakeStore()
	store.dupInvites = 2

	g, err := newTestService(store, 1).Create(context.Background(), "owner-1", validInput())
	if err != nil {
		t.Fatalf("seharusnya berhasil setelah retry: %v", err)
	}
	if store.createCalls != 3 {
		t.Errorf("Create dipanggil %d kali, want 3", store.createCalls)
	}
	if g.ID == "" {
		t.Error("grup seharusnya tersimpan")
	}

	store = newFakeStore()
	store.dupInvites = 99
	if _, err := newTestService(store, 1).Create(context.Background(), "o", validInput()); !errors.Is(err, ErrDuplicate) {
		t.Errorf("err = %v, want ErrDuplicate setelah 5 percobaan", err)
	}
	if store.createCalls != 5 {
		t.Errorf("Create dipanggil %d kali, want 5 (batas retry)", store.createCalls)
	}
}

func TestJoin(t *testing.T) {
	ctx := context.Background()
	store := newFakeStore()
	svc := newTestService(store, 1)
	g, err := svc.Create(ctx, "owner-1", validInput())
	if err != nil {
		t.Fatal(err)
	}

	t.Run("berhasil jadi pending, kode undangan tidak bocor", func(t *testing.T) {
		got, err := svc.Join(ctx, "user-2", g.InviteCode)
		if err != nil {
			t.Fatalf("Join error: %v", err)
		}
		if got.MyStatus != "pending" {
			t.Errorf("MyStatus = %q, want pending", got.MyStatus)
		}
		if got.InviteCode != "" {
			t.Error("invite code tidak boleh dikembalikan ke peserta")
		}
	})

	t.Run("kode huruf kecil dan berspasi tetap diterima", func(t *testing.T) {
		in := "  " + strings.ToLower(g.InviteCode) + " "
		if _, err := svc.Join(ctx, "user-3", in); err != nil {
			t.Errorf("seharusnya diterima: %v", err)
		}
	})

	t.Run("kode salah", func(t *testing.T) {
		if _, err := svc.Join(ctx, "user-4", "SALAH123"); !errors.Is(err, ErrNotFound) {
			t.Errorf("err = %v, want ErrNotFound", err)
		}
	})

	t.Run("sudah terdaftar (pending)", func(t *testing.T) {
		if _, err := svc.Join(ctx, "user-2", g.InviteCode); !errors.Is(err, ErrAlreadyMember) {
			t.Errorf("err = %v, want ErrAlreadyMember", err)
		}
	})

	t.Run("pemilik tidak bisa join grupnya sendiri", func(t *testing.T) {
		if _, err := svc.Join(ctx, "owner-1", g.InviteCode); !errors.Is(err, ErrAlreadyMember) {
			t.Errorf("err = %v, want ErrAlreadyMember", err)
		}
	})

	t.Run("slot sudah penuh", func(t *testing.T) {
		store.groups[g.ID].ActiveMembers = g.SlotCount
		defer func() { store.groups[g.ID].ActiveMembers = 1 }()
		if _, err := svc.Join(ctx, "user-6", g.InviteCode); !errors.Is(err, ErrGroupFull) {
			t.Errorf("err = %v, want ErrGroupFull", err)
		}
	})

	t.Run("grup tidak lagi open", func(t *testing.T) {
		store.groups[g.ID].Status = "running"
		defer func() { store.groups[g.ID].Status = "open" }()
		if _, err := svc.Join(ctx, "user-5", g.InviteCode); !errors.Is(err, ErrGroupNotOpen) {
			t.Errorf("err = %v, want ErrGroupNotOpen", err)
		}
	})
}

func TestJoinRequiresMinimumLevel(t *testing.T) {
	ctx := context.Background()
	store := newFakeStore()

	in := validInput()
	in.MinLevelRank = 2
	g, err := newTestService(store, 1).Create(ctx, "owner-1", in)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := newTestService(store, 1).Join(ctx, "u-low", g.InviteCode); !errors.Is(err, ErrLevelTooLow) {
		t.Errorf("level 1: err = %v, want ErrLevelTooLow", err)
	}
	if _, err := newTestService(store, 2).Join(ctx, "u-ok", g.InviteCode); err != nil {
		t.Errorf("level 2 seharusnya boleh: %v", err)
	}
	if _, err := newTestService(store, 3).Join(ctx, "u-high", g.InviteCode); err != nil {
		t.Errorf("level 3 seharusnya boleh: %v", err)
	}
}

func TestGetVisibility(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(newFakeStore(), 1)
	g, _ := svc.Create(ctx, "owner-1", validInput())
	if _, err := svc.Join(ctx, "member-2", g.InviteCode); err != nil {
		t.Fatal(err)
	}

	owner, err := svc.Get(ctx, "owner-1", g.ID)
	if err != nil || owner.InviteCode == "" || owner.MyStatus != "active" {
		t.Errorf("pemilik: %+v, %v", owner, err)
	}

	member, err := svc.Get(ctx, "member-2", g.ID)
	if err != nil || member.InviteCode != "" || member.MyStatus != "pending" {
		t.Errorf("anggota: %+v, %v", member, err)
	}

	if _, err := svc.Get(ctx, "stranger", g.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("orang luar: err = %v, want ErrNotFound (jangan bocorkan keberadaan grup)", err)
	}
}

func TestListMineHidesInviteCodeOfOthersGroups(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(newFakeStore(), 1)
	mine, _ := svc.Create(ctx, "u-1", validInput())
	other, _ := svc.Create(ctx, "u-2", validInput())
	if _, err := svc.Join(ctx, "u-1", other.InviteCode); err != nil {
		t.Fatal(err)
	}

	groups, err := svc.ListMine(ctx, "u-1")
	if err != nil || len(groups) != 2 {
		t.Fatalf("len = %d, err = %v, want 2", len(groups), err)
	}
	for _, g := range groups {
		isOwner := g.ID == mine.ID
		if isOwner && g.InviteCode == "" {
			t.Error("pemilik seharusnya melihat invite code grupnya")
		}
		if !isOwner && g.InviteCode != "" {
			t.Error("invite code grup orang lain tidak boleh terlihat")
		}
	}
}

func TestMembersAccess(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(newFakeStore(), 1)
	g, _ := svc.Create(ctx, "owner-1", validInput())
	if _, err := svc.Join(ctx, "pending-2", g.InviteCode); err != nil {
		t.Fatal(err)
	}

	if ms, err := svc.Members(ctx, "owner-1", g.ID); err != nil || len(ms) != 2 {
		t.Errorf("pemilik: len=%d err=%v, want 2 anggota", len(ms), err)
	}
	if _, err := svc.Members(ctx, "pending-2", g.ID); !errors.Is(err, ErrForbidden) {
		t.Errorf("anggota pending: err = %v, want ErrForbidden", err)
	}
	if _, err := svc.Members(ctx, "stranger", g.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("orang luar: err = %v, want ErrNotFound", err)
	}
}

func TestApproveAndReject(t *testing.T) {
	ctx := context.Background()
	store := newFakeStore()
	svc := newTestService(store, 1)
	g, _ := svc.Create(ctx, "owner-1", validInput())
	svc.Join(ctx, "u-2", g.InviteCode)
	svc.Join(ctx, "u-3", g.InviteCode)
	m2, _ := store.FindMember(ctx, g.ID, "u-2")
	m3, _ := store.FindMember(ctx, g.ID, "u-3")

	if err := svc.Approve(ctx, "u-2", g.ID, m3.ID); !errors.Is(err, ErrForbidden) {
		t.Errorf("non-pemilik approve: err = %v, want ErrForbidden", err)
	}
	if err := svc.Reject(ctx, "u-2", g.ID, m3.ID); !errors.Is(err, ErrForbidden) {
		t.Errorf("non-pemilik reject: err = %v, want ErrForbidden", err)
	}
	if err := svc.Approve(ctx, "stranger", g.ID, m3.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("orang luar approve: err = %v, want ErrNotFound", err)
	}

	if err := svc.Approve(ctx, "owner-1", g.ID, m2.ID); err != nil {
		t.Fatalf("approve oleh pemilik: %v", err)
	}
	if got, _ := store.FindMember(ctx, g.ID, "u-2"); got.Status != "active" {
		t.Errorf("status u-2 = %q, want active", got.Status)
	}

	if err := svc.Reject(ctx, "owner-1", g.ID, m3.ID); err != nil {
		t.Fatalf("reject oleh pemilik: %v", err)
	}
	if got, _ := store.FindMember(ctx, g.ID, "u-3"); got.Status != "removed" {
		t.Errorf("status u-3 = %q, want removed", got.Status)
	}

	// sudah diproses: tidak bisa diproses lagi
	if err := svc.Approve(ctx, "owner-1", g.ID, m2.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("approve ulang: err = %v, want ErrNotFound", err)
	}

	// anggota yang ditolak tidak lagi melihat grup
	if _, err := svc.Get(ctx, "u-3", g.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("anggota ditolak: err = %v, want ErrNotFound", err)
	}
}

func TestNewInviteCode(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 1000; i++ {
		code, err := newInviteCode()
		if err != nil || len(code) != 8 {
			t.Fatalf("code=%q err=%v", code, err)
		}
		for _, c := range code {
			if !strings.ContainsRune(inviteAlphabet, c) {
				t.Fatalf("karakter %q di luar alfabet", c)
			}
		}
		if seen[code] {
			t.Fatalf("kode %q duplikat", code)
		}
		seen[code] = true
	}
}
