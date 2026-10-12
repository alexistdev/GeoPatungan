/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package referral

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/alexistdev/geopatungan/internal/httpx"
)

func defaultRules() []Rule {
	return []Rule{{1, 4000}, {2, 2500}, {3, 1500}, {4, 1000}}
}

func fullPlan() []Beneficiary {
	return []Beneficiary{{1, "d", 4000}, {2, "c", 2500}, {3, "b", 1500}, {4, "a", 1000}}
}

// ---- Amounts ---------------------------------------------------------------

func TestAmounts(t *testing.T) {
	tests := []struct {
		name string
		plan []Beneficiary
		fee  int64
		want []int64 // per level, urut plan
	}{
		{"fee bulat", fullPlan(), 10_000, []int64{4000, 2500, 1500, 1000}},
		{"dibulatkan ke bawah per level", fullPlan(), 999, []int64{399, 249, 149, 99}},
		{"fee kecil: bagian nol dibuang", fullPlan(), 5, []int64{2, 1}},
		{"fee 1: tidak ada yang kebagian", fullPlan(), 1, nil},
		{"fee 0", fullPlan(), 0, nil},
		{"fee negatif", fullPlan(), -100, nil},
		{"tanpa penerima", nil, 10_000, nil},
		{"hanya level 2 dan 4 yang berhak", []Beneficiary{{2, "c", 2500}, {4, "a", 1000}}, 10_000, []int64{2500, 1000}},
		{"bagian 100% di satu level", []Beneficiary{{1, "d", 10000}}, 777, []int64{777}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Amounts(tc.plan, tc.fee)
			if len(got) != len(tc.want) {
				t.Fatalf("len = %d (%+v), want %d", len(got), got, len(tc.want))
			}
			for i, w := range tc.want {
				if got[i].Amount != w {
					t.Errorf("komisi ke-%d = %d, want %d", i, got[i].Amount, w)
				}
			}
			if tc.fee > 0 && Total(got) > tc.fee {
				t.Errorf("total komisi %d melebihi biaya %d", Total(got), tc.fee)
			}
		})
	}
}

func TestAmountsNeverExceedFeeEvenWithBadRules(t *testing.T) {
	// pagar tambahan: walau aturan salah (total 200%), komisi tidak pernah melebihi biaya
	plan := []Beneficiary{{1, "d", 10000}, {2, "c", 10000}}
	got := Amounts(plan, 1_000)
	if Total(got) != 1_000 || len(got) != 1 || got[0].UserID != "d" {
		t.Errorf("got = %+v, want seluruh biaya ke level 1 saja", got)
	}
	// untuk berbagai biaya, total tidak pernah melebihi biaya
	for fee := int64(1); fee < 3000; fee += 7 {
		if tot := Total(Amounts(fullPlan(), fee)); tot > fee {
			t.Fatalf("fee %d: total komisi %d melebihi biaya", fee, tot)
		}
	}
}

func TestAmountsCarryContext(t *testing.T) {
	got := Amounts(fullPlan(), 10_000)
	if got[1].Level != 2 || got[1].UserID != "c" || got[1].ShareBps != 2500 {
		t.Errorf("konteks komisi hilang: %+v", got[1])
	}
}

// ---- ValidateRules ---------------------------------------------------------

