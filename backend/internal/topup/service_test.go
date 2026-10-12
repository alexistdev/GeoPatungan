/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package topup

import (
	"bytes"
	"context"
	crand "crypto/rand"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/alexistdev/geopatungan/internal/httpx"
	"github.com/alexistdev/geopatungan/internal/settings"
)

// ---- fakes -----------------------------------------------------------------

type fakeSettings struct {
	cfg        settings.Settings
	banks      []settings.BankAccount
	err        error
	onlyActive bool
}

func (f *fakeSettings) Get(context.Context) (settings.Settings, error) { return f.cfg, f.err }

func (f *fakeSettings) ListBanks(_ context.Context, onlyActive bool) ([]settings.BankAccount, error) {
	f.onlyActive = onlyActive
	if f.err != nil {
		return nil, f.err
	}
	out := []settings.BankAccount{}
	for _, b := range f.banks {
		if !onlyActive || b.IsActive {
			out = append(out, b)
		}
	}
	return out, nil
}

type call struct {
	op         string
	user       string
	id         string
	amount     int64
	codes      []int
	hours      int
	maxOpen    int
	received   *int64
	note       string
	name, bank string
	status     string
	limit      int
}

type fakeStore struct {
	topup *Topup
	list  []Topup
	admin []AdminTopup
	err   error
	calls []call
}

func (f *fakeStore) rec(c call) { f.calls = append(f.calls, c) }

func (f *fakeStore) Create(_ context.Context, userID string, amount int64, codes []int, hours, maxOpen int) (*Topup, error) {
	f.rec(call{op: "create", user: userID, amount: amount, codes: codes, hours: hours, maxOpen: maxOpen})
	return f.topup, f.err
}
func (f *fakeStore) Get(_ context.Context, userID, id string) (*Topup, error) {
	f.rec(call{op: "get", user: userID, id: id})
	return f.topup, f.err
}
func (f *fakeStore) ListByUser(_ context.Context, userID string, limit int) ([]Topup, error) {
	f.rec(call{op: "list", user: userID, limit: limit})
	return f.list, f.err
}
func (f *fakeStore) Confirm(_ context.Context, userID, id, name, bank, note string) (*Topup, error) {
	f.rec(call{op: "confirm", user: userID, id: id, name: name, bank: bank, note: note})
	return f.topup, f.err
}
func (f *fakeStore) Cancel(_ context.Context, userID, id string) (*Topup, error) {
	f.rec(call{op: "cancel", user: userID, id: id})
	return f.topup, f.err
}
func (f *fakeStore) ListAdmin(_ context.Context, status string, limit int) ([]AdminTopup, error) {
	f.rec(call{op: "list_admin", status: status, limit: limit})
	return f.admin, f.err
}
func (f *fakeStore) Approve(_ context.Context, adminID, id string, received *int64, note string) (*Topup, error) {
	f.rec(call{op: "approve", user: adminID, id: id, received: received, note: note})
	return f.topup, f.err
}
func (f *fakeStore) Reject(_ context.Context, adminID, id, note string) (*Topup, error) {
	f.rec(call{op: "reject", user: adminID, id: id, note: note})
	return f.topup, f.err
}

func newFixture() (*Service, *fakeStore, *fakeSettings) {
	st := &fakeStore{topup: &Topup{ID: tid, UserID: "u1", Amount: 100_000, UniqueCode: 347, TransferAmount: 100_347, Status: StatusPending}}
	cfg := &fakeSettings{
		cfg:   settings.Defaults(),
		banks: []settings.BankAccount{{ID: "b1", BankName: "BCA", AccountNumber: "1234567890", AccountHolder: "PT Geo", IsActive: true}},
	}
	return NewService(st, cfg), st, cfg
}

const (
	tid = "11111111-1111-1111-1111-111111111111"
	uid = "u1"
)

// ---- pembantu murni --------------------------------------------------------

