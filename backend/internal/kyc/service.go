/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package kyc

import (
	"context"
	"strings"

	"github.com/alexistdev/geopatungan/internal/httpx"
)

const (
	DefaultListLimit = 50
	MaxListLimit     = 200
)

// Store adalah akses data KYC (diimplementasikan *Repository).
type Store interface {
	Submit(ctx context.Context, userID, note string) (*Request, error)
	StatusOf(ctx context.Context, userID string) (*Status, error)
	ListAdmin(ctx context.Context, status string, limit int) ([]AdminRequest, error)
	Review(ctx context.Context, adminID, id, decision, note string) (*Request, error)
}

type Service struct {
	store Store
}

func NewService(store Store) *Service {
	return &Service{store: store}
}

func (s *Service) Submit(ctx context.Context, userID, note string) (*Request, error) {
	note = strings.TrimSpace(note)
	if len(note) > 200 {
		return nil, httpx.ValidationError("catatan maksimal 200 karakter")
	}
	return s.store.Submit(ctx, userID, note)
}

func (s *Service) Status(ctx context.Context, userID string) (*Status, error) {
	return s.store.StatusOf(ctx, userID)
}

var validStatuses = map[string]bool{StatusPending: true, StatusVerified: true, StatusRejected: true, "all": true}

// ListAdmin: status kosong = pending. limit 0 = bawaan.
func (s *Service) ListAdmin(ctx context.Context, status string, limit int) ([]AdminRequest, error) {
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

func (s *Service) Approve(ctx context.Context, adminID, id, note string) (*Request, error) {
	note = strings.TrimSpace(note)
	if len(note) > 200 {
		return nil, httpx.ValidationError("catatan maksimal 200 karakter")
	}
	return s.store.Review(ctx, adminID, id, StatusVerified, note)
}

// Reject wajib disertai alasan agar pengguna tahu apa yang perlu diperbaiki sebelum mengajukan lagi.
func (s *Service) Reject(ctx context.Context, adminID, id, note string) (*Request, error) {
	note = strings.TrimSpace(note)
	if len(note) < 3 || len(note) > 200 {
		return nil, httpx.ValidationError("alasan penolakan 3 sampai 200 karakter")
	}
	return s.store.Review(ctx, adminID, id, StatusRejected, note)
}