func TestValidateRules(t *testing.T) {
	ok := map[string][]Rule{
		"bawaan (90%)":  defaultRules(),
		"tepat 100%":    {{1, 5000}, {2, 3000}, {3, 1500}, {4, 500}},
		"semua nol":     {{1, 0}, {2, 0}, {3, 0}, {4, 0}},
		"hanya level 1": {{1, 10000}, {2, 0}, {3, 0}, {4, 0}},
		"tidak menurun": {{1, 1000}, {2, 2000}, {3, 3000}, {4, 4000}},
	}
	for name, r := range ok {
		if err := ValidateRules(r); err != nil {
			t.Errorf("%s: seharusnya valid: %v", name, err)
		}
	}
	bad := map[string][]Rule{
		"total 100,01%":         {{1, 5000}, {2, 3000}, {3, 1500}, {4, 501}},
		"total 200%":            {{1, 10000}, {2, 10000}, {3, 0}, {4, 0}},
		"satu level > 100%":     {{1, 10001}, {2, 0}, {3, 0}, {4, 0}},
		"negatif":               {{1, -1}, {2, 0}, {3, 0}, {4, 0}},
		"3 aturan":              {{1, 100}, {2, 100}, {3, 100}},
		"5 aturan":              {{1, 100}, {2, 100}, {3, 100}, {4, 100}, {5, 100}},
		"kosong":                nil,
		"level tidak berurutan": {{1, 100}, {3, 100}, {2, 100}, {4, 100}},
		"level dobel":           {{1, 100}, {1, 100}, {3, 100}, {4, 100}},
		"level mulai dari 0":    {{0, 100}, {1, 100}, {2, 100}, {3, 100}},
	}
	for name, r := range bad {
		err := ValidateRules(r)
		var verr httpx.ValidationError
		if !errors.As(err, &verr) {
			t.Errorf("%s: err = %v, want ValidationError", name, err)
		}
	}
	if err := ValidateRules([]Rule{{1, 5000}, {2, 3000}, {3, 1500}, {4, 501}}); err == nil || !strings.Contains(err.Error(), "100,0") && !strings.Contains(err.Error(), "100.01") {
		t.Errorf("pesan harus menyebut total yang melebihi: %v", err)
	}
}

// ---- maskName --------------------------------------------------------------

func TestMaskName(t *testing.T) {
	for in, want := range map[string]string{
		"Budi Santoso":       "B*** S***",
		"Budi":               "B***",
		"  Budi   Santoso  ": "B*** S***",
		"Ani Budi Cici":      "A*** B*** C***",
		"Éric Müller":        "É*** M***",
		"":                   "***",
		"   ":                "***",
	} {
		if got := maskName(in); got != want {
			t.Errorf("maskName(%q) = %q, want %q", in, got, want)
		}
	}
	// tidak ada huruf selain yang pertama yang bocor
	if got := maskName("Rahasia Sekali"); strings.Contains(got, "ahasia") || strings.Contains(got, "ekali") {
		t.Errorf("nama bocor: %q", got)
	}
}

// ---- service (fake store) --------------------------------------------------

type fakeStore struct {
	rules   []Rule
	summary *Summary
	raws    []rawEntry
	err     error

	updates int
	gotLim  int
	gotBef  int64
}

func (f *fakeStore) Rules(context.Context) ([]Rule, error) { return f.rules, f.err }
func (f *fakeStore) UpdateRules(_ context.Context, _ string, r []Rule) error {
	if f.err != nil {
		return f.err
	}
	f.updates++
	f.rules = r
	return nil
}
func (f *fakeStore) Summary(context.Context, string) (*Summary, error) { return f.summary, f.err }
func (f *fakeStore) Commissions(_ context.Context, _ string, limit int, before int64) ([]rawEntry, error) {
	f.gotLim, f.gotBef = limit, before
	if f.err != nil {
		return nil, f.err
	}
	out := []rawEntry{}
	for _, e := range f.raws {
		if before == 0 || e.ID < before {
			out = append(out, e)
		}
		if len(out) == limit+1 {
			break
		}
	}
	return out, nil
}

func raws(n int) []rawEntry {
	out := make([]rawEntry, n)
	for i := range out {
		out[i] = rawEntry{ID: int64(n - i), Level: 1, ShareBps: 4000, Amount: 800, SourceName: "Budi Santoso", CreatedAt: time.Now()}
	}
	return out
}

func newFake() *fakeStore {
	return &fakeStore{
		rules: defaultRules(),
		summary: &Summary{ReferralCode: "ABCD2345", Eligible: true, Rules: defaultRules(),
			Downline: DownlineStats{Level1: 2, Level2: 1, Total: 3},
			Earned:   EarnedStats{Total: 1300, ByLevel: map[string]int64{"1": 800, "2": 500, "3": 0, "4": 0}}},
	}
}

