/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package contribution

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
	"github.com/alexistdev/geopatungan/internal/round"
)

type fakeMembers map[string]string // userID -> status

func (f fakeMembers) FindMember(_ context.Context, _, userID string) (*group.Member, error) {
	st, ok := f[userID]
	if !ok {
		return nil, group.ErrNotFound
	}
	return &group.Member{UserID: userID, Status: st}, nil
}

type fakeStore struct {
	byRound     []Contribution
	mine        []Mine
	err         error
	calls       int
	gotStatus   string
	gotRoundID  string
	gotGroupID  string
	gotUserID   string
	roundsKnown map[string]bool // nil = semua putaran dianggap ada
}

func (f *fakeStore) ListByRound(_ context.Context, groupID, roundID string) ([]Contribution, error) {
	f.calls++
	f.gotGroupID, f.gotRoundID = groupID, roundID
	if f.roundsKnown != nil && !f.roundsKnown[roundID] {
		return nil, round.ErrNotFound
	}
	return f.byRound, f.err
}

func (f *fakeStore) ListByUser(_ context.Context, userID, status string) ([]Mine, error) {
	f.calls++
	f.gotUserID, f.gotStatus = userID, status
	return f.mine, f.err
}

var members = fakeMembers{"owner": "active", "member": "active", "pending": "pending", "rejected": "removed"}

const (
	gid = "11111111-1111-1111-1111-111111111111"
	rid = "22222222-2222-2222-2222-222222222222"
)

func TestServiceListByRoundAccess(t *testing.T) {
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
			store := &fakeStore{byRound: []Contribution{{ID: "c1"}}}
			_, err := NewService(store, members).ListByRound(context.Background(), tc.user, gid, rid)
			if tc.want == nil && err != nil || tc.want != nil && !errors.Is(err, tc.want) {
				t.Errorf("err = %v, want %v", err, tc.want)
			}
			if store.calls != tc.wantCalls {
				t.Errorf("store dipanggil %d kali, want %d", store.calls, tc.wantCalls)
			}
		})
	}
}

func TestServiceMineValidatesStatus(t *testing.T) {
	for _, st := range []string{"", "unpaid", "paid"} {
		store := &fakeStore{}
		if _, err := NewService(store, members).Mine(context.Background(), "u1", st); err != nil {
			t.Errorf("status %q: %v", st, err)
		}
		if store.gotStatus != st || store.gotUserID != "u1" {
			t.Errorf("status %q: store menerima (%q, %q)", st, store.gotUserID, store.gotStatus)
		}
	}
	for _, st := range []string{"late", "PAID", "unpaid ", "' OR 1=1 --", "all"} {
		store := &fakeStore{}
		_, err := NewService(store, members).Mine(context.Background(), "u1", st)
		var verr group.ValidationError
		if !errors.As(err, &verr) {
			t.Errorf("status %q: err = %v, want ValidationError", st, err)
		}
		if store.calls != 0 {
			t.Errorf("status %q: input tidak valid tidak boleh sampai ke store", st)
		}
	}
}

func serve(store *fakeStore, user, method, path string) *httptest.ResponseRecorder {
	h := NewHandler(NewService(store, members))
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			next.ServeHTTP(w, req.WithContext(httpx.WithUser(req.Context(), user, "member")))
		})
	})
	r.Route("/groups", h.Routes)
	r.Get("/me/contributions", h.Mine)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec
}

