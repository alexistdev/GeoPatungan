/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package reputation

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/alexistdev/geopatungan/internal/httpx"
)

func ip(v int) *int       { return &v }
func i64p(v int64) *int64 { return &v }

// defaultLevels meniru seed migrasi.
func defaultLevels() []Level {
	return []Level{
		{Rank: 1, Code: "baru", Name: "Baru", DrawWeight: 1, CanCreateGroup: true, MaxGroupValue: i64p(500_000)},
		{Rank: 2, Code: "terpercaya", Name: "Terpercaya", RequireVerified: true, MinCompletedCycles: 1, MaxLateCount: ip(0), DrawWeight: 2, CanCreateGroup: true, MaxGroupValue: i64p(5_000_000)},
		{Rank: 3, Code: "senior", Name: "Senior", RequireVerified: true, MinCompletedCycles: 3, MaxLateCount: ip(0), DrawWeight: 3, CanCreateGroup: true},
	}
}

func stats(verified bool, cycles, late, overdue int) Stats {
	return Stats{Verified: verified, CompletedCycles: cycles, LatePayments: late, OverdueUnpaid: overdue, LateTotal: late + overdue}
}

// ---- Evaluate --------------------------------------------------------------

func TestEvaluateMatrix(t *testing.T) {
	levels := defaultLevels()
	tests := []struct {
		name string
		st   Stats
		want int
	}{
		{"pengguna baru", stats(false, 0, 0, 0), 1},
		{"hanya terverifikasi", stats(true, 0, 0, 0), 1},
		{"arisan tuntas tetapi belum terverifikasi", stats(false, 5, 0, 0), 1},
		{"terverifikasi + 1 arisan tuntas (syarat level 2 persis)", stats(true, 1, 0, 0), 2},
		{"terverifikasi + 2 arisan tuntas", stats(true, 2, 0, 0), 2},
		{"terverifikasi + 3 arisan tuntas (syarat level 3 persis)", stats(true, 3, 0, 0), 3},
		{"terverifikasi + 10 arisan tuntas", stats(true, 10, 0, 0), 3},
		{"level 2 tetapi pernah terlambat satu kali", stats(true, 1, 1, 0), 1},
		{"level 3 tetapi pernah terlambat satu kali", stats(true, 3, 1, 0), 1},
		{"level 3 tetapi sedang menunggak", stats(true, 3, 0, 1), 1},
		{"terlambat dan menunggak dijumlahkan", stats(true, 3, 1, 1), 1},
		{"tidak terverifikasi walau arisan banyak", stats(false, 9, 0, 0), 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Evaluate(levels, tc.st); got.Rank != tc.want {
				t.Errorf("level = %d, want %d", got.Rank, tc.want)
			}
		})
	}
}

func TestEvaluateRespectsAdminThresholds(t *testing.T) {
	levels := defaultLevels()
	levels[1].MaxLateCount = ip(2) // admin melonggarkan: boleh terlambat sampai 2 kali
	levels[1].MinCompletedCycles = 2
	levels[2].MaxLateCount = ip(0)

	if got := Evaluate(levels, stats(true, 2, 2, 0)); got.Rank != 2 {
		t.Errorf("2 keterlambatan masih boleh di level 2: got %d", got.Rank)
	}
	if got := Evaluate(levels, stats(true, 2, 3, 0)); got.Rank != 1 {
		t.Errorf("3 keterlambatan melebihi batas: got %d", got.Rank)
	}
	if got := Evaluate(levels, stats(true, 1, 0, 0)); got.Rank != 1 {
		t.Errorf("syarat arisan dinaikkan menjadi 2: got %d", got.Rank)
	}

	unlimited := defaultLevels()
	unlimited[1].MaxLateCount = nil // tanpa batas keterlambatan
	if got := Evaluate(unlimited, stats(true, 1, 99, 5)); got.Rank != 2 {
		t.Errorf("tanpa batas keterlambatan: got %d, want 2", got.Rank)
	}
}