func TestServiceCommissionsPagingAndMasking(t *testing.T) {
	ctx := context.Background()
	st := newFake()
	st.raws = raws(10)
	svc := NewService(st)

	page, err := svc.Commissions(ctx, "u1", 4, 0)
	if err != nil || len(page.Commissions) != 4 || page.NextBefore == nil || *page.NextBefore != page.Commissions[3].ID {
		t.Fatalf("page = %+v, %v", page, err)
	}
	for _, c := range page.Commissions {
		if c.SourceName != "B*** S***" {
			t.Errorf("nama sumber harus disamarkan: %q", c.SourceName)
		}
	}

	seen := map[int64]bool{}
	var before int64
	for i := 0; i < 20; i++ {
		p, err := svc.Commissions(ctx, "u1", 3, before)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range p.Commissions {
			if seen[c.ID] {
				t.Fatalf("entri %d ganda", c.ID)
			}
			seen[c.ID] = true
		}
		if p.NextBefore == nil {
			break
		}
		before = *p.NextBefore
	}
	if len(seen) != 10 {
		t.Errorf("terbaca %d entri, want 10", len(seen))
	}

	// tepat limit: tidak ada halaman berikutnya; kosong: slice non-nil
	st.raws = raws(4)
	if p, _ := svc.Commissions(ctx, "u1", 4, 0); len(p.Commissions) != 4 || p.NextBefore != nil {
		t.Errorf("tepat limit: %+v", p)
	}
	st.raws = nil
	if p, _ := svc.Commissions(ctx, "u1", 5, 0); p.Commissions == nil || len(p.Commissions) != 0 || p.NextBefore != nil {
		t.Errorf("kosong: %+v", p)
	}
}

func TestServiceCommissionsValidatesParams(t *testing.T) {
	for _, tc := range []struct {
		limit  int
		before int64
		bad    bool
	}{{0, 0, false}, {1, 0, false}, {100, 0, false}, {101, 0, true}, {-1, 0, true}, {10, -1, true}} {
		st := newFake()
		_, err := NewService(st).Commissions(context.Background(), "u1", tc.limit, tc.before)
		var verr httpx.ValidationError
		if tc.bad != errors.As(err, &verr) {
			t.Errorf("limit %d before %d: err = %v, bad = %v", tc.limit, tc.before, err, tc.bad)
		}
		if tc.bad && st.gotLim != 0 {
			t.Errorf("input tidak valid tidak boleh sampai ke store")
		}
	}
	st := newFake()
	NewService(st).Commissions(context.Background(), "u1", 0, 0)
	if st.gotLim != DefaultListLimit {
		t.Errorf("limit bawaan = %d, want %d", st.gotLim, DefaultListLimit)
	}
}

func TestServiceUpdateRulesValidatesBeforeStore(t *testing.T) {
	st := newFake()
	bad := []Rule{{1, 9000}, {2, 9000}, {3, 0}, {4, 0}}
	if _, err := NewService(st).UpdateRules(context.Background(), "adm", bad); err == nil {
		t.Fatal("seharusnya ditolak")
	}
	if st.updates != 0 {
		t.Error("aturan tidak valid tidak boleh disimpan")
	}
	good := []Rule{{1, 3000}, {2, 2000}, {3, 1000}, {4, 500}}
	got, err := NewService(st).UpdateRules(context.Background(), "adm", good)
	if err != nil || st.updates != 1 || got[0].ShareBps != 3000 {
		t.Errorf("got = %+v, %v, updates = %d", got, err, st.updates)
	}
}

// ---- handler ---------------------------------------------------------------

func serve(st *fakeStore, user, method, path, body string) (*httptest.ResponseRecorder, map[string]any) {
	h := NewHandler(NewService(st))
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			next.ServeHTTP(w, req.WithContext(httpx.WithUser(req.Context(), user, "member")))
		})
	})
	r.Route("/me/referral", h.Routes)
	r.Route("/admin", h.AdminRoutes)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec, out
}

