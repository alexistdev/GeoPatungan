/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package draw

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"

	"github.com/alexistdev/geopatungan/internal/group"
)

// Store adalah akses data kocokan (diimplementasikan *Repository).
type Store interface {
	Commit(ctx context.Context, d *Draw) error
	Execute(ctx context.Context, groupID string, ordered []Entry) error
	Manual(ctx context.Context, d *Draw) error
	Get(ctx context.Context, groupID string) (*Draw, error)
}

// GroupReader adalah bagian dari paket group yang dibutuhkan kocokan.
type GroupReader interface {
	FindByID(ctx context.Context, id string) (*group.Group, error)
	FindMember(ctx context.Context, groupID, userID string) (*group.Member, error)
	ListMembers(ctx context.Context, groupID string) ([]group.Member, error)
}

type Service struct {
	groups GroupReader
	store  Store
	levels group.LevelProvider
	rand   io.Reader
}

func NewService(groups GroupReader, store Store, levels group.LevelProvider) *Service {
	return &Service{groups: groups, store: store, levels: levels, rand: rand.Reader}
}

// Commit membuat seed rahasia, mengunci bobot tiap anggota, dan mengumumkan
// hash seed. Hanya pemilik; grup harus open dan penuh; metode random/weighted.
//
// Batasan yang jujur: seed dibuat dan disimpan server, jadi komitmen ini membuktikan
// seed tidak diganti setelah komitmen diumumkan dan hasilnya bisa diverifikasi ulang,
// tetapi tidak membuktikan operator tidak mencoba banyak seed sebelum mengumumkannya.
func (s *Service) Commit(ctx context.Context, userID, groupID string) (*Draw, error) {
	g, err := s.ownerGroup(ctx, userID, groupID)
	if err != nil {
		return nil, err
	}
	if g.Status != "open" {
		return nil, ErrGroupStarted
	}
	if g.DrawMethod == "manual" {
		return nil, ErrWrongMethod
	}

	members, err := s.activeMembers(ctx, groupID)
	if err != nil {
		return nil, err
	}
	if len(members) != g.SlotCount {
		return nil, ErrGroupNotFull
	}

	entries := make([]Entry, len(members))
	for i, m := range members {
		w := 1.0 // random: semua bobot setara
		if g.DrawMethod == "weighted" {
			rank, err := s.levels.Rank(ctx, m.UserID)
			if err != nil {
				return nil, err
			}
			w = WeightForRank(rank)
		}
		entries[i] = Entry{MemberID: m.ID, UserName: m.UserName, Weight: w}
	}

	seed, err := NewSeed(s.rand)
	if err != nil {
		return nil, err
	}
	d := &Draw{
		GroupID:    groupID,
		Algorithm:  Algorithm,
		SeedCommit: CommitOf(seed),
		Entries:    entries,
		seed:       hex.EncodeToString(seed),
	}
	if err := s.store.Commit(ctx, d); err != nil {
		return nil, err
	}
	return d, nil
}

// Execute menghitung urutan dari seed yang sudah dikomit, menyimpannya, membuka seed,
// dan memulai grup (status running).
func (s *Service) Execute(ctx context.Context, userID, groupID string) (*Draw, error) {
	if _, err := s.ownerGroup(ctx, userID, groupID); err != nil {
		return nil, err
	}
	d, err := s.store.Get(ctx, groupID)
	if err != nil {
		return nil, err
	}
	if d.ExecutedAt != nil {
		return nil, ErrAlreadyExecuted
	}
	if d.Algorithm != Algorithm {
		return nil, ErrWrongMethod
	}

	seed, err := hex.DecodeString(d.seed)
	if err != nil {
		return nil, err
	}
	ordered, err := ComputeOrder(seed, d.Entries)
	if err != nil {
		return nil, err
	}
	if err := s.store.Execute(ctx, groupID, ordered); err != nil {
		return nil, err
	}
	return s.store.Get(ctx, groupID)
}

// Manual menetapkan urutan giliran sesuai daftar member ID dari pemilik grup
// (hanya untuk grup ber-draw_method manual) dan memulai grup.
func (s *Service) Manual(ctx context.Context, userID, groupID string, order []string) (*Draw, error) {
	g, err := s.ownerGroup(ctx, userID, groupID)
	if err != nil {
		return nil, err
	}
	if g.Status != "open" {
		return nil, ErrGroupStarted
	}
	if g.DrawMethod != "manual" {
		return nil, ErrWrongMethod
	}

	members, err := s.activeMembers(ctx, groupID)
	if err != nil {
		return nil, err
	}
	if len(members) != g.SlotCount {
		return nil, ErrGroupNotFull
	}

	names := make(map[string]string, len(members))
	for _, m := range members {
		names[m.ID] = m.UserName
	}
	if len(order) != len(members) {
		return nil, group.ValidationError("urutan harus memuat semua anggota aktif tepat satu kali")
	}
	entries := make([]Entry, len(order))
	seen := make(map[string]bool, len(order))
	for i, id := range order {
		name, ok := names[id]
		if !ok || seen[id] {
			return nil, group.ValidationError("urutan harus memuat semua anggota aktif tepat satu kali")
		}
		seen[id] = true
		pos := i + 1
		entries[i] = Entry{MemberID: id, UserName: name, Weight: 1, Order: &pos}
	}

	d := &Draw{GroupID: groupID, Algorithm: "manual", Entries: entries}
	if err := s.store.Manual(ctx, d); err != nil {
		return nil, err
	}
	return s.store.Get(ctx, groupID)
}

// Get hanya untuk anggota aktif grup (termasuk pemilik).
func (s *Service) Get(ctx context.Context, userID, groupID string) (*Draw, error) {
	m, err := s.groups.FindMember(ctx, groupID, userID)
	if err != nil {
		return nil, err
	}
	if m.Status == "removed" {
		return nil, group.ErrNotFound
	}
	if m.Status != "active" {
		return nil, group.ErrForbidden
	}
	return s.store.Get(ctx, groupID)
}

func (s *Service) ownerGroup(ctx context.Context, userID, groupID string) (*group.Group, error) {
	g, err := s.groups.FindByID(ctx, groupID)
	if err != nil {
		return nil, err
	}
	m, err := s.groups.FindMember(ctx, groupID, userID)
	if err != nil {
		return nil, err // bukan anggota: 404, keberadaan grup tidak bocor
	}
	if m.Status == "removed" {
		return nil, group.ErrNotFound
	}
	if g.OwnerID != userID {
		return nil, group.ErrForbidden
	}
	return g, nil
}

func (s *Service) activeMembers(ctx context.Context, groupID string) ([]group.Member, error) {
	all, err := s.groups.ListMembers(ctx, groupID)
	if err != nil {
		return nil, err
	}
	active := make([]group.Member, 0, len(all))
	for _, m := range all {
		if m.Status == "active" {
			active = append(active, m)
		}
	}
	return active, nil
}
