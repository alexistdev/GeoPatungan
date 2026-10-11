/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package round

import (
	"context"

	"github.com/alexistdev/geopatungan/internal/group"
)

// Store adalah akses data putaran (diimplementasikan *Repository).
type Store interface {
	ListByGroup(ctx context.Context, groupID string) ([]Round, error)
}

type Service struct {
	store   Store
	members group.MemberFinder
}

func NewService(store Store, members group.MemberFinder) *Service {
	return &Service{store: store, members: members}
}

// List hanya untuk anggota aktif grup (transparansi: semua anggota melihat jadwal yang sama).
func (s *Service) List(ctx context.Context, userID, groupID string) ([]Round, error) {
	if err := group.RequireActiveMember(ctx, s.members, groupID, userID); err != nil {
		return nil, err
	}
	return s.store.ListByGroup(ctx, groupID)
}