func TestRupiah(t *testing.T) {
	for n, want := range map[int64]string{0: "0", 7: "7", 999: "999", 1_000: "1.000", 10_000: "10.000",
		100_000_000: "100.000.000", 1_234_567: "1.234.567", 1_000_000_000: "1.000.000.000"} {
		if got := rupiah(n); got != want {
			t.Errorf("rupiah(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestRandomCodes(t *testing.T) {
	for _, n := range []int{1, 25, 500, 999} {
		codes, err := randomCodes(bytesReader(), n)
		if err != nil {
			t.Fatalf("n=%d: %v", n, err)
		}
		if len(codes) != n {
			t.Fatalf("len = %d, want %d", len(codes), n)
		}
		seen := map[int]bool{}
		for _, c := range codes {
			if c < 1 || c > 999 {
				t.Fatalf("kode %d di luar 1..999", c)
			}
			if seen[c] {
				t.Fatalf("kode %d ganda", c)
			}
			seen[c] = true
		}
	}
	// n = 999 harus memuat seluruh rentang tepat sekali
	all, _ := randomCodes(bytesReader(), 999)
	sum := 0
	for _, c := range all {
		sum += c
	}
	if sum != 999*1000/2 {
		t.Errorf("jumlah kode = %d, want %d (semua 1..999 tepat sekali)", sum, 999*1000/2)
	}

	for _, n := range []int{0, -1, 1000} {
		if _, err := randomCodes(bytesReader(), n); err == nil {
			t.Errorf("n=%d seharusnya ditolak", n)
		}
	}
	if _, err := randomCodes(strings.NewReader("pendek"), 25); err == nil {
		t.Error("sumber acak yang habis seharusnya menghasilkan error, bukan kode palsu")
	}
	// dua pemanggilan menghasilkan urutan berbeda (acak sungguhan)
	a, _ := randomCodes(bytesReader(), 25)
	b, _ := randomCodes(bytesReader(), 25)
	same := true
	for i := range a {
		if a[i] != b[i] {
			same = false
		}
	}
	if same {
		t.Error("dua urutan kode identik; sumber acak seharusnya berbeda tiap pemanggilan")
	}
}

// ---- Create ----------------------------------------------------------------

func TestServiceCreate(t *testing.T) {
	svc, st, cfg := newFixture()
	cfg.cfg.TopupExpiryHours = 36

	d, err := svc.Create(context.Background(), uid, 100_000)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if d.Topup == nil || len(d.BankAccounts) != 1 || d.BankAccounts[0].BankName != "BCA" {
		t.Fatalf("detail = %+v", d)
	}
	if !cfg.onlyActive {
		t.Error("harus meminta hanya rekening aktif")
	}

	c := st.calls[0]
	if c.op != "create" || c.user != uid || c.amount != 100_000 || c.hours != 36 || c.maxOpen != MaxOpenPerUser {
		t.Errorf("parameter ke store = %+v", c)
	}
	if len(c.codes) != codeCandidates {
		t.Errorf("kandidat kode = %d, want %d", len(c.codes), codeCandidates)
	}
	seen := map[int]bool{}
	for _, code := range c.codes {
		if code < 1 || code > 999 || seen[code] {
			t.Errorf("kandidat kode tidak valid: %d", code)
		}
		seen[code] = true
	}
}

func TestServiceCreateAmountBounds(t *testing.T) {
	for _, tc := range []struct {
		amount  int64
		wantErr bool
	}{
		{9_999, true}, {10_000, false}, {10_001, false}, {100_000_000, false}, {100_000_001, true},
		{0, true}, {-50_000, true},
	} {
		svc, st, _ := newFixture()
		_, err := svc.Create(context.Background(), uid, tc.amount)
		var verr httpx.ValidationError
		if tc.wantErr != errors.As(err, &verr) {
			t.Errorf("amount %d: err = %v, wantErr = %v", tc.amount, err, tc.wantErr)
		}
		if tc.wantErr && len(st.calls) != 0 {
			t.Errorf("amount %d: nominal tidak valid tidak boleh sampai ke store", tc.amount)
		}
	}

	// pesan memuat batas dengan titik ribuan dan mengikuti pengaturan admin
	svc, _, cfg := newFixture()
	cfg.cfg.TopupMinAmount, cfg.cfg.TopupMaxAmount = 50_000, 2_500_000
	_, err := svc.Create(context.Background(), uid, 1)
	if err == nil || !strings.Contains(err.Error(), "Rp50.000") || !strings.Contains(err.Error(), "Rp2.500.000") {
		t.Errorf("pesan = %v, want memuat Rp50.000 dan Rp2.500.000", err)
	}
	if _, err := svc.Create(context.Background(), uid, 2_500_000); err != nil {
		t.Errorf("batas atas pengaturan admin seharusnya diterima: %v", err)
	}
}

func TestServiceCreateWithoutActiveBank(t *testing.T) {
	svc, st, cfg := newFixture()
	cfg.banks[0].IsActive = false
	if _, err := svc.Create(context.Background(), uid, 100_000); !errors.Is(err, ErrNoBankAccount) {
		t.Errorf("err = %v, want ErrNoBankAccount", err)
	}
	if len(st.calls) != 0 {
		t.Error("tanpa rekening tujuan, permintaan tidak boleh dibuat (pengguna tidak punya tempat mentransfer)")
	}
	cfg.banks = nil
	if _, err := svc.Create(context.Background(), uid, 100_000); !errors.Is(err, ErrNoBankAccount) {
		t.Errorf("tanpa rekening sama sekali: err = %v", err)
	}
}

func TestServiceCreatePropagatesErrors(t *testing.T) {
	svc, st, _ := newFixture()
	st.err = ErrTooManyOpen
	if _, err := svc.Create(context.Background(), uid, 100_000); !errors.Is(err, ErrTooManyOpen) {
		t.Errorf("err = %v, want ErrTooManyOpen", err)
	}
	svc, _, cfg := newFixture()
	cfg.err = errors.New("db mati")
	if _, err := svc.Create(context.Background(), uid, 100_000); err == nil {
		t.Error("kegagalan pengaturan seharusnya menggagalkan pembuatan")
	}
}

// ---- Get -------------------------------------------------------------------

func TestServiceGetShowsBanksOnlyWhileOpen(t *testing.T) {
	for status, wantBanks := range map[string]bool{
		StatusPending: true, StatusSubmitted: true,
		StatusApproved: false, StatusRejected: false, StatusCancelled: false, StatusExpired: false,
	} {
		svc, st, _ := newFixture()
		st.topup.Status = status
		d, err := svc.Get(context.Background(), uid, tid)
		if err != nil {
			t.Fatal(err)
		}
		if got := len(d.BankAccounts) > 0; got != wantBanks {
			t.Errorf("status %s: rekening tampil = %v, want %v", status, got, wantBanks)
		}
		if d.BankAccounts == nil {
			t.Errorf("status %s: BankAccounts harus [] bukan null", status)
		}
	}
}

// ---- Confirm ---------------------------------------------------------------

func TestServiceConfirmValidation(t *testing.T) {
	good := func() (string, string, string) { return "Budi Santoso", "BCA", "transfer pukul 10" }

	svc, st, _ := newFixture()
	n, b, note := good()
	if _, err := svc.Confirm(context.Background(), uid, tid, "  "+n+"  ", " "+b+" ", " "+note+" "); err != nil {
		t.Fatal(err)
	}
	if c := st.calls[0]; c.name != n || c.bank != b || c.note != note {
		t.Errorf("input tidak dipangkas: %+v", c)
	}

	bad := map[string][3]string{
		"nama kosong":      {"", "BCA", ""},
		"nama 1 huruf":     {"B", "BCA", ""},
		"nama 101 huruf":   {strings.Repeat("a", 101), "BCA", ""},
		"nama hanya spasi": {"     ", "BCA", ""},
		"bank kosong":      {"Budi", "", ""},
		"bank 1 huruf":     {"Budi", "B", ""},
		"bank 51 huruf":    {"Budi", strings.Repeat("a", 51), ""},
		"catatan 201":      {"Budi", "BCA", strings.Repeat("a", 201)},
	}
	for name, v := range bad {
		t.Run(name, func(t *testing.T) {
			svc, st, _ := newFixture()
			_, err := svc.Confirm(context.Background(), uid, tid, v[0], v[1], v[2])
			var verr httpx.ValidationError
			if !errors.As(err, &verr) || len(st.calls) != 0 {
				t.Errorf("err = %v calls = %d, want ValidationError tanpa menyentuh store", err, len(st.calls))
			}
		})
	}
	// batas tepat
	svc, _, _ = newFixture()
	if _, err := svc.Confirm(context.Background(), uid, tid, strings.Repeat("a", 100), strings.Repeat("b", 50), strings.Repeat("c", 200)); err != nil {
		t.Errorf("batas tepat seharusnya diterima: %v", err)
	}
}

// ---- Approve / Reject ------------------------------------------------------

func ptr(v int64) *int64 { return &v }

func TestServiceApproveValidation(t *testing.T) {
	for name, tc := range map[string]struct {
		received *int64
		note     string
		wantErr  bool
	}{
		"tanpa penyesuaian":    {nil, "", false},
		"penyesuaian 1":        {ptr(1), "", false},
		"penyesuaian batas":    {ptr(1_000_000_000), "", false},
		"nol":                  {ptr(0), "", true},
		"negatif":              {ptr(-5), "", true},
		"melebihi batas keras": {ptr(1_000_000_001), "", true},
		"catatan 200":          {nil, strings.Repeat("a", 200), false},
		"catatan 201":          {nil, strings.Repeat("a", 201), true},
	} {
		t.Run(name, func(t *testing.T) {
			svc, st, _ := newFixture()
			_, err := svc.Approve(context.Background(), "admin", tid, tc.received, tc.note)
			var verr httpx.ValidationError
			if tc.wantErr != errors.As(err, &verr) {
				t.Errorf("err = %v, wantErr = %v", err, tc.wantErr)
			}
			if tc.wantErr && len(st.calls) != 0 {
				t.Error("input tidak valid tidak boleh sampai ke store")
			}
		})
	}
}

func TestServiceRejectRequiresReason(t *testing.T) {
	for name, tc := range map[string]struct {
		note    string
		wantErr bool
	}{
		"kosong": {"", true}, "spasi": {"    ", true}, "2 huruf": {"ab", true},
		"3 huruf": {"abc", false}, "200 huruf": {strings.Repeat("a", 200), false}, "201 huruf": {strings.Repeat("a", 201), true},
	} {
		t.Run(name, func(t *testing.T) {
			svc, st, _ := newFixture()
			_, err := svc.Reject(context.Background(), "admin", tid, tc.note)
			var verr httpx.ValidationError
			if tc.wantErr != errors.As(err, &verr) {
				t.Errorf("err = %v, wantErr = %v", err, tc.wantErr)
			}
			if tc.wantErr && len(st.calls) != 0 {
				t.Error("tanpa alasan, penolakan tidak boleh sampai ke store")
			}
		})
	}
}

func TestServiceListAdminValidation(t *testing.T) {
	svc, st, _ := newFixture()
	if _, err := svc.ListAdmin(context.Background(), "", 0); err != nil {
		t.Fatal(err)
	}
	if c := st.calls[0]; c.status != "open" || c.limit != DefaultListLimit {
		t.Errorf("bawaan = %+v, want open dan %d", c, DefaultListLimit)
	}
	for _, s := range []string{"open", "all", "pending", "submitted", "approved", "rejected", "cancelled", "expired"} {
		if _, err := NewService(&fakeStore{}, &fakeSettings{}).ListAdmin(context.Background(), s, 10); err != nil {
			t.Errorf("status %q seharusnya valid: %v", s, err)
		}
	}
	for _, s := range []string{"OPEN", "done", "pending;DROP", "approved "} {
		svc, st, _ := newFixture()
		_, err := svc.ListAdmin(context.Background(), s, 10)
		var verr httpx.ValidationError
		if !errors.As(err, &verr) || len(st.calls) != 0 {
			t.Errorf("status %q: err = %v", s, err)
		}
	}
	for _, l := range []int{-1, 201} {
		if _, err := svc.ListAdmin(context.Background(), "all", l); err == nil {
			t.Errorf("limit %d seharusnya ditolak", l)
		}
	}
	if _, err := svc.ListAdmin(context.Background(), "all", 200); err != nil {
		t.Errorf("limit 200 seharusnya diterima: %v", err)
	}
}

// ---- handler ---------------------------------------------------------------

func serve(svc *Service, user, method, path, body string) (*httptest.ResponseRecorder, map[string]any) {
	h := NewHandler(svc)
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			next.ServeHTTP(w, req.WithContext(httpx.WithUser(req.Context(), user, "member")))
		})
	})
	r.Route("/wallet", h.Routes)
	r.Route("/admin", h.AdminRoutes)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec, out
}

