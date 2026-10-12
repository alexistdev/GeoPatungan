/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package group

import (
	"context"
	"crypto/rand"
	"errors"
	"math/big"
	"strings"
	"time"
)

// Store adalah akses data yang dibutuhkan Service (diimplementasikan *Repository).
type Store interface {
	Create(ctx context.Context, g *Group) error
	FindByID(ctx context.Context, id string) (*Group, error)
	FindByInviteCode(ctx context.Context, code string) (*Group, error)
	ListByUser(ctx context.Context, userID string) ([]Group, error)
	AddMember(ctx context.Context, groupID, userID string) (*Member, error)
	FindMember(ctx context.Context, groupID, userID string) (*Member, error)
	ListMembers(ctx context.Context, groupID string) ([]Member, error)
	ApproveMember(ctx context.Context, groupID, memberID string) error
	RejectMember(ctx context.Context, groupID, memberID string) error
}

// Policy adalah hak dan batas seorang pengguna menurut level reputasinya.
type Policy struct {
	Rank           int     // level reputasi 1..3
	DrawWeight     float64 // bobot kocokan berbobot (1..3)
	CanCreateGroup bool    // boleh menjadi bandar (membuat grup)
	MaxGroupValue  int64   // iuran maksimal grup yang boleh dibuat/diikuti; 0 = tanpa batas
}

// LevelProvider memberi kebijakan seorang pengguna. Implementasi sebenarnya ada di paket
// reputation (dihitung dari data nyata); group tidak mengimpornya supaya tidak melingkar.
type LevelProvider interface {
	Policy(ctx context.Context, userID string) (Policy, error)
}

// BaseLevel menganggap semua pengguna level 1 tanpa batas. Dipakai di tes dan sebagai
// pengganti jika modul reputasi tidak dipasang.
type BaseLevel struct{}

func (BaseLevel) Policy(context.Context, string) (Policy, error) {
	return Policy{Rank: 1, DrawWeight: 1, CanCreateGroup: true}, nil
}

type Service struct {
	store  Store
	levels LevelProvider
	now    func() time.Time
}

func NewService(store Store, levels LevelProvider) *Service {
	return &Service{store: store, levels: levels, now: time.Now}
}

type CreateInput struct {
	Name               string
	ContributionAmount int64
	PeriodType         string
	SlotCount          int
	StartDate          string // YYYY-MM-DD
	LateFee            int64
	DrawMethod         string // kosong = random
	MinLevelRank       int    // 0 = 1
}

func (s *Service) Create(ctx context.Context, ownerID string, in CreateInput) (*Group, error) {
	g, err := s.validate(ownerID, in)
	if err != nil {
		return nil, err
	}
	pol, err := s.levels.Policy(ctx, ownerID)
	if err != nil {
		return nil, err
	}
	if !pol.CanCreateGroup {
		return nil, ErrCannotCreateGroup
	}
	if pol.MaxGroupValue > 0 && g.ContributionAmount > pol.MaxGroupValue {
		return nil, ErrGroupTooExpensive
	}

	// Kode undangan acak bisa (sangat jarang) bentrok; coba ulang beberapa kali.
	for attempt := 0; attempt < 5; attempt++ {
		if g.InviteCode, err = newInviteCode(); err != nil {
			return nil, err
		}
		if err = s.store.Create(ctx, g); !errors.Is(err, ErrDuplicate) {
			break
		}
	}
	if err != nil {
		return nil, err
	}
	return g, nil
}

func (s *Service) validate(ownerID string, in CreateInput) (*Group, error) {
	name := strings.TrimSpace(in.Name)
	if len(name) < 3 || len(name) > 100 {
		return nil, ValidationError("nama grup 3 sampai 100 karakter")
	}
	if in.ContributionAmount < 10_000 || in.ContributionAmount > 100_000_000 {
		return nil, ValidationError("iuran harus antara Rp10.000 dan Rp100.000.000")
	}
	if in.PeriodType != "weekly" && in.PeriodType != "monthly" {
		return nil, ValidationError("periode harus weekly atau monthly")
	}
	if in.SlotCount < 2 || in.SlotCount > 50 {
		return nil, ValidationError("jumlah slot 2 sampai 50")
	}
	start, err := time.Parse("2006-01-02", in.StartDate)
	if err != nil {
		return nil, ValidationError("start_date harus berformat YYYY-MM-DD")
	}
	if in.StartDate < s.now().Format("2006-01-02") {
		return nil, ValidationError("start_date tidak boleh di masa lalu")
	}
	if in.LateFee < 0 || in.LateFee > in.ContributionAmount {
		return nil, ValidationError("denda keterlambatan harus 0 sampai sebesar iuran")
	}
	draw := in.DrawMethod
	if draw == "" {
		draw = "random"
	}
	if draw != "random" && draw != "weighted" && draw != "manual" {
		return nil, ValidationError("draw_method harus random, weighted, atau manual")
	}
	level := in.MinLevelRank
	if level == 0 {
		level = 1
	}
	if level < 1 || level > 3 {
		return nil, ValidationError("min_level_rank harus 1 sampai 3")
	}

	return &Group{
		OwnerID:            ownerID,
		Name:               name,
		ContributionAmount: in.ContributionAmount,
		PeriodType:         in.PeriodType,
		SlotCount:          in.SlotCount,
		StartDate:          start.Format("2006-01-02"),
		LateFee:            in.LateFee,
		DrawMethod:         draw,
		MinLevelRank:       level,
	}, nil
}

