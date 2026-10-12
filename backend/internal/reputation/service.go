/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package reputation

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"strings"

	"github.com/alexistdev/geopatungan/internal/group"
	"github.com/alexistdev/geopatungan/internal/httpx"
)

// Store adalah akses data reputasi (diimplementasikan *Repository).
type Store interface {
	Levels(ctx context.Context) ([]Level, error)
	Stats(ctx context.Context, userID string) (Stats, error)
	UpdateLevels(ctx context.Context, adminID string, levels []Level) error
}

type Service struct {
	store Store
}

func NewService(store Store) *Service {
	return &Service{store: store}
}

// Profile mengembalikan reputasi seorang pengguna beserta kemajuan menuju level berikutnya.
func (s *Service) Profile(ctx context.Context, userID string) (*Profile, error) {
	levels, err := s.store.Levels(ctx)
	if err != nil {
		return nil, err
	}
	st, err := s.store.Stats(ctx, userID)
	if err != nil {
		return nil, err
	}
	return BuildProfile(levels, st), nil
}

// Policy memenuhi group.LevelProvider: hak dan batas seorang pengguna menurut levelnya saat ini.
func (s *Service) Policy(ctx context.Context, userID string) (group.Policy, error) {
	p, err := s.Profile(ctx, userID)
	if err != nil {
		return group.Policy{}, err
	}
	pol := group.Policy{Rank: p.Level.Rank, DrawWeight: p.Level.DrawWeight, CanCreateGroup: p.Level.CanCreateGroup}
	if p.Level.MaxGroupValue != nil {
		pol.MaxGroupValue = *p.Level.MaxGroupValue
	}
	return pol, nil
}

func (s *Service) Levels(ctx context.Context) ([]Level, error) {
	return s.store.Levels(ctx)
}

var codeRe = regexp.MustCompile(`^[a-z_]{2,20}$`)

// ValidateLevels memastikan definisi level masuk akal dan KONSISTEN: level yang lebih tinggi
// tidak boleh lebih mudah dicapai atau lebih terbatas haknya daripada level di bawahnya.
// Tanpa itu, naik level bisa justru mengurangi hak, atau level 3 tercapai tanpa level 2.
func ValidateLevels(levels []Level) error {
	bad := func(format string, a ...any) error { return httpx.ValidationError(fmt.Sprintf(format, a...)) }

	if len(levels) != 3 {
		return bad("harus tepat 3 level (rank 1, 2, 3)")
	}
	codes := map[string]bool{}
	for i, l := range levels {
		if l.Rank != i+1 {
			return bad("level harus berurutan rank 1, 2, 3")
		}
		name := strings.TrimSpace(l.Name)
		if len(name) < 2 || len(name) > 30 {
			return bad("level %d: nama 2 sampai 30 karakter", l.Rank)
		}
		if !codeRe.MatchString(l.Code) || codes[l.Code] {
			return bad("level %d: kode harus huruf kecil/garis bawah 2-20 karakter dan unik", l.Rank)
		}
		codes[l.Code] = true
		if l.MinCompletedCycles < 0 || l.MinCompletedCycles > 1000 {
			return bad("level %d: min_completed_cycles harus 0 sampai 1000", l.Rank)
		}
		if l.MaxLateCount != nil && (*l.MaxLateCount < 0 || *l.MaxLateCount > 1000) {
			return bad("level %d: max_late_count harus 0 sampai 1000 atau kosong", l.Rank)
		}
		if math.IsNaN(l.DrawWeight) || l.DrawWeight < 1 || l.DrawWeight > 3 {
			return bad("level %d: draw_weight harus 1 sampai 3 (rasio maksimal 3:1)", l.Rank)
		}
		if l.MaxGroupValue != nil && (*l.MaxGroupValue < 10_000 || *l.MaxGroupValue > 100_000_000) {
			return bad("level %d: max_group_value harus Rp10.000 sampai Rp100.000.000 atau kosong", l.Rank)
		}
	}

	base := levels[0]
	if base.RequireVerified || base.MinCompletedCycles != 0 || base.MaxLateCount != nil {
		return bad("level 1 adalah dasar dan tidak boleh punya syarat")
	}
	for i := 1; i < 3; i++ {
		lo, hi := levels[i-1], levels[i]
		switch {
		case lo.RequireVerified && !hi.RequireVerified:
			return bad("level %d tidak boleh lebih mudah dari level %d (verifikasi)", hi.Rank, lo.Rank)
		case hi.MinCompletedCycles < lo.MinCompletedCycles:
			return bad("level %d tidak boleh lebih mudah dari level %d (arisan tuntas)", hi.Rank, lo.Rank)
		case lo.MaxLateCount != nil && (hi.MaxLateCount == nil || *hi.MaxLateCount > *lo.MaxLateCount):
			return bad("level %d tidak boleh lebih longgar dari level %d (batas keterlambatan)", hi.Rank, lo.Rank)
		case hi.DrawWeight < lo.DrawWeight:
			return bad("level %d tidak boleh berbobot lebih kecil dari level %d", hi.Rank, lo.Rank)
		case lo.CanCreateGroup && !hi.CanCreateGroup:
			return bad("level %d tidak boleh kehilangan hak membuat grup yang dimiliki level %d", hi.Rank, lo.Rank)
		case lo.MaxGroupValue == nil && hi.MaxGroupValue != nil,
			lo.MaxGroupValue != nil && hi.MaxGroupValue != nil && *hi.MaxGroupValue < *lo.MaxGroupValue:
			return bad("level %d tidak boleh punya batas nilai grup lebih kecil dari level %d", hi.Rank, lo.Rank)
		}
		// level yang lebih tinggi harus lebih sulit atau sama sulit, tetapi tidak identik di semua syarat
		// jika haknya juga identik: itu level mubazir. Dibiarkan; admin yang memutuskan.
	}
	return nil
}

// UpdateLevels memvalidasi lalu menyimpan definisi level. Perubahan berlaku seketika untuk semua
// pengguna karena level dihitung ulang saat dibutuhkan.
func (s *Service) UpdateLevels(ctx context.Context, adminID string, levels []Level) ([]Level, error) {
	for i := range levels {
		levels[i].Name = strings.TrimSpace(levels[i].Name)
	}
	if err := ValidateLevels(levels); err != nil {
		return nil, err
	}
	if err := s.store.UpdateLevels(ctx, adminID, levels); err != nil {
		return nil, err
	}
	return s.store.Levels(ctx)
}