func TestHandlerSummary(t *testing.T) {
	rec, body := serve(newFake(), "u1", "GET", "/me/referral/", "")
	if rec.Code != 200 {
		t.Fatalf("status = %d (%s)", rec.Code, rec.Body)
	}
	if body["referral_code"] != "ABCD2345" || body["eligible"] != true {
		t.Errorf("body = %v", body)
	}
	down := body["downline"].(map[string]any)
	if down["level_1"].(float64) != 2 || down["total"].(float64) != 3 {
		t.Errorf("downline = %v", down)
	}
	earned := body["earned"].(map[string]any)
	if earned["total"].(float64) != 1300 || earned["by_level"].(map[string]any)["2"].(float64) != 500 {
		t.Errorf("earned = %v", earned)
	}
	if len(body["rules"].([]any)) != 4 {
		t.Errorf("rules = %v", body["rules"])
	}

	st := newFake()
	st.err = ErrNotFound
	if rec, _ := serve(st, "ghost", "GET", "/me/referral/", ""); rec.Code != http.StatusNotFound {
		t.Errorf("pengguna tidak ada: %d", rec.Code)
	}
	st.err = errors.New("pq: koneksi putus rahasia")
	if rec, _ := serve(st, "u1", "GET", "/me/referral/", ""); rec.Code != 500 || strings.Contains(rec.Body.String(), "rahasia") {
		t.Errorf("kegagalan database: %d %s", rec.Code, rec.Body)
	}
}

func TestHandlerCommissions(t *testing.T) {
	st := newFake()
	st.raws = raws(5)
	rec, body := serve(st, "u1", "GET", "/me/referral/commissions?limit=2&before=5", "")
	if rec.Code != 200 || st.gotLim != 2 || st.gotBef != 5 {
		t.Fatalf("status=%d limit=%d before=%d (%s)", rec.Code, st.gotLim, st.gotBef, rec.Body)
	}
	first := body["commissions"].([]any)[0].(map[string]any)
	for _, key := range []string{"id", "level", "share_bps", "amount", "source_name", "created_at"} {
		if _, ok := first[key]; !ok {
			t.Errorf("field %q hilang", key)
		}
	}
	if first["source_name"] != "B*** S***" {
		t.Errorf("source_name = %v", first["source_name"])
	}
	if strings.Contains(rec.Body.String(), "Santoso") {
		t.Error("nama downline bocor")
	}

	for name, path := range map[string]string{
		"limit huruf": "/me/referral/commissions?limit=abc", "before huruf": "/me/referral/commissions?before=x",
		"limit besar": "/me/referral/commissions?limit=500", "before negatif": "/me/referral/commissions?before=-1",
		"sisipan SQL": "/me/referral/commissions?before=1%20OR%201=1",
	} {
		if rec, _ := serve(newFake(), "u1", "GET", path, ""); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", name, rec.Code)
		}
	}
}

func TestHandlerAdminRules(t *testing.T) {
	st := newFake()
	rec, body := serve(st, "adm", "GET", "/admin/referral/rules", "")
	if rec.Code != 200 || len(body["rules"].([]any)) != 4 {
		t.Fatalf("GET: %d %s", rec.Code, rec.Body)
	}
	good, _ := json.Marshal(map[string]any{"rules": []Rule{{1, 3000}, {2, 2000}, {3, 1000}, {4, 500}}})
	if rec, _ := serve(st, "adm", "PUT", "/admin/referral/rules", string(good)); rec.Code != 200 || st.updates != 1 {
		t.Errorf("PUT valid: %d %s", rec.Code, rec.Body)
	}
	over, _ := json.Marshal(map[string]any{"rules": []Rule{{1, 9000}, {2, 9000}, {3, 0}, {4, 0}}})
	before := st.updates
	for name, b := range map[string]string{
		"total melebihi 100%": string(over), "JSON rusak": `{`, "field asing": `{"rules":[],"x":1}`,
		"kosong": `{"rules":[]}`, "tanpa body": ``,
	} {
		if rec, _ := serve(st, "adm", "PUT", "/admin/referral/rules", b); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400 (%s)", name, rec.Code, rec.Body)
		}
	}
	if st.updates != before {
		t.Error("permintaan tidak valid tidak boleh menyimpan apa pun")
	}
}
