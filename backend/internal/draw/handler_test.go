/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package draw

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/alexistdev/geopatungan/internal/httpx"
)

func newTestServer(fx *fixture) http.Handler {
	h := NewHandler(fx.svc)
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			next.ServeHTTP(w, req.WithContext(httpx.WithUser(req.Context(), req.Header.Get("X-Test-User"), "member")))
		})
	})
	r.Route("/groups", h.Routes)
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

const gidUUID = "11111111-1111-1111-1111-111111111111"

func uuidFixture(t *testing.T, method string) *fixture {
	t.Helper()
	fx := newFixture(t, 4, method, nil)
	// handler menolak ID non-UUID, jadi grup uji memakai ID berformat UUID
	g := fx.groups.groups[fx.gid]
	g.ID = gidUUID
	fx.groups.groups[gidUUID] = g
	fx.groups.members[gidUUID] = fx.groups.members[fx.gid]
	for i := range fx.groups.members[gidUUID] {
		fx.groups.members[gidUUID][i].GroupID = gidUUID
	}
	fx.gid = gidUUID
	return fx
}

func TestHandlerCommitExecuteFlow(t *testing.T) {
	fx := uuidFixture(t, "weighted")
	srv := newTestServer(fx)
	base := "/groups/" + gidUUID + "/draw"

	// belum ada kocokan
	if code, _ := call(t, srv, fx.owner, http.MethodGet, base, ""); code != http.StatusNotFound {
		t.Errorf("GET sebelum commit: %d, want 404", code)
	}

	// commit
	code, c := call(t, srv, fx.owner, http.MethodPost, base+"/commit", "")
	if code != http.StatusCreated {
		t.Fatalf("commit: %d %v", code, c)
	}
	if c["seed_commit"] == "" || c["seed_commit"] == nil {
		t.Error("seed_commit harus ada di respons commit")
	}
	if _, has := c["seed_reveal"]; has {
		t.Error("seed_reveal tidak boleh ada di respons commit")
	}

	// anggota lain bisa melihat komitmen, tapi seed masih tertutup
	code, g := call(t, srv, "user-2", http.MethodGet, base, "")
	if code != http.StatusOK || g["seed_commit"] != c["seed_commit"] {
		t.Errorf("GET anggota: %d %v", code, g)
	}
	if _, has := g["seed_reveal"]; has {
		t.Error("seed_reveal tidak boleh terlihat sebelum execute")
	}

	// bukan pemilik tidak boleh execute
	if code, _ := call(t, srv, "user-2", http.MethodPost, base+"/execute", ""); code != http.StatusForbidden {
		t.Errorf("execute oleh anggota: %d, want 403", code)
	}

	// execute
	code, e := call(t, srv, fx.owner, http.MethodPost, base+"/execute", "")
	if code != http.StatusOK {
		t.Fatalf("execute: %d %v", code, e)
	}
	reveal, _ := e["seed_reveal"].(string)
	if reveal == "" || !VerifyCommit(reveal, e["seed_commit"].(string)) {
		t.Errorf("seed_reveal harus cocok dengan seed_commit: %q", reveal)
	}
	entries := e["entries"].([]any)
	if len(entries) != 4 {
		t.Fatalf("len(entries) = %d, want 4", len(entries))
	}
	for i, x := range entries {
		if int(x.(map[string]any)["turn_order"].(float64)) != i+1 {
			t.Errorf("entri %d tidak berurutan", i)
		}
	}

	// execute lagi
	if code, _ := call(t, srv, fx.owner, http.MethodPost, base+"/execute", ""); code != http.StatusConflict {
		t.Errorf("execute ulang: %d, want 409", code)
	}
	// commit lagi
	if code, _ := call(t, srv, fx.owner, http.MethodPost, base+"/commit", ""); code != http.StatusConflict {
		t.Errorf("commit ulang: %d, want 409", code)
	}
}

func TestHandlerManual(t *testing.T) {
	fx := uuidFixture(t, "manual")
	// handler memeriksa format UUID pada order, jadi member ID juga berformat UUID
	ids := []string{
		"aaaaaaaa-0000-0000-0000-000000000000", "aaaaaaaa-0000-0000-0000-000000000001",
		"aaaaaaaa-0000-0000-0000-000000000002", "aaaaaaaa-0000-0000-0000-000000000003",
	}
	for i := range fx.groups.members[fx.gid] {
		fx.groups.members[fx.gid][i].ID = ids[i]
	}
	srv := newTestServer(fx)
	url := "/groups/" + gidUUID + "/draw/manual"

	if code, _ := call(t, srv, fx.owner, http.MethodPost, url, `{`); code != http.StatusBadRequest {
		t.Errorf("JSON rusak: %d, want 400", code)
	}
	if code, _ := call(t, srv, fx.owner, http.MethodPost, url, `{"order":["bukan-uuid"]}`); code != http.StatusBadRequest {
		t.Errorf("order bukan UUID: %d, want 400", code)
	}
	if code, _ := call(t, srv, fx.owner, http.MethodPost, url, `{"order":["`+ids[0]+`"]}`); code != http.StatusBadRequest {
		t.Errorf("order tidak lengkap: %d, want 400", code)
	}
	if code, _ := call(t, srv, "user-1", http.MethodPost, url, `{"order":["`+strings.Join(ids, `","`)+`"]}`); code != http.StatusForbidden {
		t.Errorf("bukan pemilik: %d, want 403", code)
	}

	body := `{"order":["` + ids[3] + `","` + ids[1] + `","` + ids[0] + `","` + ids[2] + `"]}`
	code, d := call(t, srv, fx.owner, http.MethodPost, url, body)
	if code != http.StatusCreated {
		t.Fatalf("manual: %d %v", code, d)
	}
	first := d["entries"].([]any)[0].(map[string]any)
	if first["member_id"] != ids[3] {
		t.Errorf("giliran pertama = %v, want %s", first["member_id"], ids[3])
	}
	if code, _ := call(t, srv, fx.owner, http.MethodPost, url, body); code != http.StatusConflict {
		t.Errorf("manual ulang: %d, want 409", code)
	}
}

func TestHandlerErrors(t *testing.T) {
	fx := uuidFixture(t, "random")
	srv := newTestServer(fx)

	tests := []struct {
		name, user, method, path, body string
		want                           int
	}{
		{"id bukan UUID (get)", fx.owner, "GET", "/groups/abc/draw", "", 404},
		{"id bukan UUID (commit)", fx.owner, "POST", "/groups/abc/draw/commit", "", 404},
		{"id SQL injection", fx.owner, "POST", "/groups/1'%20OR%20'1'='1/draw/execute", "", 404},
		{"grup tidak ada", fx.owner, "POST", "/groups/22222222-2222-2222-2222-222222222222/draw/commit", "", 404},
		{"orang luar commit", "stranger", "POST", "/groups/" + gidUUID + "/draw/commit", "", 404},
		{"anggota commit", "user-1", "POST", "/groups/" + gidUUID + "/draw/commit", "", 403},
		{"execute tanpa commit", fx.owner, "POST", "/groups/" + gidUUID + "/draw/execute", "", 404},
		{"manual di grup random", fx.owner, "POST", "/groups/" + gidUUID + "/draw/manual", `{"order":[]}`, 409},
		{"manual field asing", fx.owner, "POST", "/groups/" + gidUUID + "/draw/manual", `{"order":[],"x":1}`, 400},
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