func TestHandlerUserFlow(t *testing.T) {
	svc, st, _ := newFixture()
	base := "/wallet/topups"

	rec, body := serve(svc, uid, "POST", base, `{"amount":100000}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body)
	}
	topup := body["topup"].(map[string]any)
	if topup["transfer_amount"].(float64) != 100_347 || topup["unique_code"].(float64) != 347 {
		t.Errorf("topup = %v", topup)
	}
	if banks := body["bank_accounts"].([]any); len(banks) != 1 {
		t.Errorf("bank_accounts = %v", banks)
	}

	if rec, _ := serve(svc, uid, "GET", base, ""); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"topups"`) {
		t.Errorf("list: %d %s", rec.Code, rec.Body)
	}
	if rec, _ := serve(svc, uid, "GET", base+"/"+tid, ""); rec.Code != 200 {
		t.Errorf("get: %d", rec.Code)
	}
	if rec, _ := serve(svc, uid, "POST", base+"/"+tid+"/confirm", `{"sender_name":"Budi","sender_bank":"BCA"}`); rec.Code != 200 {
		t.Errorf("confirm: %d %s", rec.Code, rec.Body)
	}
	if rec, _ := serve(svc, uid, "POST", base+"/"+tid+"/cancel", ``); rec.Code != 200 {
		t.Errorf("cancel: %d %s", rec.Code, rec.Body)
	}
	last := st.calls[len(st.calls)-1]
	if last.op != "cancel" || last.user != uid || last.id != tid {
		t.Errorf("store menerima %+v; identitas harus dari token", last)
	}
}

