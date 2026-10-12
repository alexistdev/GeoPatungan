/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package topup

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"math/big"
	"strings"

	"github.com/alexistdev/geopatungan/internal/httpx"
	"github.com/alexistdev/geopatungan/internal/settings"
)

const (
	// MaxOpenPerUser membatasi permintaan top-up terbuka (pending/submitted) per pengguna.
	MaxOpenPerUser = 3
	// codeCandidates adalah jumlah kode unik yang dicoba per permintaan sebelum menyerah.
	codeCandidates = 25
	// maxReceived adalah batas keras nominal yang boleh dikreditkan admin dalam satu persetujuan.
	maxReceived = 1_000_000_000

	DefaultListLimit = 50
	MaxListLimit     = 200
)

// Store adalah akses data top-up (diimplementasikan *Repository).
type Store interface {
	Create(ctx context.Context, userID string, amount int64, codes []int, expiryHours, maxOpen int) (*Topup, error)
	Get(ctx context.Context, userID, id string) (*Topup, error)
	ListByUser(ctx context.Context, userID string, limit int) ([]Topup, error)
	Confirm(ctx context.Context, userID, id, senderName, senderBank, note string) (*Topup, error)
	Cancel(ctx context.Context, userID, id string) (*Topup, error)
	ListAdmin(ctx context.Context, status string, limit int) ([]AdminTopup, error)
	Approve(ctx context.Context, adminID, id string, received *int64, note string) (*Topup, error)
	Reject(ctx context.Context, adminID, id, note string) (*Topup, error)
}

// Settings adalah bagian dari paket settings yang dibutuhkan top-up.
type Settings interface {
	Get(ctx context.Context) (settings.Settings, error)
	ListBanks(ctx context.Context, onlyActive bool) ([]settings.BankAccount, error)
}

type Service struct {
	store    Store
	settings Settings
	rand     io.Reader
}

func NewService(store Store, settings Settings) *Service {
	return &Service{store: store, settings: settings, rand: rand.Reader}
}

// rupiah memformat nominal dengan titik ribuan: 1500000 -> "1.500.000".
func rupiah(n int64) string {
	s := fmt.Sprintf("%d", n)
	var out []byte
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, '.')
		}
		out = append(out, c)
	}
	return string(out)
}

// randomCodes mengembalikan n kode unik berbeda dalam rentang 1..999, berurutan acak.
func randomCodes(r io.Reader, n int) ([]int, error) {
	if n < 1 || n > 999 {
		return nil, fmt.Errorf("jumlah kode harus 1 sampai 999")
	}
	seen := make(map[int]bool, n)
	out := make([]int, 0, n)
	for len(out) < n {
		v, err := rand.Int(r, big.NewInt(999))
		if err != nil {
			return nil, err
		}
		code := int(v.Int64()) + 1
		if !seen[code] {
			seen[code] = true
			out = append(out, code)
		}
	}
	return out, nil
}

// Create membuat permintaan top-up dan mengembalikan rekening tujuan transfer.
func (s *Service) Create(ctx context.Context, userID string, amount int64) (*Detail, error) {
	cfg, err := s.settings.Get(ctx)
	if err != nil {
		return nil, err
	}
	if amount < cfg.TopupMinAmount || amount > cfg.TopupMaxAmount {
		return nil, httpx.ValidationError(fmt.Sprintf("nominal top-up harus antara Rp%s dan Rp%s",
			rupiah(cfg.TopupMinAmount), rupiah(cfg.TopupMaxAmount)))
	}
	banks, err := s.settings.ListBanks(ctx, true)
	if err != nil {
		return nil, err
	}
	if len(banks) == 0 {
		return nil, ErrNoBankAccount
	}
	codes, err := randomCodes(s.rand, codeCandidates)
	if err != nil {
		return nil, err
	}
	t, err := s.store.Create(ctx, userID, amount, codes, cfg.TopupExpiryHours, MaxOpenPerUser)
	if err != nil {
		return nil, err
	}
	return &Detail{Topup: t, BankAccounts: banks}, nil
}

// Get mengembalikan permintaan milik pengguna; rekening tujuan hanya disertakan selagi terbuka.
func (s *Service) Get(ctx context.Context, userID, id string) (*Detail, error) {
	t, err := s.store.Get(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	d := &Detail{Topup: t, BankAccounts: []settings.BankAccount{}}
	if t.Open() {
		banks, err := s.settings.ListBanks(ctx, true)
		if err != nil {
			return nil, err
		}
		d.BankAccounts = banks
	}
	return d, nil
}

func (s *Service) List(ctx context.Context, userID string) ([]Topup, error) {
	return s.store.ListByUser(ctx, userID, DefaultListLimit)
}

func (s *Service) Confirm(ctx context.Context, userID, id, senderName, senderBank, note string) (*Topup, error) {
	senderName, senderBank, note = strings.TrimSpace(senderName), strings.TrimSpace(senderBank), strings.TrimSpace(note)
	switch {
	case len(senderName) < 2 || len(senderName) > 100:
		return nil, httpx.ValidationError("nama pengirim 2 sampai 100 karakter")
	case len(senderBank) < 2 || len(senderBank) > 50:
		return nil, httpx.ValidationError("bank pengirim 2 sampai 50 karakter")
	case len(note) > 200:
		return nil, httpx.ValidationError("catatan maksimal 200 karakter")
	}
	return s.store.Confirm(ctx, userID, id, senderName, senderBank, note)
}

func (s *Service) Cancel(ctx context.Context, userID, id string) (*Topup, error) {
	return s.store.Cancel(ctx, userID, id)
}

var validStatuses = map[string]bool{
	"open": true, "all": true, StatusPending: true, StatusSubmitted: true,
	StatusApproved: true, StatusRejected: true, StatusCancelled: true, StatusExpired: true,
}

// ListAdmin: status kosong = "open". limit 0 = bawaan.
func (s *Service) ListAdmin(ctx context.Context, status string, limit int) ([]AdminTopup, error) {
	if status == "" {
		status = "open"
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

// Approve: received nil = sebesar nominal transfer. Nominal yang disesuaikan harus positif
// dan tidak melebihi batas keras.
func (s *Service) Approve(ctx context.Context, adminID, id string, received *int64, note string) (*Topup, error) {
	note = strings.TrimSpace(note)
	if len(note) > 200 {
		return nil, httpx.ValidationError("catatan maksimal 200 karakter")
	}
	if received != nil && (*received < 1 || *received > maxReceived) {
		return nil, httpx.ValidationError("received_amount harus antara Rp1 dan Rp1.000.000.000")
	}
	return s.store.Approve(ctx, adminID, id, received, note)
}

// Reject wajib disertai alasan agar pengguna tahu mengapa ditolak.
func (s *Service) Reject(ctx context.Context, adminID, id, note string) (*Topup, error) {
	note = strings.TrimSpace(note)
	if len(note) < 3 || len(note) > 200 {
		return nil, httpx.ValidationError("alasan penolakan 3 sampai 200 karakter")
	}
	return s.store.Reject(ctx, adminID, id, note)
}
