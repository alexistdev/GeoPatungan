/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package group

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/alexistdev/geopatungan/internal/httpx"
)

// newTestServer memasang rute grup dengan "autentikasi" palsu: identitas
// diambil dari header X-Test-User (middleware JWT asli diuji di paketnya sendiri).
func newTestServer(level int) http.Handler {
	h := NewHandler(newTestService(newFakeStore(), level))
	r := chi.NewRouter()
	r.Group(func(r chi.Router) {
		r.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				id := req.Header.Get("X-Test-User")
				next.ServeHTTP(w, req.WithContext(httpx.WithUser(req.Context(), id, "member")))
			})
		})
		r.Route("/groups", func(r chi.Router) { h.Routes(r, nil) })
	})
	return r
}

func call(t *testing.T, srv http.Handler, user, method, path, body string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("X-Test-User", user)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	var out map[string]any
	if rec.Body.Len() > 0 {
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("body bukan JSON object: %v (%s)", err, rec.Body.String())
		}
	}
	return rec.Code, out
}

const createBody = `{"name":"Arisan Kantor","contribution_amount":500000,"period_type":"monthly","slot_count":10,"start_date":"2026-11-01","late_fee":10000}`

func TestHandlerFlow(t *testing.T) {
	srv := newTestServer(1)
	owner, guest := "u-owner", "u-guest"

	// buat grup
	code, g := call(t, srv, owner, http.MethodPost, "/groups", createBody)
	if code != http.StatusCreated {
		t.Fatalf("create: status %d, body %v", code, g)
	}
	gid, invite := g["id"].(string), g["invite_code"].(string)
	if gid == "" || invite == "" {
		t.Fatalf("id/invite_code kosong: %v", g)
	}

	// tamu bergabung -> pending, tanpa invite_code
	code, j := call(t, srv, guest, http.MethodPost, "/groups/join", `{"invite_code":"`+invite+`"}`)
	if code != http.StatusCreated || j["my_status"] != "pending" {
		t.Fatalf("join: %d %v", code, j)
	}
	if _, has := j["invite_code"]; has {
		t.Error("respons join tidak boleh memuat invite_code")
	}

	// tamu belum aktif: daftar anggota terlarang
	if code, _ := call(t, srv, guest, http.MethodGet, "/groups/"+gid+"/members", ""); code != http.StatusForbidden {
		t.Errorf("members oleh pending: %d, want 403", code)
	}

	// pemilik melihat anggota dan menemukan permintaan pending
	code, ms := call(t, srv, owner, http.MethodGet, "/groups/"+gid+"/members", "")
	if code != http.StatusOK {
		t.Fatalf("members: %d", code)
	}
	var pendingID string
	for _, m := range ms["members"].([]any) {
		mm := m.(map[string]any)
		if mm["status"] == "pending" {
			pendingID = mm["id"].(string)
		}
	}
	if pendingID == "" {
		t.Fatal("permintaan pending tidak ditemukan")
	}

	// tamu tidak boleh menyetujui dirinya sendiri
	if code, _ := call(t, srv, guest, http.MethodPost, "/groups/"+gid+"/members/"+pendingID+"/approve", ""); code != http.StatusForbidden {
		t.Errorf("approve oleh non-pemilik: %d, want 403", code)
	}
	// pemilik menyetujui
	if code, j := call(t, srv, owner, http.MethodPost, "/groups/"+gid+"/members/"+pendingID+"/approve", ""); code != http.StatusOK || j["status"] != "active" {
		t.Errorf("approve: %d %v", code, j)
	}
	// tamu kini anggota aktif
	if code, _ := call(t, srv, guest, http.MethodGet, "/groups/"+gid+"/members", ""); code != http.StatusOK {
		t.Errorf("members oleh anggota aktif: %d, want 200", code)
	}

	// daftar grup milik tamu
	code, list := call(t, srv, guest, http.MethodGet, "/groups", "")
	if code != http.StatusOK || len(list["groups"].([]any)) != 1 {
		t.Errorf("list: %d %v", code, list)
	}
}

func TestHandlerErrors(t *testing.T) {
	srv := newTestServer(1)
	_, g := call(t, srv, "u-owner", http.MethodPost, "/groups", createBody)
	gid := g["id"].(string)

	tests := []struct {
		name         string
		user, method string
		path, body   string
		want         int
	}{
		{"create: JSON rusak", "u1", "POST", "/groups", `{`, 400},
		{"create: field asing", "u1", "POST", "/groups", `{"name":"abc","owner_id":"x"}`, 400},
		{"create: validasi gagal", "u1", "POST", "/groups", `{"name":"a"}`, 400},
		{"join: tanpa kode", "u1", "POST", "/groups/join", `{}`, 400},
		{"join: kode salah", "u1", "POST", "/groups/join", `{"invite_code":"NOPE1234"}`, 404},
		{"get: id bukan UUID", "u-owner", "GET", "/groups/abc", "", 404},
		{"get: id SQL injection", "u-owner", "GET", "/groups/1'%20OR%20'1'='1", "", 404},
		{"get: UUID tidak ada", "u-owner", "GET", "/groups/11111111-1111-1111-1111-111111111111", "", 404},
		{"get: orang luar", "stranger", "GET", "/groups/" + gid, "", 404},
		{"approve: member id bukan UUID", "u-owner", "POST", "/groups/" + gid + "/members/xyz/approve", "", 404},
		{"approve: member tidak ada", "u-owner", "POST", "/groups/" + gid + "/members/11111111-1111-1111-1111-111111111111/approve", "", 404},
		{"pemilik join grup sendiri", "u-owner", "POST", "/groups/join", `{"invite_code":"` + g["invite_code"].(string) + `"}`, 409},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			code, body := call(t, srv, tc.user, tc.method, tc.path, tc.body)
			if code != tc.want {
				t.Errorf("status = %d, want %d (%v)", code, tc.want, body)
			}
			if body["error"] == nil {
				t.Error(`respons error seharusnya berisi {"error": "..."}`)
			}
		})
	}
}

func TestHandlerJoinLevelTooLow(t *testing.T) {
	srv := newTestServer(1)
	body := `{"name":"Arisan Elit","contribution_amount":1000000,"period_type":"monthly","slot_count":5,"start_date":"2026-11-01","min_level_rank":3}`
	_, g := call(t, srv, "u-owner", http.MethodPost, "/groups", body)

	code, _ := call(t, srv, "u-newbie", http.MethodPost, "/groups/join", `{"invite_code":"`+g["invite_code"].(string)+`"}`)
	if code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", code)
	}
}

func TestCreateGuard(t *testing.T) {
	h := NewHandler(newTestService(newFakeStore(), 1))
	deny := func(http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { httpx.Error(w, http.StatusForbidden, "akses ditolak") })
	}
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			next.ServeHTTP(w, req.WithContext(httpx.WithUser(req.Context(), "u1", "member")))
		})
	})
	r.Route("/groups", func(r chi.Router) { h.Routes(r, deny) })

	if code, _ := call(t, r, "u1", "POST", "/groups", createBody); code != http.StatusForbidden {
		t.Fatalf("buat grup: %d, mau 403", code)
	}
	// guard hanya untuk pembuatan: daftar grup dan gabung tetap terbuka
	if code, _ := call(t, r, "u1", "GET", "/groups", ""); code != http.StatusOK {
		t.Fatalf("daftar grup: %d, mau 200", code)
	}
}
