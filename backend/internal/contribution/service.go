/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package contribution

import (
	"context"

	"github.com/alexistdev/geopatungan/internal/group"
)

// Store adalah akses data iuran (diimplementasikan *Repository).
type Store interface {
	ListByRound(ctx context.Context, groupID, roundID string) ([]Contribution, error)
	ListByUser(ctx context.Context, userID, status string) ([]Mine, error)
}

type Service struct {
	store   Store
	members group.MemberFinder
}

func NewService(store Store, members group.MemberFinder) *Service {
	return &Service{store: store, members: members}
}

// ListByRound hanya untuk anggota aktif grup. Semua anggota melihat siapa yang
// sudah dan belum membayar (transparansi).
func (s *Service) ListByRound(ctx context.Context, userID, groupID, roundID string) ([]Contribution, error) {
	if err := group.RequireActiveMember(ctx, s.members, groupID, userID); err != nil {
		return nil, err
	}
	return s.store.ListByRound(ctx, groupID, roundID)
}

// Mine mengembalikan iuran milik pengguna di semua grupnya. status: "", "unpaid", atau "paid".
func (s *Service) Mine(ctx context.Context, userID, status string) ([]Mine, error) {
	if status != "" && status != "unpaid" && status != "paid" {
		return nil, group.ValidationError("status harus unpaid atau paid")
	}
	return s.store.ListByUser(ctx, userID, status)
}