func TestEvaluateFallsBackToBaseLevel(t *testing.T) {
	if got := Evaluate(defaultLevels()[:1], stats(true, 99, 0, 0)); got.Rank != 1 {
		t.Errorf("hanya level dasar: got %d", got.Rank)
	}
}

// ---- Progress / BuildProfile -----------------------------------------------

func TestProgressDescribesEachRequirement(t *testing.T) {
	l2 := defaultLevels()[1]

	reqs := Progress(l2, stats(false, 0, 0, 0))
	if len(reqs) != 3 {
		t.Fatalf("len = %d, want 3 syarat (verifikasi, arisan, keterlambatan)", len(reqs))
	}
	by := map[string]Requirement{}
	for _, r := range reqs {
		by[r.Key] = r
	}
	if r := by["verified"]; r.Met || r.Current != 0 || r.Required != 1 {
		t.Errorf("verified = %+v", r)
	}
	if r := by["completed_cycles"]; r.Met || r.Current != 0 || r.Required != 1 || !strings.Contains(r.Label, "1 arisan") {
		t.Errorf("completed_cycles = %+v", r)
	}
	if r := by["late"]; !r.Met || r.Current != 0 || r.Required != 0 || r.Label != "Tanpa keterlambatan" {
		t.Errorf("late = %+v", r)
	}

	reqs = Progress(l2, stats(true, 1, 1, 0))
	for _, r := range reqs {
		want := r.Key != "late"
		if r.Met != want {
			t.Errorf("%s: met = %v, want %v", r.Key, r.Met, want)
		}
	}

	// label menyesuaikan batas keterlambatan
	l2.MaxLateCount = ip(2)
	for _, r := range Progress(l2, stats(true, 1, 3, 0)) {
		if r.Key == "late" && (r.Met || !strings.Contains(r.Label, "Maksimal 2")) {
			t.Errorf("late = %+v", r)
		}
	}
}

func TestProgressForLevelWithoutRequirementsIsEmptySliceNotNull(t *testing.T) {
	reqs := Progress(defaultLevels()[0], stats(false, 0, 0, 0))
	if reqs == nil || len(reqs) != 0 {
		t.Errorf("reqs = %#v, want slice kosong non-nil", reqs)
	}
	body, _ := json.Marshal(reqs)
	if string(body) != "[]" {
		t.Errorf("JSON = %s, want []", body)
	}
}

func TestBuildProfile(t *testing.T) {
	levels := defaultLevels()

	p := BuildProfile(levels, stats(true, 1, 0, 0)) // level 2
	if p.Level.Rank != 2 || p.Next == nil || p.Next.Level.Rank != 3 {
		t.Fatalf("profil = %+v", p)
	}
	var cycles *Requirement
	for i := range p.Next.Requirements {
		if p.Next.Requirements[i].Key == "completed_cycles" {
			cycles = &p.Next.Requirements[i]
		}
	}
	if cycles == nil || cycles.Met || cycles.Current != 1 || cycles.Required != 3 {
		t.Errorf("kemajuan menuju level 3 = %+v", cycles)
	}

	top := BuildProfile(levels, stats(true, 3, 0, 0))
	if top.Level.Rank != 3 || top.Next != nil {
		t.Errorf("level tertinggi tidak punya level berikutnya: %+v", top)
	}

	base := BuildProfile(levels, stats(false, 0, 0, 0))
	if base.Level.Rank != 1 || base.Next == nil || base.Next.Level.Rank != 2 {
		t.Errorf("pengguna baru: %+v", base)
	}
	body, _ := json.Marshal(top)
	if !strings.Contains(string(body), `"next_level":null`) {
		t.Errorf("next_level harus null di level tertinggi: %s", body)
	}
}

// ---- ValidateLevels --------------------------------------------------------