// Join membuat permintaan bergabung lewat kode undangan. Pemilik grup yang
// menyetujuinya (Approve).
func (s *Service) Join(ctx context.Context, userID, inviteCode string) (*Group, error) {
	g, err := s.store.FindByInviteCode(ctx, strings.ToUpper(strings.TrimSpace(inviteCode)))
	if err != nil {
		return nil, err
	}
	if g.Status != "open" {
		return nil, ErrGroupNotOpen
	}
	if g.ActiveMembers >= g.SlotCount {
		return nil, ErrGroupFull
	}

	pol, err := s.levels.Policy(ctx, userID)
	if err != nil {
		return nil, err
	}
	if pol.Rank < g.MinLevelRank {
		return nil, ErrLevelTooLow
	}
	if pol.MaxGroupValue > 0 && g.ContributionAmount > pol.MaxGroupValue {
		return nil, ErrGroupTooExpensive
	}

	m, err := s.store.AddMember(ctx, g.ID, userID)
	if err != nil {
		return nil, err
	}
	g.MyStatus = m.Status
	g.InviteCode = "" // kode undangan hanya untuk pemilik
	return g, nil
}

func (s *Service) ListMine(ctx context.Context, userID string) ([]Group, error) {
	groups, err := s.store.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	for i := range groups {
		if groups[i].OwnerID != userID {
			groups[i].InviteCode = ""
		}
	}
	return groups, nil
}

// Get hanya untuk pemilik/anggota. Orang luar mendapat ErrNotFound agar
// keberadaan grup tidak bocor.
func (s *Service) Get(ctx context.Context, userID, groupID string) (*Group, error) {
	g, m, err := s.load(ctx, userID, groupID)
	if err != nil {
		return nil, err
	}
	g.MyStatus = m.Status
	if g.OwnerID != userID {
		g.InviteCode = ""
	}
	return g, nil
}

// Members hanya untuk pemilik dan anggota aktif.
func (s *Service) Members(ctx context.Context, userID, groupID string) ([]Member, error) {
	_, m, err := s.load(ctx, userID, groupID)
	if err != nil {
		return nil, err
	}
	if m.Status != "active" {
		return nil, ErrForbidden
	}
	return s.store.ListMembers(ctx, groupID)
}

func (s *Service) Approve(ctx context.Context, userID, groupID, memberID string) error {
	if err := s.requireOwner(ctx, userID, groupID); err != nil {
		return err
	}
	return s.store.ApproveMember(ctx, groupID, memberID)
}

func (s *Service) Reject(ctx context.Context, userID, groupID, memberID string) error {
	if err := s.requireOwner(ctx, userID, groupID); err != nil {
		return err
	}
	return s.store.RejectMember(ctx, groupID, memberID)
}

// load mengambil grup beserta keanggotaan pemanggil (selain removed).
func (s *Service) load(ctx context.Context, userID, groupID string) (*Group, *Member, error) {
	g, err := s.store.FindByID(ctx, groupID)
	if err != nil {
		return nil, nil, err
	}
	m, err := s.store.FindMember(ctx, groupID, userID)
	if err != nil {
		return nil, nil, err
	}
	if m.Status == "removed" {
		return nil, nil, ErrNotFound
	}
	return g, m, nil
}

func (s *Service) requireOwner(ctx context.Context, userID, groupID string) error {
	g, _, err := s.load(ctx, userID, groupID)
	if err != nil {
		return err
	}
	if g.OwnerID != userID {
		return ErrForbidden
	}
	return nil
}

// Tanpa huruf/angka yang mudah tertukar (0/O, 1/I).
const inviteAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

func newInviteCode() (string, error) {
	b := make([]byte, 8)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(inviteAlphabet))))
		if err != nil {
			return "", err
		}
		b[i] = inviteAlphabet[n.Int64()]
	}
	return string(b), nil
}
