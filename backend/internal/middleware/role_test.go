/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alexistdev/geopatungan/internal/httpx"
	"github.com/alexistdev/geopatungan/internal/user"
)

type fakeUsers struct {
	roles map[string]string // userID -> peran SAAT INI di database
	err   error
}

func (f *fakeUsers) FindByID(_ context.Context, id string) (*user.User, error) {
	if f.err != nil {
		return nil, f.err
	}
	role, ok := f.roles[id]
	if !ok {
		return nil, user.ErrNotFound
	}
	return &user.User{ID: id, Role: role}, nil
}

func runRole(users RoleLookup, userID, tokenRole string, roles ...string) (*httptest.ResponseRecorder, bool, string) {
	var called bool
	var seenRole string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		seenRole = httpx.Role(r.Context())
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if userID != "" {
		req = req.WithContext(httpx.WithUser(req.Context(), userID, tokenRole))
	}
	rec := httptest.NewRecorder()
	RequireRole(users, roles...)(next).ServeHTTP(rec, req)
	return rec, called, seenRole
}

func TestRequireRole(t *testing.T) {
	users := &fakeUsers{roles: map[string]string{"admin-1": "admin", "member-1": "member", "owner-1": "owner"}}

	tests := []struct {
		name       string
		userID     string
		tokenRole  string
		roles      []string
		wantStatus int
		wantNext   bool
	}{
		{"admin lolos", "admin-1", "admin", []string{"admin"}, 200, true},
		{"member ditolak", "member-1", "member", []string{"admin"}, 403, false},
		{"salah satu dari beberapa peran", "owner-1", "owner", []string{"admin", "owner"}, 200, true},
		{"tanpa identitas (belum login)", "", "", []string{"admin"}, 401, false},
		{"akun sudah dihapus", "hantu", "admin", []string{"admin"}, 401, false},
		// token masih mengaku admin, tetapi database sudah menurunkannya
		{"admin yang dicabut haknya", "member-1", "admin", []string{"admin"}, 403, false},
		// token mengaku member, tetapi database sudah menjadikannya admin
		{"member yang baru dipromosikan", "admin-1", "member", []string{"admin"}, 200, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec, called, _ := runRole(users, tc.userID, tc.tokenRole, tc.roles...)
			if rec.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
			if called != tc.wantNext {
				t.Errorf("next dipanggil = %v, want %v", called, tc.wantNext)
			}
			if !tc.wantNext && !strings.Contains(rec.Body.String(), `"error"`) {
				t.Errorf("body = %s, want JSON error", rec.Body)
			}
		})
	}
}

func TestRequireRoleUsesCurrentRoleInContext(t *testing.T) {
	users := &fakeUsers{roles: map[string]string{"u": "admin"}}
	_, called, seen := runRole(users, "u", "member", "admin")
	if !called || seen != "admin" {
		t.Errorf("called=%v role di context = %q, want admin (peran dari database)", called, seen)
	}
}

func TestRequireRoleDatabaseFailureDoesNotLeak(t *testing.T) {
	users := &fakeUsers{err: errors.New("pq: koneksi putus rahasia")}
	rec, called, _ := runRole(users, "u", "admin", "admin")
	if rec.Code != http.StatusInternalServerError || called {
		t.Errorf("status = %d called = %v, want 500 dan false", rec.Code, called)
	}
	if strings.Contains(rec.Body.String(), "rahasia") {
		t.Error("detail error internal bocor ke klien")
	}
}