func TestHandlerAdminFlow(t *testing.T) {
	svc, st, _ := newFixture()
	st.admin = []AdminTopup{{Topup: *st.topup, UserName: "Budi", UserEmail: "budi@example.com"}}

	rec, body := serve(svc, "adm", "GET", "/admin/topups?status=open&limit=10", "")
	if rec.Code != 200 || st.calls[0].status != "open" || st.calls[0].limit != 10 {
		t.Fatalf("list admin: %d %s calls=%+v", rec.Code, rec.Body, st.calls)
	}
	first := body["topups"].([]any)[0].(map[string]any)
	if first["user_name"] != "Budi" || first["user_email"] != "budi@example.com" {
		t.Errorf("identitas pemohon tidak tampil: %v", first)
	}

	// approve: body kosong, `{}`, dan dengan penyesuaian semuanya sah
	for _, b := range []string{``, `{}`, `{"note":"cocok dengan mutasi"}`, `{"received_amount":100347}`} {
		if rec, _ := serve(svc, "adm", "POST", "/admin/topups/"+tid+"/approve", b); rec.Code != 200 {
			t.Errorf("approve body %q: %d %s", b, rec.Code, rec.Body)
		}
	}
	last := st.calls[len(st.calls)-1]
	if last.op != "approve" || last.user != "adm" || last.received == nil || *last.received != 100347 {
		t.Errorf("store menerima %+v", last)
	}

	if rec, _ := serve(svc, "adm", "POST", "/admin/topups/"+tid+"/reject", `{"note":"transfer tidak ditemukan"}`); rec.Code != 200 {
		t.Errorf("reject: %d %s", rec.Code, rec.Body)
	}
}

