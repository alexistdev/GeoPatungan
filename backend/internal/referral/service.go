/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package referral

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/alexistdev/geopatungan/internal/httpx"
)

const (
	DefaultListLimit = 20
	MaxListLimit     = 100
)

// rawEntry adalah baris riwayat dari database, sebelum nama sumber disamarkan.
type rawEntry struct {
	ID         int64
	Level      int
	ShareBps   int
	Amount     int64
	SourceName string
	CreatedAt  time.Time
}

// Store adalah akses data referral (diimplementasikan *Repository).
type Store interface {
	Rules(ctx context.Context) ([]Rule, error)
	UpdateRules(ctx context.Context, adminID string, rules []Rule) error
	Summary(ctx context.Context, userID string) (*Summary, error)
	Commissions(ctx context.Context, userID string, limit int, before int64) ([]rawEntry, error)
}

type Service struct {
	store Store
}

func NewService(store Store) *Service {
	return &Service{store: store}
}

func (s *Service) Summary(ctx context.Context, userID string) (*Summary, error) {
	return s.store.Summary(ctx, userID)
}

func (s *Service) Rules(ctx context.Context) ([]Rule, error) {
	return s.store.Rules(ctx)
}

// ValidateRules memastikan aturan komisi masuk akal: tepat level 1..4, tiap bagian 0..100%, dan
// total TIDAK melebihi 100% dari biaya platform (platform tidak boleh membayar komisi lebih banyak
// daripada biaya yang diterimanya).
func ValidateRules(rules []Rule) error {
	bad := func(format string, a ...any) error { return httpx.ValidationError(fmt.Sprintf(format, a...)) }
	if len(rules) != 4 {
		return bad("harus tepat 4 aturan (level 1, 2, 3, 4)")
	}
	total := 0
	for i, r := range rules {
		if r.Level != i+1 {
			return bad("aturan harus berurutan level 1, 2, 3, 4")
		}
		if r.ShareBps < 0 || r.ShareBps > 10000 {
			return bad("level %d: share_bps harus 0 sampai 10000", r.Level)
		}
		total += r.ShareBps
	}
	if total > 10000 {
		return bad("total bagian komisi %d.%02d%% melebihi 100%% dari biaya platform", total/100, total%100)
	}
	return nil
}

// UpdateRules memvalidasi lalu menyimpan aturan komisi. Berlaku untuk pembayaran berikutnya;
// komisi yang sudah dibayarkan tidak berubah.
func (s *Service) UpdateRules(ctx context.Context, adminID string, rules []Rule) ([]Rule, error) {
	if err := ValidateRules(rules); err != nil {
		return nil, err
	}
	if err := s.store.UpdateRules(ctx, adminID, rules); err != nil {
		return nil, err
	}
	return s.store.Rules(ctx)
}

// maskName menyamarkan nama: tiap kata dipersingkat menjadi huruf pertama + "***".
// "Budi Santoso" -> "B*** S***". Pengguna melihat dari mana komisinya datang tanpa membuka identitas
// downline-nya.
func maskName(name string) string {
	words := strings.Fields(name)
	if len(words) == 0 {
		return "***"
	}
	for i, w := range words {
		r, _ := utf8.DecodeRuneInString(w)
		words[i] = string(r) + "***"
	}
	return strings.Join(words, " ")
}

// Commissions mengembalikan satu halaman riwayat komisi.
func (s *Service) Commissions(ctx context.Context, userID string, limit int, before int64) (*CommissionPage, error) {
	if limit == 0 {
		limit = DefaultListLimit
	}
	if limit < 1 || limit > MaxListLimit {
		return nil, httpx.ValidationError("limit harus 1 sampai 100")
	}
	if before < 0 {
		return nil, httpx.ValidationError("before tidak valid")
	}
	rows, err := s.store.Commissions(ctx, userID, limit, before)
	if err != nil {
		return nil, err
	}
	page := &CommissionPage{Commissions: []CommissionEntry{}}
	more := len(rows) > limit
	if more {
		rows = rows[:limit]
	}
	for _, e := range rows {
		page.Commissions = append(page.Commissions, CommissionEntry{
			ID: e.ID, Level: e.Level, ShareBps: e.ShareBps, Amount: e.Amount,
			SourceName: maskName(e.SourceName), CreatedAt: e.CreatedAt,
		})
	}
	if more {
		last := rows[len(rows)-1].ID
		page.NextBefore = &last
	}
	return page, nil
}
