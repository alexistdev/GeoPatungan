/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package wallet

import (
	"context"

	"github.com/alexistdev/geopatungan/internal/httpx"
)

const (
	DefaultLedgerLimit = 20
	MaxLedgerLimit     = 100
)

// Store adalah akses data dompet (diimplementasikan *Repository).
type Store interface {
	ByUser(ctx context.Context, userID string) (*Wallet, error)
	Ledger(ctx context.Context, userID string, limit int, before int64) ([]Entry, error)
	Reconcile(ctx context.Context) (*Report, error)
}

type Service struct {
	store Store
}

func NewService(store Store) *Service {
	return &Service{store: store}
}

func (s *Service) Wallet(ctx context.Context, userID string) (*Wallet, error) {
	return s.store.ByUser(ctx, userID)
}

// LedgerPage adalah satu halaman riwayat. NextBefore terisi jika masih ada halaman berikutnya;
// kirim nilainya sebagai parameter `before` untuk melanjutkan.
type LedgerPage struct {
	Entries    []Entry `json:"entries"`
	NextBefore *int64  `json:"next_before"`
}

// Ledger mengembalikan riwayat mutasi pengguna. limit 0 = bawaan; di atas maksimum ditolak.
func (s *Service) Ledger(ctx context.Context, userID string, limit int, before int64) (*LedgerPage, error) {
	if limit == 0 {
		limit = DefaultLedgerLimit
	}
	if limit < 1 || limit > MaxLedgerLimit {
		return nil, httpx.ValidationError("limit harus 1 sampai 100")
	}
	if before < 0 {
		return nil, httpx.ValidationError("before tidak valid")
	}
	entries, err := s.store.Ledger(ctx, userID, limit, before)
	if err != nil {
		return nil, err
	}
	page := &LedgerPage{Entries: entries}
	if len(entries) > limit { // ada halaman berikutnya
		page.Entries = entries[:limit]
		next := entries[limit-1].ID
		page.NextBefore = &next
	}
	return page, nil
}

func (s *Service) Reconcile(ctx context.Context) (*Report, error) {
	return s.store.Reconcile(ctx)
}
