/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package round

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/alexistdev/geopatungan/internal/group"
	"github.com/alexistdev/geopatungan/internal/httpx"
)

type fakeMembers map[string]string // userID -> status; tidak ada = bukan anggota

func (f fakeMembers) FindMember(_ context.Context, _, userID string) (*group.Member, error) {
	st, ok := f[userID]
	if !ok {
		return nil, group.ErrNotFound
	}
	return &group.Member{UserID: userID, Status: st}, nil
}

type fakeStore struct {
	rounds []Round
	err    error
	calls  int
}

func (f *fakeStore) ListByGroup(context.Context, string) ([]Round, error) {
	f.calls++
	return f.rounds, f.err
}

var members = fakeMembers{"owner": "active", "member": "active", "pending": "pending", "rejected": "removed"}

const gid = "11111111-1111-1111-1111-111111111111"

func TestServiceListAccess(t *testing.T) {
	tests := []struct {
		user      string
		want      error
		wantCalls int
	}{
		{"owner", nil, 1},
		{"member", nil, 1},
		{"pending", group.ErrForbidden, 0},
		{"rejected", group.ErrNotFound, 0},
		{"stranger", group.ErrNotFound, 0},
	}
	for _, tc := range tests {
		t.Run(tc.user, func(t *testing.T) {
			store := &fakeStore{rounds: []Round{{RoundNo: 1}}}
			_, err := NewService(store, members).List(context.Background(), tc.user, gid)
			if tc.want == nil && err != nil || tc.want != nil && !errors.Is(err, tc.want) {
				t.Errorf("err = %v, want %v", err, tc.want)
			}
			if store.calls != tc.wantCalls {
				t.Errorf("store dipanggil %d kali, want %d (tanpa akses tidak boleh menyentuh data)", store.calls, tc.wantCalls)
			}
		})
	}
}

func serve(store *fakeStore, user, path string) *httptest.ResponseRecorder {
	h := NewHandler(NewService(store, members))
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			next.ServeHTTP(w, req.WithContext(httpx.WithUser(req.Context(), user, "member")))
		})
	})
	r.Route("/groups", h.Routes)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestHandlerList(t *testing.T) {
	t.Run("200 dengan bentuk JSON", func(t *testing.T) {
		store := &fakeStore{rounds: []Round{{ID: "r1", RoundNo: 1, DueDate: "2026-12-01", Status: "collecting", PotAmount: 300_000}}}
		rec := serve(store, "member", "/groups/"+gid+"/rounds")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body)
		}
		var body struct {
			Rounds []map[string]any `json:"rounds"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || len(body.Rounds) != 1 {
			t.Fatalf("body = %s (%v)", rec.Body, err)
		}
		r := body.Rounds[0]
		for _, key := range []string{"id", "group_id", "round_no", "due_date", "recipient_member_id",
			"recipient_name", "status", "paid_count", "member_count", "pot_amount"} {
			if _, ok := r[key]; !ok {
				t.Errorf("field %q hilang dari respons", key)
			}
		}
	})

	t.Run("daftar kosong berupa [] bukan null", func(t *testing.T) {
		rec := serve(&fakeStore{rounds: []Round{}}, "member", "/groups/"+gid+"/rounds")
		if !strings.Contains(rec.Body.String(), `"rounds":[]`) {
			t.Errorf("body = %s, want \"rounds\":[]", rec.Body)
		}
	})

	errCases := []struct {
		name, user, path string
		store            *fakeStore
		want             int
	}{
		{"id bukan UUID", "member", "/groups/abc/rounds", &fakeStore{}, 404},
		{"orang luar", "stranger", "/groups/" + gid + "/rounds", &fakeStore{}, 404},
		{"anggota pending", "pending", "/groups/" + gid + "/rounds", &fakeStore{}, 403},
		{"anggota ditolak", "rejected", "/groups/" + gid + "/rounds", &fakeStore{}, 404},
		{"kegagalan database", "member", "/groups/" + gid + "/rounds", &fakeStore{err: errors.New("pq: koneksi putus rahasia")}, 500},
	}
	for _, tc := range errCases {
		t.Run(tc.name, func(t *testing.T) {
			rec := serve(tc.store, tc.user, tc.path)
			if rec.Code != tc.want {
				t.Errorf("status = %d, want %d", rec.Code, tc.want)
			}
			if !strings.Contains(rec.Body.String(), `"error"`) {
				t.Errorf("body = %s, want JSON error", rec.Body)
			}
			if strings.Contains(rec.Body.String(), "rahasia") {
				t.Error("detail error internal bocor ke klien")
			}
		})
	}
}