func TestValidateLevelsAcceptsDefaultsAndReasonableChanges(t *testing.T) {
	if err := ValidateLevels(defaultLevels()); err != nil {
		t.Fatalf("seed bawaan harus valid: %v", err)
	}
	ok := map[string]func([]Level){
		"melonggarkan keterlambatan": func(l []Level) { l[1].MaxLateCount = ip(1); l[2].MaxLateCount = ip(0) },
		"menaikkan syarat arisan":    func(l []Level) { l[1].MinCompletedCycles = 2; l[2].MinCompletedCycles = 5 },
		"batas nilai grup level 3":   func(l []Level) { l[2].MaxGroupValue = i64p(10_000_000) },
		"bobot pecahan":              func(l []Level) { l[1].DrawWeight = 1.5; l[2].DrawWeight = 2.5 },
		"level 3 sama dengan level 2": func(l []Level) {
			l[2] = Level{Rank: 3, Code: "senior", Name: "Senior", RequireVerified: true, MinCompletedCycles: 1, MaxLateCount: ip(0), DrawWeight: 2, CanCreateGroup: true, MaxGroupValue: i64p(5_000_000)}
		},
		"level 1 tidak boleh membuat": func(l []Level) { l[0].CanCreateGroup = false },
		"level 1 tanpa batas nilai":   func(l []Level) { l[0].MaxGroupValue = nil; l[1].MaxGroupValue = nil; l[2].MaxGroupValue = nil },
	}
	for name, mutate := range ok {
		t.Run(name, func(t *testing.T) {
			l := defaultLevels()
			mutate(l)
			if err := ValidateLevels(l); err != nil {
				t.Errorf("seharusnya valid: %v", err)
			}
		})
	}
}

func TestValidateLevelsRejectsInconsistentDefinitions(t *testing.T) {
	bad := map[string]func([]Level) []Level{
		"hanya 2 level":                   func(l []Level) []Level { return l[:2] },
		"4 level":                         func(l []Level) []Level { return append(l, Level{Rank: 4}) },
		"kosong":                          func(l []Level) []Level { return nil },
		"rank tidak berurutan":            func(l []Level) []Level { l[1].Rank = 3; return l },
		"nama terlalu pendek":             func(l []Level) []Level { l[0].Name = "B"; return l },
		"nama terlalu panjang":            func(l []Level) []Level { l[0].Name = strings.Repeat("a", 31); return l },
		"kode huruf besar":                func(l []Level) []Level { l[0].Code = "Baru"; return l },
		"kode kembar":                     func(l []Level) []Level { l[1].Code = "baru"; return l },
		"kode kosong":                     func(l []Level) []Level { l[2].Code = ""; return l },
		"bobot di bawah 1":                func(l []Level) []Level { l[0].DrawWeight = 0.5; return l },
		"bobot di atas 3 (melanggar 3:1)": func(l []Level) []Level { l[2].DrawWeight = 3.1; return l },
		"bobot nol":                       func(l []Level) []Level { l[1].DrawWeight = 0; return l },
		"level 1 punya syarat verifikasi": func(l []Level) []Level { l[0].RequireVerified = true; return l },
		"level 1 punya syarat arisan":     func(l []Level) []Level { l[0].MinCompletedCycles = 1; return l },
		"level 1 punya batas telat":       func(l []Level) []Level { l[0].MaxLateCount = ip(0); return l },
		"level 3 tidak butuh verifikasi tetapi level 2 butuh": func(l []Level) []Level { l[2].RequireVerified = false; return l },
		"level 3 syarat arisan lebih rendah":                  func(l []Level) []Level { l[2].MinCompletedCycles = 0; return l },
		"level 3 lebih longgar soal telat":                    func(l []Level) []Level { l[1].MaxLateCount = ip(0); l[2].MaxLateCount = ip(1); return l },
		"level 3 tanpa batas telat padahal level 2 ada":       func(l []Level) []Level { l[2].MaxLateCount = nil; return l },
		"bobot level 3 lebih kecil":                           func(l []Level) []Level { l[2].DrawWeight = 1.5; return l },
		"level 3 kehilangan hak membuat grup":                 func(l []Level) []Level { l[2].CanCreateGroup = false; return l },
		"batas nilai level 3 lebih kecil":                     func(l []Level) []Level { l[2].MaxGroupValue = i64p(1_000_000); return l },
		"level 1 tanpa batas, level 2 berbatas":               func(l []Level) []Level { l[0].MaxGroupValue = nil; return l },
		"batas nilai terlalu kecil":                           func(l []Level) []Level { l[0].MaxGroupValue = i64p(5_000); return l },
		"batas nilai terlalu besar":                           func(l []Level) []Level { l[2].MaxGroupValue = i64p(200_000_000); return l },
		"min arisan negatif":                                  func(l []Level) []Level { l[1].MinCompletedCycles = -1; return l },
		"batas telat negatif":                                 func(l []Level) []Level { l[1].MaxLateCount = ip(-1); return l },
	}
	for name, mutate := range bad {
		t.Run(name, func(t *testing.T) {
			err := ValidateLevels(mutate(defaultLevels()))
			var verr httpx.ValidationError
			if !errors.As(err, &verr) {
				t.Errorf("err = %v, want ValidationError", err)
			}
		})
	}
}

