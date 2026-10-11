/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package user

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Tes ini butuh database Postgres dengan migrasi sudah dijalankan.
// Dilewati otomatis jika TEST_DATABASE_URL tidak diisi.
//
//	TEST_DATABASE_URL="postgres://geopatungan:geopatungan_dev@localhost:5432/geopatungan?sslmode=disable" go test ./internal/user/
func newTestRepo(t *testing.T) (*Repository, *pgxpool.Pool, string) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL tidak diisi; tes integrasi dilewati")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("gagal konek: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping gagal: %v", err)
	}

	// Awalan unik per run: tes hanya menyentuh baris miliknya sendiri.
	prefix := fmt.Sprintf("uttest-%d", time.Now().UnixNano())
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM users WHERE email LIKE $1`, prefix+"-%")
		pool.Close()
	})
	return NewRepository(pool), pool, prefix
}

func newUser(prefix, name string, n int, referredBy *string) *User {
	return &User{
		Name:         name,
		Phone:        fmt.Sprintf("%s-%d", prefix, n),
		Email:        fmt.Sprintf("%s-%s@example.com", prefix, name),
		PasswordHash: "hash-bukan-asli",
		ReferralCode: fmt.Sprintf("%s-%d", prefix, n),
		ReferredBy:   referredBy,
	}
}

func TestCreateAndFind(t *testing.T) {
	repo, _, prefix := newTestRepo(t)
	ctx := context.Background()

	u := newUser(prefix, "a", 1, nil)
	if err := repo.Create(ctx, u); err != nil {
		t.Fatalf("Create error: %v", err)
	}
	if u.ID == "" || u.Role != "member" || u.KYCStatus != "none" || u.CreatedAt.IsZero() {
		t.Errorf("default dari database tidak terisi: %+v", u)
	}

	byEmail, err := repo.FindByEmail(ctx, u.Email)
	if err != nil || byEmail.ID != u.ID {
		t.Errorf("FindByEmail = %+v, %v", byEmail, err)
	}
	byCode, err := repo.FindByReferralCode(ctx, u.ReferralCode)
	if err != nil || byCode.ID != u.ID {
		t.Errorf("FindByReferralCode = %+v, %v", byCode, err)
	}
	byID, err := repo.FindByID(ctx, u.ID)
	if err != nil || byID.Email != u.Email {
		t.Errorf("FindByID = %+v, %v", byID, err)
	}

	if _, err := repo.FindByEmail(ctx, prefix+"-tidak-ada@example.com"); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestCreateDuplicate(t *testing.T) {
	repo, _, prefix := newTestRepo(t)
	ctx := context.Background()

	if err := repo.Create(ctx, newUser(prefix, "a", 1, nil)); err != nil {
		t.Fatal(err)
	}

	dupEmail := newUser(prefix, "a", 2, nil) // email sama, telepon & kode beda
	if err := repo.Create(ctx, dupEmail); !errors.Is(err, ErrDuplicate) {
		t.Errorf("email duplikat: err = %v, want ErrDuplicate", err)
	}

	dupPhone := newUser(prefix, "b", 1, nil) // telepon & kode sama, email beda
	if err := repo.Create(ctx, dupPhone); !errors.Is(err, ErrDuplicate) {
		t.Errorf("telepon duplikat: err = %v, want ErrDuplicate", err)
	}
}

// Rantai a > b > c > d > e > f > g: upline hanya dicatat sampai 4 level.
func TestCreateBuildsReferralTreeUpTo4Levels(t *testing.T) {
	repo, pool, prefix := newTestRepo(t)
	ctx := context.Background()

	names := []string{"a", "b", "c", "d", "e", "f", "g"}
	ids := make(map[string]string)
	var prev *string
	for i, name := range names {
		u := newUser(prefix, name, i+1, prev)
		if err := repo.Create(ctx, u); err != nil {
			t.Fatalf("Create %s: %v", name, err)
		}
		ids[name] = u.ID
		id := u.ID
		prev = &id
	}

	// siapa upline tiap user, per level
	want := map[string]map[int]string{
		"a": {},
		"b": {1: "a"},
		"c": {1: "b", 2: "a"},
		"e": {1: "d", 2: "c", 3: "b", 4: "a"},
		"f": {1: "e", 2: "d", 3: "c", 4: "b"}, // a sudah di luar 4 level
		"g": {1: "f", 2: "e", 3: "d", 4: "c"},
	}
	for name, levels := range want {
		rows, err := pool.Query(ctx,
			`SELECT ancestor_id, level FROM referrals WHERE descendant_id = $1`, ids[name])
		if err != nil {
			t.Fatal(err)
		}
		got := make(map[int]string)
		for rows.Next() {
			var anc string
			var lvl int
			if err := rows.Scan(&anc, &lvl); err != nil {
				t.Fatal(err)
			}
			got[lvl] = anc
		}
		rows.Close()

		if len(got) != len(levels) {
			t.Errorf("%s punya %d upline, want %d", name, len(got), len(levels))
		}
		for lvl, ancName := range levels {
			if got[lvl] != ids[ancName] {
				t.Errorf("upline level %d milik %s bukan %s", lvl, name, ancName)
			}
		}
	}
}

func TestCreateRollsBackWhenReferralInsertImpossible(t *testing.T) {
	repo, pool, prefix := newTestRepo(t)
	ctx := context.Background()

	ghost := "00000000-0000-0000-0000-000000000000" // pengajak yang tidak ada
	err := repo.Create(ctx, newUser(prefix, "orphan", 1, &ghost))
	if err == nil {
		t.Fatal("seharusnya gagal: referred_by menunjuk user yang tidak ada")
	}

	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM users WHERE email LIKE $1`, prefix+"-%").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("ada %d user tersisa; transaksi seharusnya dibatalkan seluruhnya", n)
	}
}

func TestSetRole(t *testing.T) {
	repo, pool, prefix := newTestRepo(t)
	ctx := context.Background()

	u := newUser(prefix, "a", 1, nil)
	if err := repo.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	roleOf := func() string {
		var r string
		if err := pool.QueryRow(ctx, `SELECT role FROM users WHERE id = $1`, u.ID).Scan(&r); err != nil {
			t.Fatal(err)
		}
		return r
	}

	if roleOf() != "member" {
		t.Fatalf("peran awal = %s, want member", roleOf())
	}
	// email tidak peka huruf besar/kecil
	if err := repo.SetRole(ctx, strings.ToUpper(u.Email), "admin"); err != nil {
		t.Fatalf("promote: %v", err)
	}
	if roleOf() != "admin" {
		t.Errorf("peran = %s, want admin", roleOf())
	}
	if err := repo.SetRole(ctx, u.Email, "member"); err != nil || roleOf() != "member" {
		t.Errorf("demote: err=%v peran=%s", err, roleOf())
	}

	if err := repo.SetRole(ctx, prefix+"-tidak-ada@example.com", "admin"); !errors.Is(err, ErrNotFound) {
		t.Errorf("email tidak ada: err = %v, want ErrNotFound", err)
	}
	for _, bad := range []string{"", "root", "ADMIN", "admin; DROP TABLE users"} {
		if err := repo.SetRole(ctx, u.Email, bad); err == nil {
			t.Errorf("peran %q seharusnya ditolak", bad)
		}
	}
	if roleOf() != "member" {
		t.Errorf("peran berubah oleh input tidak valid: %s", roleOf())
	}
}