func TestHandlerRejectsBadInput(t *testing.T) {
	svc, st, _ := newFixture()
	tests := []struct {
		name, user, method, path, body string
		want                           int
	}{
		{"create: bukan JSON", uid, "POST", "/wallet/topups", `x`, 400},
		{"create: field asing", uid, "POST", "/wallet/topups", `{"amount":100000,"user_id":"x"}`, 400},
		{"create: nominal kecil", uid, "POST", "/wallet/topups", `{"amount":5}`, 400},
		{"create: tanpa body", uid, "POST", "/wallet/topups", ``, 400},
		{"get: bukan UUID", uid, "GET", "/wallet/topups/abc", ``, 404},
		{"confirm: bukan UUID", uid, "POST", "/wallet/topups/abc/confirm", `{}`, 404},
		{"confirm: nama kosong", uid, "POST", "/wallet/topups/" + tid + "/confirm", `{"sender_name":"","sender_bank":"BCA"}`, 400},
		{"confirm: field asing", uid, "POST", "/wallet/topups/" + tid + "/confirm", `{"sender_name":"Budi","sender_bank":"BCA","status":"approved"}`, 400},
		{"cancel: bukan UUID", uid, "POST", "/wallet/topups/xyz/cancel", ``, 404},
		{"admin list: status asing", "a", "GET", "/admin/topups?status=semua", ``, 400},
		{"admin list: limit bukan angka", "a", "GET", "/admin/topups?limit=abc", ``, 400},
		{"admin list: limit terlalu besar", "a", "GET", "/admin/topups?limit=999", ``, 400},
		{"approve: bukan UUID", "a", "POST", "/admin/topups/abc/approve", `{}`, 404},
		{"approve: field asing", "a", "POST", "/admin/topups/" + tid + "/approve", `{"status":"approved"}`, 400},
		{"approve: nominal nol", "a", "POST", "/admin/topups/" + tid + "/approve", `{"received_amount":0}`, 400},
		{"reject: tanpa alasan", "a", "POST", "/admin/topups/" + tid + "/reject", `{}`, 400},
		{"reject: tanpa body", "a", "POST", "/admin/topups/" + tid + "/reject", ``, 400},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			before := len(st.calls)
			rec, body := serve(svc, tc.user, tc.method, tc.path, tc.body)
			if rec.Code != tc.want {
				t.Errorf("status = %d, want %d (%s)", rec.Code, tc.want, rec.Body)
			}
			if body["error"] == nil {
				t.Error(`respons seharusnya berisi {"error": "..."}`)
			}
			if len(st.calls) != before {
				t.Error("input tidak valid tidak boleh sampai ke store")
			}
		})
	}
}

