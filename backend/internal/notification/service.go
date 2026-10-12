/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package notification

import (
	"context"

	"github.com/alexistdev/geopatungan/internal/httpx"
)

const (
	DefaultListLimit = 20
	MaxListLimit     = 100
)

// Store adalah akses data pemberitahuan (diimplementasikan *Repository).
type Store interface {
	List(ctx context.Context, userID string, limit int, before int64, unreadOnly bool) ([]Item, error)
	UnreadCount(ctx context.Context, userID string) (int, error)
	MarkRead(ctx context.Context, userID string, id int64) error
	MarkAllRead(ctx context.Context, userID string) (int64, error)
}

type Service struct {
	store Store
}

func NewService(store Store) *Service {
	return &Service{store: store}
}

// List mengembalikan satu halaman pemberitahuan beserta jumlah yang belum dibaca.
func (s *Service) List(ctx context.Context, userID string, limit int, before int64, unreadOnly bool) (*Page, error) {
	if limit == 0 {
		limit = DefaultListLimit
	}
	if limit < 1 || limit > MaxListLimit {
		return nil, httpx.ValidationError("limit harus 1 sampai 100")
	}
	if before < 0 {
		return nil, httpx.ValidationError("before tidak valid")
	}
	items, err := s.store.List(ctx, userID, limit, before, unreadOnly)
	if err != nil {
		return nil, err
	}
	page := &Page{Notifications: items}
	if len(items) > limit {
		page.Notifications = items[:limit]
		last := items[limit-1].ID
		page.NextBefore = &last
	}
	if page.UnreadCount, err = s.store.UnreadCount(ctx, userID); err != nil {
		return nil, err
	}
	return page, nil
}

func (s *Service) UnreadCount(ctx context.Context, userID string) (int, error) {
	return s.store.UnreadCount(ctx, userID)
}

func (s *Service) MarkRead(ctx context.Context, userID string, id int64) error {
	if id < 1 {
		return ErrNotFound
	}
	return s.store.MarkRead(ctx, userID, id)
}

func (s *Service) MarkAllRead(ctx context.Context, userID string) (int64, error) {
	return s.store.MarkAllRead(ctx, userID)
}
