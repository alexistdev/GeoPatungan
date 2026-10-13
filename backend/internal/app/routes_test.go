/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package app

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/alexistdev/geopatungan/internal/auth"
)

// contractDir adalah folder kontrak di akar repositori; dibaca juga oleh tes frontend.
const contractDir = "../../../contract"

func updating() bool { return os.Getenv("UPDATE_CONTRACT") == "1" }

// routeList mengumpulkan "METODE /path" dari router asli, terurut. Bagian dinamis memakai nama
// parameter chi (mis. /api/groups/{id}); frontend menormalkan URL-nya ke bentuk yang sama.
func routeList(t *testing.T) []string {
	t.Helper()
	r := NewRouter(nil, auth.NewTokenManager("x-secret-yang-cukup-panjang-untuk-tes-ini", time.Hour))
	var out []string
	err := chi.Walk(r, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		route = strings.ReplaceAll(route, "/*/", "/") // sisa mount chi
		out = append(out, method+" "+route)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(out)
	return out
}

// Daftar rute yang ter-commit di contract/routes.json harus sama dengan router sebenarnya. Menambah,
// menghapus, atau mengganti nama rute tanpa memperbarui kontrak membuat tes ini gagal, dan tes
// frontend memakai berkas yang sama untuk memastikan setiap URL yang dipanggilnya benar-benar ada.
func TestRouteManifestMatchesRouter(t *testing.T) {
	got := routeList(t)
	path := filepath.Join(contractDir, "routes.json")

	if updating() {
		b, _ := json.MarshalIndent(got, "", "  ")
		if err := os.WriteFile(path, append(b, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("routes.json diperbarui (%d rute)", len(got))
		return
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("contract/routes.json belum ada; jalankan: UPDATE_CONTRACT=1 go test ./internal/app/ (%v)", err)
	}
	var want []string
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatal(err)
	}
	added, removed := diff(got, want), diff(want, got)
	if len(added)+len(removed) > 0 {
		t.Fatalf("daftar rute berubah. Baru: %v. Hilang: %v.\nPerbarui kontrak: UPDATE_CONTRACT=1 go test ./internal/app/ lalu commit contract/routes.json (dan periksa frontend)", added, removed)
	}
}

func diff(a, b []string) []string {
	in := map[string]bool{}
	for _, s := range b {
		in[s] = true
	}
	var out []string
	for _, s := range a {
		if !in[s] {
			out = append(out, s)
		}
	}
	return out
}

func TestRoutesHaveExpectedShape(t *testing.T) {
	routes := routeList(t)
	if len(routes) < 50 {
		t.Fatalf("hanya %d rute; router kemungkinan tidak lengkap", len(routes))
	}
	for _, r := range routes {
		if strings.Contains(r, "/*") {
			t.Errorf("rute berisi wildcard sisa: %s", r)
		}
		if !strings.HasPrefix(r[strings.Index(r, " ")+1:], "/") {
			t.Errorf("rute tidak diawali /: %s", r)
		}
	}
}
