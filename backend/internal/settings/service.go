/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package settings

import (
	"context"
	"regexp"
	"strings"

	"github.com/alexistdev/geopatungan/internal/httpx"
)

// Store adalah akses data pengaturan (diimplementasikan *Repository).
type Store interface {
	Get(ctx context.Context) (Settings, error)
	Update(ctx context.Context, adminID string, s Settings) error
	CreateBank(ctx context.Context, adminID string, b *BankAccount) error
	UpdateBank(ctx context.Context, adminID, id string, b *BankAccount) error
	ListBanks(ctx context.Context, onlyActive bool) ([]BankAccount, error)
}

type Service struct {
	store Store
}

func NewService(store Store) *Service {
	return &Service{store: store}
}

func (s *Service) Get(ctx context.Context) (Settings, error) {
	return s.store.Get(ctx)
}

func (s *Service) Update(ctx context.Context, adminID string, in Settings) (Settings, error) {
	if err := validateSettings(in); err != nil {
		return Settings{}, err
	}
	if err := s.store.Update(ctx, adminID, in); err != nil {
		return Settings{}, err
	}
	return in, nil
}

func validateSettings(s Settings) error {
	switch {
	case s.PlatformFeeBps < 0 || s.PlatformFeeBps > 1000:
		return httpx.ValidationError("platform_fee_bps harus 0 sampai 1000 (maksimal 10%)")
	case s.TopupMinAmount < 1_000:
		return httpx.ValidationError("topup_min_amount minimal Rp1.000")
	case s.TopupMaxAmount < s.TopupMinAmount:
		return httpx.ValidationError("topup_max_amount tidak boleh lebih kecil dari topup_min_amount")
	case s.TopupMaxAmount > 1_000_000_000:
		return httpx.ValidationError("topup_max_amount maksimal Rp1.000.000.000")
	case s.WithdrawMinAmount < 10_000:
		return httpx.ValidationError("withdraw_min_amount minimal Rp10.000")
	case s.TopupExpiryHours < 1 || s.TopupExpiryHours > 168:
		return httpx.ValidationError("topup_expiry_hours harus 1 sampai 168 jam")
	}
	return nil
}

var accountNumberRe = regexp.MustCompile(`^[0-9]{5,30}$`)

func normalizeBank(b BankAccount) (BankAccount, error) {
	b.BankName = strings.TrimSpace(b.BankName)
	b.AccountNumber = strings.ReplaceAll(strings.TrimSpace(b.AccountNumber), " ", "")
	b.AccountHolder = strings.TrimSpace(b.AccountHolder)
	b.Notes = strings.TrimSpace(b.Notes)

	switch {
	case len(b.BankName) < 2 || len(b.BankName) > 50:
		return b, httpx.ValidationError("nama bank 2 sampai 50 karakter")
	case !accountNumberRe.MatchString(b.AccountNumber):
		return b, httpx.ValidationError("nomor rekening harus 5 sampai 30 digit angka")
	case len(b.AccountHolder) < 2 || len(b.AccountHolder) > 100:
		return b, httpx.ValidationError("nama pemilik rekening 2 sampai 100 karakter")
	case len(b.Notes) > 500:
		return b, httpx.ValidationError("catatan maksimal 500 karakter")
	}
	return b, nil
}

func (s *Service) CreateBank(ctx context.Context, adminID string, in BankAccount) (*BankAccount, error) {
	b, err := normalizeBank(in)
	if err != nil {
		return nil, err
	}
	if err := s.store.CreateBank(ctx, adminID, &b); err != nil {
		return nil, err
	}
	return &b, nil
}

func (s *Service) UpdateBank(ctx context.Context, adminID, id string, in BankAccount) (*BankAccount, error) {
	b, err := normalizeBank(in)
	if err != nil {
		return nil, err
	}
	if err := s.store.UpdateBank(ctx, adminID, id, &b); err != nil {
		return nil, err
	}
	return &b, nil
}

// ListBanks untuk admin: semua rekening, termasuk yang nonaktif.
func (s *Service) ListBanks(ctx context.Context) ([]BankAccount, error) {
	return s.store.ListBanks(ctx, false)
}

// PaymentInfo untuk pengguna: hanya rekening aktif beserta batas nominal.
func (s *Service) PaymentInfo(ctx context.Context) (*PaymentInfo, error) {
	cfg, err := s.store.Get(ctx)
	if err != nil {
		return nil, err
	}
	banks, err := s.store.ListBanks(ctx, true)
	if err != nil {
		return nil, err
	}
	return &PaymentInfo{
		BankAccounts:      banks,
		TopupMinAmount:    cfg.TopupMinAmount,
		TopupMaxAmount:    cfg.TopupMaxAmount,
		WithdrawMinAmount: cfg.WithdrawMinAmount,
		PlatformFeeBps:    cfg.PlatformFeeBps,
		TopupExpiryHours:  cfg.TopupExpiryHours,
	}, nil
}