func TestHandlerListByRound(t *testing.T) {
	url := "/groups/" + gid + "/rounds/" + rid + "/contributions"

	t.Run("200 dengan bentuk JSON", func(t *testing.T) {
		paid := true
		store := &fakeStore{byRound: []Contribution{{ID: "c1", RoundNo: 1, DueDate: "2026-12-01",
			Amount: 100_000, Status: "paid", PaidOnTime: &paid}}}
		rec := serve(store, "member", http.MethodGet, url)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d (%s)", rec.Code, rec.Body)
		}
		var body struct {
			Contributions []map[string]any `json:"contributions"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || len(body.Contributions) != 1 {
			t.Fatalf("body = %s (%v)", rec.Body, err)
		}
		for _, key := range []string{"id", "round_id", "round_no", "due_date", "member_id", "user_name", "amount",
			"platform_fee", "late_fee", "status", "paid_on_time", "paid_at", "overdue"} {
			if _, ok := body.Contributions[0][key]; !ok {
				t.Errorf("field %q hilang", key)
			}
		}
		if store.gotGroupID != gid || store.gotRoundID != rid {
			t.Errorf("store menerima (%s, %s)", store.gotGroupID, store.gotRoundID)
		}
	})

	errCases := []struct {
		name, user, path string
		store            *fakeStore
		want             int
	}{
		{"group id bukan UUID", "member", "/groups/abc/rounds/" + rid + "/contributions", &fakeStore{}, 404},
		{"round id bukan UUID", "member", "/groups/" + gid + "/rounds/xyz/contributions", &fakeStore{}, 404},
		{"orang luar", "stranger", url, &fakeStore{}, 404},
		{"anggota pending", "pending", url, &fakeStore{}, 403},
		{"putaran bukan milik grup", "member", url, &fakeStore{roundsKnown: map[string]bool{}}, 404},
		{"kegagalan database", "member", url, &fakeStore{err: errors.New("pq: rahasia")}, 500},
	}
	for _, tc := range errCases {
		t.Run(tc.name, func(t *testing.T) {
			rec := serve(tc.store, tc.user, http.MethodGet, tc.path)
			if rec.Code != tc.want {
				t.Errorf("status = %d, want %d (%s)", rec.Code, tc.want, rec.Body)
			}
			if !strings.Contains(rec.Body.String(), `"error"`) || strings.Contains(rec.Body.String(), "rahasia") {
				t.Errorf("body = %s", rec.Body)
			}
		})
	}
}

func TestHandlerMine(t *testing.T) {
	t.Run("meneruskan status dan identitas pengguna", func(t *testing.T) {
		store := &fakeStore{mine: []Mine{{GroupID: gid, GroupName: "Arisan RT"}}}
		rec := serve(store, "u-77", http.MethodGet, "/me/contributions?status=unpaid")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d (%s)", rec.Code, rec.Body)
		}
		if store.gotUserID != "u-77" || store.gotStatus != "unpaid" {
			t.Errorf("store menerima (%q, %q)", store.gotUserID, store.gotStatus)
		}
		if !strings.Contains(rec.Body.String(), `"group_name":"Arisan RT"`) ||
			!strings.Contains(rec.Body.String(), `"group_id"`) {
			t.Errorf("info grup tidak ikut di JSON: %s", rec.Body)
		}
	})

	t.Run("daftar kosong berupa []", func(t *testing.T) {
		rec := serve(&fakeStore{mine: []Mine{}}, "u1", http.MethodGet, "/me/contributions")
		if !strings.Contains(rec.Body.String(), `"contributions":[]`) {
			t.Errorf("body = %s", rec.Body)
		}
	})

	t.Run("status tidak valid -> 400", func(t *testing.T) {
		store := &fakeStore{}
		rec := serve(store, "u1", http.MethodGet, "/me/contributions?status=late")
		if rec.Code != http.StatusBadRequest || store.calls != 0 {
			t.Errorf("status = %d calls = %d, want 400 dan 0", rec.Code, store.calls)
		}
	})

	t.Run("kegagalan database -> 500 tanpa bocor", func(t *testing.T) {
		rec := serve(&fakeStore{err: errors.New("pq: rahasia")}, "u1", http.MethodGet, "/me/contributions")
		if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "rahasia") {
			t.Errorf("status = %d body = %s", rec.Code, rec.Body)
		}
	})
}