// ---- service dan handler (fake store) --------------------------------------

type fakeStore struct {
	levels  []Level
	stats   Stats
	statErr error
	err     error

	updates int
}

func (f *fakeStore) Levels(context.Context) ([]Level, error) { return f.levels, f.err }
func (f *fakeStore) Stats(context.Context, string) (Stats, error) {
	return f.stats, f.statErr
}
func (f *fakeStore) UpdateLevels(_ context.Context, _ string, levels []Level) error {
	if f.err != nil {
		return f.err
	}
	f.updates++
	f.levels = levels
	return nil
}

func newFake() *fakeStore { return &fakeStore{levels: defaultLevels(), stats: stats(true, 1, 0, 0)} }

func TestServicePolicyMapsLevelToGroupPolicy(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name       string
		st         Stats
		wantRank   int
		wantWeight float64
		wantMax    int64
	}{
		{"level 1", stats(false, 0, 0, 0), 1, 1, 500_000},
		{"level 2", stats(true, 1, 0, 0), 2, 2, 5_000_000},
		{"level 3 tanpa batas nilai", stats(true, 3, 0, 0), 3, 3, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := newFake()
			st.stats = tc.st
			pol, err := NewService(st).Policy(ctx, "u1")
			if err != nil {
				t.Fatal(err)
			}
			if pol.Rank != tc.wantRank || pol.DrawWeight != tc.wantWeight || pol.MaxGroupValue != tc.wantMax || !pol.CanCreateGroup {
				t.Errorf("policy = %+v", pol)
			}
		})
	}

	st := newFake()
	st.statErr = ErrNotFound
	if _, err := NewService(st).Policy(ctx, "ghost"); !errors.Is(err, ErrNotFound) {
		t.Errorf("pengguna tidak ada: err = %v, want ErrNotFound", err)
	}
}

func TestServiceUpdateLevelsValidatesBeforeStore(t *testing.T) {
	st := newFake()
	bad := defaultLevels()
	bad[2].DrawWeight = 9
	if _, err := NewService(st).UpdateLevels(context.Background(), "adm", bad); err == nil {
		t.Fatal("seharusnya ditolak")
	}
	if st.updates != 0 {
		t.Error("definisi tidak valid tidak boleh disimpan")
	}

	good := defaultLevels()
	good[1].MaxLateCount = ip(1)
	good[1].Name = "  Terpercaya  "
	got, err := NewService(st).UpdateLevels(context.Background(), "adm", good)
	if err != nil || st.updates != 1 {
		t.Fatalf("err = %v updates = %d", err, st.updates)
	}
	if got[1].Name != "Terpercaya" || *got[1].MaxLateCount != 1 {
		t.Errorf("hasil = %+v (nama harus dipangkas)", got[1])
	}
}

