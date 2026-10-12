/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package withdrawal

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/alexistdev/geopatungan/internal/httpx"
	"github.com/alexistdev/geopatungan/internal/settings"
)

const (
	// MaxOpenPerUser membatasi permintaan penarikan yang masih pending per pengguna.
	MaxOpenPerUser = 3
	// maxAmount adalah batas keras satu permintaan penarikan.
	maxAmount = 1_000_000_000

	DefaultListLimit = 50
	MaxListLimit     = 200
)

// Store adalah akses data penarikan (diimplementasikan *Repository).
type Store interface {
	Create(ctx context.Context, userID string, in Input, maxOpen int) (*Withdrawal, error)
	Get(ctx context.Context, userID, id string) (*Withdrawal, error)
	ListByUser(ctx context.Context, userID string, limit int) ([]Withdrawal, error)
	ListAdmin(ctx context.Context, status string, limit int) ([]AdminWithdrawal, error)
	Complete(ctx context.Context, adminID, id, reference, note string) (*Withdrawal, error)
	Reject(ctx context.Context, adminID, id, note string) (*Withdrawal, error)
	Cancel(ctx context.Context, userID, id string) (*Withdrawal, error)
}

// Settings adalah bagian dari paket settings yang dibutuhkan penarikan.
type Settings interface {
	Get(ctx context.Context) (settings.Settings, error)
}

type Service struct {
	store    Store
	settings Settings
}

func NewService(store Store, settings Settings) *Service {
	return &Service{store: store, settings: settings}
}

var accountNumberRe = regexp.MustCompile(`^[0-9]{5,30}$`)

func normalize(in Input) (Input, error) {
	in.BankName = strings.TrimSpace(in.BankName)
	in.AccountNumber = strings.ReplaceAll(strings.TrimSpace(in.AccountNumber), " ", "")
	in.AccountHolder = strings.TrimSpace(in.AccountHolder)
	in.Note = strings.TrimSpace(in.Note)

	switch {
	case len(in.BankName) < 2 || len(in.BankName) > 50:
		return in, httpx.ValidationError("nama bank 2 sampai 50 karakter")
	case !accountNumberRe.MatchString(in.AccountNumber):
		return in, httpx.ValidationError("nomor rekening harus 5 sampai 30 digit angka")
	case len(in.AccountHolder) < 2 || len(in.AccountHolder) > 100:
		return in, httpx.ValidationError("nama pemilik rekening 2 sampai 100 karakter")
	case len(in.Note) > 200:
		return in, httpx.ValidationError("catatan maksimal 200 karakter")
	}
	return in, nil
}

// Create meminta penarikan. Nominal minimum mengikuti pengaturan admin. Dana langsung
// dipindahkan ke dompet holding; saldo kurang menghasilkan *wallet.InsufficientFundsError.
func (s *Service) Create(ctx context.Context, userID string, in Input) (*Withdrawal, error) {
	cfg, err := s.settings.Get(ctx)
	if err != nil {
		return nil, err
	}
	if in.Amount < cfg.WithdrawMinAmount || in.Amount > maxAmount {
		return nil, httpx.ValidationError(fmt.Sprintf("nominal penarikan harus antara Rp%s dan Rp%s",
			httpx.Rupiah(cfg.WithdrawMinAmount), httpx.Rupiah(maxAmount)))
	}
	in, err = normalize(in)
	if err != nil {
		return nil, err
	}
	return s.store.Create(ctx, userID, in, MaxOpenPerUser)
}

func (s *Service) Get(ctx context.Context, userID, id string) (*Withdrawal, error) {
	return s.store.Get(ctx, userID, id)
}

func (s *Service) List(ctx context.Context, userID string) ([]Withdrawal, error) {
	return s.store.ListByUser(ctx, userID, DefaultListLimit)
}

func (s *Service) Cancel(ctx context.Context, userID, id string) (*Withdrawal, error) {
	return s.store.Cancel(ctx, userID, id)
}

var validStatuses = map[string]bool{
	StatusPending: true, StatusCompleted: true, StatusRejected: true, StatusCancelled: true, "all": true,
}

// ListAdmin: status kosong = pending. limit 0 = bawaan.
func (s *Service) ListAdmin(ctx context.Context, status string, limit int) ([]AdminWithdrawal, error) {
	if status == "" {
		status = StatusPending
	}
	if !validStatuses[status] {
		return nil, httpx.ValidationError("status tidak dikenal")
	}
	if limit == 0 {
		limit = DefaultListLimit
	}
	if limit < 1 || limit > MaxListLimit {
		return nil, httpx.ValidationError("limit harus 1 sampai 200")
	}
	return s.store.ListAdmin(ctx, status, limit)
}

// Complete: reference (nomor referensi/bukti transfer bank) wajib, supaya ada jejak bahwa uangnya
// benar-benar sudah dikirim.
func (s *Service) Complete(ctx context.Context, adminID, id, reference, note string) (*Withdrawal, error) {
	reference, note = strings.TrimSpace(reference), strings.TrimSpace(note)
	if len(reference) < 3 || len(reference) > 100 {
		return nil, httpx.ValidationError("referensi transfer 3 sampai 100 karakter")
	}
	if len(note) > 200 {
		return nil, httpx.ValidationError("catatan maksimal 200 karakter")
	}
	return s.store.Complete(ctx, adminID, id, reference, note)
}

// Reject wajib disertai alasan agar pengguna tahu mengapa ditolak.
func (s *Service) Reject(ctx context.Context, adminID, id, note string) (*Withdrawal, error) {
	note = strings.TrimSpace(note)
	if len(note) < 3 || len(note) > 200 {
		return nil, httpx.ValidationError("alasan penolakan 3 sampai 200 karakter")
	}
	return s.store.Reject(ctx, adminID, id, note)
}