func TestHandlerMapsDomainErrors(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want int
	}{
		{ErrNotFound, 404}, {ErrSelfReview, 403}, {ErrInvalidState, 409}, {ErrTooManyOpen, 409},
		{ErrBusy, 409}, {ErrNoBankAccount, 503}, {errors.New("pq: rahasia"), 500},
	} {
		svc, st, _ := newFixture()
		st.err = tc.err
		for _, c := range []struct{ user, method, path, body string }{
			{"a", "POST", "/admin/topups/" + tid + "/approve", `{}`},
			{"a", "POST", "/admin/topups/" + tid + "/reject", `{"note":"alasan"}`},
			{uid, "POST", "/wallet/topups/" + tid + "/cancel", ``},
			{uid, "POST", "/wallet/topups/" + tid + "/confirm", `{"sender_name":"Budi","sender_bank":"BCA"}`},
			{uid, "GET", "/wallet/topups/" + tid, ``},
		} {
			rec, _ := serve(svc, c.user, c.method, c.path, c.body)
			if rec.Code != tc.want {
				t.Errorf("%s %s dengan %v: status = %d, want %d", c.method, c.path, tc.err, rec.Code, tc.want)
			}
			if strings.Contains(rec.Body.String(), "rahasia") {
				t.Errorf("detail internal bocor: %s", rec.Body)
			}
		}
	}
}

func TestHandlerAdminSeesOnlyWhatStoreReturns(t *testing.T) {
	svc, st, _ := newFixture()
	st.admin = []AdminTopup{}
	rec, _ := serve(svc, "a", "GET", "/admin/topups", "")
	if !strings.Contains(rec.Body.String(), `"topups":[]`) {
		t.Errorf("daftar kosong harus [] bukan null: %s", rec.Body)
	}
}

// bytesReader membekali randomCodes dengan byte acak sungguhan yang cukup banyak.
func bytesReader() *bytes.Reader {
	b := make([]byte, 1<<16)
	if _, err := crand.Read(b); err != nil {
		panic(err)
	}
	return bytes.NewReader(b)
}