func serve(st *fakeStore, user, method, path, body string) (*httptest.ResponseRecorder, map[string]any) {
	h := NewHandler(NewService(st))
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			next.ServeHTTP(w, req.WithContext(httpx.WithUser(req.Context(), user, "member")))
		})
	})
	r.Get("/me/reputation", h.Me)
	r.Route("/admin", h.AdminRoutes)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec, out
}

func TestHandlerMe(t *testing.T) {
	rec, body := serve(newFake(), "u1", "GET", "/me/reputation", "")
	if rec.Code != 200 {
		t.Fatalf("status = %d (%s)", rec.Code, rec.Body)
	}
	level := body["level"].(map[string]any)
	if level["rank"].(float64) != 2 || level["code"] != "terpercaya" || level["draw_weight"].(float64) != 2 {
		t.Errorf("level = %v", level)
	}
	st := body["stats"].(map[string]any)
	for _, key := range []string{"kyc_status", "verified", "completed_cycles", "on_time_payments", "late_payments", "overdue_unpaid", "late_total"} {
		if _, ok := st[key]; !ok {
			t.Errorf("stats.%s hilang", key)
		}
	}
	next := body["next_level"].(map[string]any)
	if next["level"].(map[string]any)["rank"].(float64) != 3 || len(next["requirements"].([]any)) == 0 {
		t.Errorf("next_level = %v", next)
	}

	st2 := newFake()
	st2.statErr = ErrNotFound
	if rec, _ := serve(st2, "ghost", "GET", "/me/reputation", ""); rec.Code != http.StatusNotFound {
		t.Errorf("pengguna tidak ada: %d", rec.Code)
	}
	st3 := newFake()
	st3.err = errors.New("pq: koneksi putus rahasia")
	rec, _ = serve(st3, "u1", "GET", "/me/reputation", "")
	if rec.Code != 500 || strings.Contains(rec.Body.String(), "rahasia") {
		t.Errorf("kegagalan database: %d %s", rec.Code, rec.Body)
	}
}

func TestHandlerAdminLevels(t *testing.T) {
	st := newFake()
	rec, body := serve(st, "adm", "GET", "/admin/reputation/levels", "")
	if rec.Code != 200 || len(body["levels"].([]any)) != 3 {
		t.Fatalf("GET: %d %s", rec.Code, rec.Body)
	}
	first := body["levels"].([]any)[0].(map[string]any)
	if first["max_late_count"] != nil || first["max_group_value"].(float64) != 500_000 {
		t.Errorf("nil harus menjadi null di JSON: %v", first)
	}
	third := body["levels"].([]any)[2].(map[string]any)
	if third["max_group_value"] != nil {
		t.Errorf("level 3 tanpa batas nilai harus null: %v", third)
	}

	good, _ := json.Marshal(map[string]any{"levels": defaultLevels()})
	if rec, _ := serve(st, "adm", "PUT", "/admin/reputation/levels", string(good)); rec.Code != 200 || st.updates != 1 {
		t.Errorf("PUT valid: %d %s updates=%d", rec.Code, rec.Body, st.updates)
	}

	badLevels := defaultLevels()
	badLevels[2].DrawWeight = 7
	bad, _ := json.Marshal(map[string]any{"levels": badLevels})
	before := st.updates
	for name, b := range map[string]string{
		"tidak konsisten": string(bad),
		"JSON rusak":      `{`,
		"field asing":     `{"levels":[],"admin":true}`,
		"kosong":          `{"levels":[]}`,
		"tanpa body":      ``,
	} {
		rec, _ := serve(st, "adm", "PUT", "/admin/reputation/levels", b)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400 (%s)", name, rec.Code, rec.Body)
		}
	}
	if st.updates != before {
		t.Error("permintaan tidak valid tidak boleh menyimpan apa pun")
	}
}
