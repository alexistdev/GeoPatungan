/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package withdrawal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/alexistdev/geopatungan/internal/httpx"
	"github.com/alexistdev/geopatungan/internal/settings"
	"github.com/alexistdev/geopatungan/internal/wallet"
)

// ---- fakes -----------------------------------------------------------------

type fakeSettings struct {
	cfg settings.Settings
	err error
}

func (f *fakeSettings) Get(context.Context) (settings.Settings, error) { return f.cfg, f.err }

type fakeStore struct {
	wd    *Withdrawal
	list  []Withdrawal
	admin []AdminWithdrawal
	err   error

	calls   []string
	lastIn  Input
	maxOpen int
	status  string
	limit   int
	ref     string
	note    string
}

func (f *fakeStore) rec(format string, a ...any) {
	f.calls = append(f.calls, fmt.Sprintf(format, a...))
}

func (f *fakeStore) Create(_ context.Context, userID string, in Input, maxOpen int) (*Withdrawal, error) {
	f.rec("create %s", userID)
	f.lastIn, f.maxOpen = in, maxOpen
	return f.wd, f.err
}
func (f *fakeStore) Get(_ context.Context, userID, id string) (*Withdrawal, error) {
	f.rec("get %s %s", userID, id)
	return f.wd, f.err
}
func (f *fakeStore) ListByUser(_ context.Context, userID string, limit int) ([]Withdrawal, error) {
	f.rec("list %s", userID)
	f.limit = limit
	return f.list, f.err
}
func (f *fakeStore) ListAdmin(_ context.Context, status string, limit int) ([]AdminWithdrawal, error) {
	f.rec("list_admin")
	f.status, f.limit = status, limit
	return f.admin, f.err
}
func (f *fakeStore) Complete(_ context.Context, adminID, id, reference, note string) (*Withdrawal, error) {
	f.rec("complete %s %s", adminID, id)
	f.ref, f.note = reference, note
	return f.wd, f.err
}
func (f *fakeStore) Reject(_ context.Context, adminID, id, note string) (*Withdrawal, error) {
	f.rec("reject %s %s", adminID, id)
	f.note = note
	return f.wd, f.err
}
func (f *fakeStore) Cancel(_ context.Context, userID, id string) (*Withdrawal, error) {
	f.rec("cancel %s %s", userID, id)
	return f.wd, f.err
}

const wid = "11111111-1111-1111-1111-111111111111"

func newFixture() (*Service, *fakeStore, *fakeSettings) {
	st := &fakeStore{wd: &Withdrawal{ID: wid, UserID: "u1", Amount: 200_000, BankName: "BCA",
		AccountNumber: "1234567890", AccountHolder: "Budi", Status: StatusPending}}
	cfg := &fakeSettings{cfg: settings.Defaults()}
	return NewService(st, cfg), st, cfg
}

func goodInput() Input {
	return Input{Amount: 200_000, BankName: "BCA", AccountNumber: "1234567890", AccountHolder: "Budi Santoso"}
}

// ---- Create ----------------------------------------------------------------

func TestServiceCreateNormalizesAndForwards(t *testing.T) {
	svc, st, _ := newFixture()
	in := Input{Amount: 200_000, BankName: "  BCA ", AccountNumber: " 123 4567 890 ", AccountHolder: " Budi Santoso ", Note: "  tarik ke rekening utama "}
	if _, err := svc.Create(context.Background(), "u1", in); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got := st.lastIn
	if got.BankName != "BCA" || got.AccountNumber != "1234567890" || got.AccountHolder != "Budi Santoso" || got.Note != "tarik ke rekening utama" {
		t.Errorf("input tidak dinormalisasi: %+v", got)
	}
	if st.maxOpen != MaxOpenPerUser {
		t.Errorf("maxOpen = %d, want %d", st.maxOpen, MaxOpenPerUser)
	}
	if st.calls[0] != "create u1" {
		t.Errorf("calls = %v", st.calls)
	}
}

func TestServiceCreateAmountBounds(t *testing.T) {
	for _, tc := range []struct {
		amount  int64
		wantErr bool
	}{
		{49_999, true}, {50_000, false}, {50_001, false}, {1_000_000_000, false}, {1_000_000_001, true},
		{0, true}, {-1, true}, {-200_000, true},
	} {
		svc, st, _ := newFixture()
		in := goodInput()
		in.Amount = tc.amount
		_, err := svc.Create(context.Background(), "u1", in)
		var verr httpx.ValidationError
		if tc.wantErr != errors.As(err, &verr) {
			t.Errorf("amount %d: err = %v, wantErr = %v", tc.amount, err, tc.wantErr)
		}
		if tc.wantErr && len(st.calls) != 0 {
			t.Errorf("amount %d: nominal tidak valid tidak boleh sampai ke store", tc.amount)
		}
	}

	// minimum mengikuti pengaturan admin; pesan memuat angkanya dengan titik ribuan
	svc, _, cfg := newFixture()
	cfg.cfg.WithdrawMinAmount = 250_000
	_, err := svc.Create(context.Background(), "u1", Input{Amount: 249_999, BankName: "BCA", AccountNumber: "12345", AccountHolder: "Budi"})
	if err == nil || !strings.Contains(err.Error(), "Rp250.000") || !strings.Contains(err.Error(), "Rp1.000.000.000") {
		t.Errorf("pesan = %v, want memuat Rp250.000 dan Rp1.000.000.000", err)
	}
	if _, err := svc.Create(context.Background(), "u1", Input{Amount: 250_000, BankName: "BCA", AccountNumber: "12345", AccountHolder: "Budi"}); err != nil {
		t.Errorf("tepat di minimum seharusnya diterima: %v", err)
	}
}

func TestServiceCreateBankDetailsValidation(t *testing.T) {
	bad := map[string]func(*Input){
		"bank kosong":           func(i *Input) { i.BankName = "" },
		"bank 1 huruf":          func(i *Input) { i.BankName = "B" },
		"bank 51 huruf":         func(i *Input) { i.BankName = strings.Repeat("a", 51) },
		"rekening kosong":       func(i *Input) { i.AccountNumber = "" },
		"rekening 4 digit":      func(i *Input) { i.AccountNumber = "1234" },
		"rekening 31 digit":     func(i *Input) { i.AccountNumber = strings.Repeat("1", 31) },
		"rekening berisi huruf": func(i *Input) { i.AccountNumber = "12ab567" },
		"rekening dengan tanda": func(i *Input) { i.AccountNumber = "123-456-789" },
		"rekening sisipan SQL":  func(i *Input) { i.AccountNumber = "12345'; DROP TABLE users;--" },
		"pemilik kosong":        func(i *Input) { i.AccountHolder = "   " },
		"pemilik 1 huruf":       func(i *Input) { i.AccountHolder = "A" },
		"pemilik 101 huruf":     func(i *Input) { i.AccountHolder = strings.Repeat("a", 101) },
		"catatan 201 huruf":     func(i *Input) { i.Note = strings.Repeat("a", 201) },
	}
	for name, mutate := range bad {
		t.Run(name, func(t *testing.T) {
			svc, st, _ := newFixture()
			in := goodInput()
			mutate(&in)
			_, err := svc.Create(context.Background(), "u1", in)
			var verr httpx.ValidationError
			if !errors.As(err, &verr) || len(st.calls) != 0 {
				t.Errorf("err = %v calls = %d, want ValidationError tanpa menyentuh store", err, len(st.calls))
			}
		})
	}

	svc, _, _ := newFixture()
	edge := Input{Amount: 50_000, BankName: strings.Repeat("a", 50), AccountNumber: strings.Repeat("1", 30),
		AccountHolder: strings.Repeat("b", 100), Note: strings.Repeat("c", 200)}
	if _, err := svc.Create(context.Background(), "u1", edge); err != nil {
		t.Errorf("batas tepat seharusnya diterima: %v", err)
	}
}

func TestServiceCreateSettingsErrorBlocksRequest(t *testing.T) {
	svc, st, cfg := newFixture()
	cfg.err = errors.New("pengaturan rusak")
	if _, err := svc.Create(context.Background(), "u1", goodInput()); err == nil {
		t.Fatal("kegagalan pengaturan seharusnya menggagalkan permintaan")
	}
	if len(st.calls) != 0 {
		t.Error("tanpa pengaturan, permintaan tidak boleh diteruskan")
	}
}

// ---- Complete / Reject / ListAdmin ----------------------------------------

func TestServiceCompleteRequiresTransferReference(t *testing.T) {
	for name, tc := range map[string]struct {
		ref, note string
		wantErr   bool
	}{
		"referensi normal": {"TRX-20261012-001", "", false},
		"dipangkas":        {"  ABC123  ", "  ok  ", false},
		"kosong":           {"", "", true},
		"spasi":            {"     ", "", true},
		"2 karakter":       {"ab", "", true},
		"3 karakter":       {"abc", "", false},
		"100 karakter":     {strings.Repeat("a", 100), "", false},
		"101 karakter":     {strings.Repeat("a", 101), "", true},
		"catatan 200":      {"abc", strings.Repeat("n", 200), false},
		"catatan 201":      {"abc", strings.Repeat("n", 201), true},
	} {
		t.Run(name, func(t *testing.T) {
			svc, st, _ := newFixture()
			_, err := svc.Complete(context.Background(), "adm", wid, tc.ref, tc.note)
			var verr httpx.ValidationError
			if tc.wantErr != errors.As(err, &verr) {
				t.Errorf("err = %v, wantErr = %v", err, tc.wantErr)
			}
			if tc.wantErr && len(st.calls) != 0 {
				t.Error("input tidak valid tidak boleh sampai ke store")
			}
			if !tc.wantErr && st.ref != strings.TrimSpace(tc.ref) {
				t.Errorf("referensi ke store = %q, want %q", st.ref, strings.TrimSpace(tc.ref))
			}
		})
	}
}

func TestServiceRejectRequiresReason(t *testing.T) {
	for name, tc := range map[string]struct {
		note    string
		wantErr bool
	}{"kosong": {"", true}, "spasi": {"   ", true}, "2": {"ab", true}, "3": {"abc", false},
		"200": {strings.Repeat("a", 200), false}, "201": {strings.Repeat("a", 201), true}} {
		t.Run(name, func(t *testing.T) {
			svc, st, _ := newFixture()
			_, err := svc.Reject(context.Background(), "adm", wid, tc.note)
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
	if st.status != StatusPending || st.limit != DefaultListLimit {
		t.Errorf("bawaan = %q/%d, want pending/%d", st.status, st.limit, DefaultListLimit)
	}
	for _, s := range []string{"pending", "completed", "rejected", "cancelled", "all"} {
		if _, err := newFixtureService().ListAdmin(context.Background(), s, 10); err != nil {
			t.Errorf("status %q seharusnya valid: %v", s, err)
		}
	}
	for _, s := range []string{"PENDING", "open", "selesai", "all;DROP"} {
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
}

func newFixtureService() *Service { svc, _, _ := newFixture(); return svc }

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

const goodBody = `{"amount":200000,"bank_name":"BCA","account_number":"1234567890","account_holder":"Budi Santoso"}`

func TestHandlerUserFlow(t *testing.T) {
	svc, st, _ := newFixture()

	rec, body := serve(svc, "u-7", "POST", "/wallet/withdrawals", goodBody)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body)
	}
	wd := body["withdrawal"].(map[string]any)
	for _, key := range []string{"id", "amount", "bank_name", "account_number", "account_holder", "status",
		"transfer_reference", "review_note", "reviewed_at", "created_at"} {
		if _, ok := wd[key]; !ok {
			t.Errorf("withdrawal.%s hilang", key)
		}
	}
	if st.calls[0] != "create u-7" {
		t.Errorf("store menerima %v; pemilik harus dari identitas login", st.calls)
	}

	if rec, _ := serve(svc, "u-7", "GET", "/wallet/withdrawals", ""); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"withdrawals"`) {
		t.Errorf("list: %d %s", rec.Code, rec.Body)
	}
	if rec, _ := serve(svc, "u-7", "GET", "/wallet/withdrawals/"+wid, ""); rec.Code != 200 {
		t.Errorf("get: %d", rec.Code)
	}
	if rec, _ := serve(svc, "u-7", "POST", "/wallet/withdrawals/"+wid+"/cancel", ""); rec.Code != 200 {
		t.Errorf("cancel: %d %s", rec.Code, rec.Body)
	}
	if last := st.calls[len(st.calls)-1]; last != "cancel u-7 "+wid {
		t.Errorf("cancel diteruskan sebagai %q", last)
	}
}

func TestHandlerAdminFlow(t *testing.T) {
	svc, st, _ := newFixture()
	st.admin = []AdminWithdrawal{{Withdrawal: *st.wd, UserName: "Budi", UserEmail: "budi@example.com"}}

	rec, body := serve(svc, "adm", "GET", "/admin/withdrawals?limit=10", "")
	if rec.Code != 200 || st.status != StatusPending || st.limit != 10 {
		t.Fatalf("list admin: %d %s status=%s limit=%d", rec.Code, rec.Body, st.status, st.limit)
	}
	first := body["withdrawals"].([]any)[0].(map[string]any)
	if first["user_name"] != "Budi" || first["user_email"] != "budi@example.com" || first["account_number"] != "1234567890" {
		t.Errorf("admin harus melihat identitas dan rekening tujuan: %v", first)
	}

	if rec, _ := serve(svc, "adm", "POST", "/admin/withdrawals/"+wid+"/complete", `{"reference":"TRX-001","note":"sudah ditransfer"}`); rec.Code != 200 {
		t.Errorf("complete: %d %s", rec.Code, rec.Body)
	}
	if st.ref != "TRX-001" || st.note != "sudah ditransfer" {
		t.Errorf("ref/note = %q/%q", st.ref, st.note)
	}
	if rec, _ := serve(svc, "adm", "POST", "/admin/withdrawals/"+wid+"/reject", `{"note":"nama rekening tidak cocok"}`); rec.Code != 200 {
		t.Errorf("reject: %d %s", rec.Code, rec.Body)
	}
}

func TestHandlerRejectsBadInputBeforeStore(t *testing.T) {
	svc, st, _ := newFixture()
	tests := []struct {
		name, user, method, path, body string
		want                           int
	}{
		{"create: bukan JSON", "u1", "POST", "/wallet/withdrawals", `x`, 400},
		{"create: tanpa body", "u1", "POST", "/wallet/withdrawals", ``, 400},
		{"create: field asing", "u1", "POST", "/wallet/withdrawals", `{"amount":200000,"bank_name":"BCA","account_number":"12345","account_holder":"Budi","status":"completed"}`, 400},
		{"create: nominal kecil", "u1", "POST", "/wallet/withdrawals", `{"amount":10,"bank_name":"BCA","account_number":"12345","account_holder":"Budi"}`, 400},
		{"create: rekening huruf", "u1", "POST", "/wallet/withdrawals", `{"amount":200000,"bank_name":"BCA","account_number":"abc","account_holder":"Budi"}`, 400},
		{"get: bukan UUID", "u1", "GET", "/wallet/withdrawals/abc", ``, 404},
		{"cancel: bukan UUID", "u1", "POST", "/wallet/withdrawals/abc/cancel", ``, 404},
		{"admin: status asing", "a", "GET", "/admin/withdrawals?status=selesai", ``, 400},
		{"admin: limit bukan angka", "a", "GET", "/admin/withdrawals?limit=abc", ``, 400},
		{"complete: bukan UUID", "a", "POST", "/admin/withdrawals/abc/complete", `{"reference":"TRX-1"}`, 404},
		{"complete: tanpa referensi", "a", "POST", "/admin/withdrawals/" + wid + "/complete", `{}`, 400},
		{"complete: tanpa body", "a", "POST", "/admin/withdrawals/" + wid + "/complete", ``, 400},
		{"complete: field asing", "a", "POST", "/admin/withdrawals/" + wid + "/complete", `{"reference":"TRX-1","amount":1}`, 400},
		{"reject: tanpa alasan", "a", "POST", "/admin/withdrawals/" + wid + "/reject", `{}`, 400},
		{"reject: bukan UUID", "a", "POST", "/admin/withdrawals/abc/reject", `{"note":"alasan"}`, 404},
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
		{errors.New("pq: koneksi putus rahasia"), 500},
	} {
		for _, c := range []struct{ user, method, path, body string }{
			{"u1", "POST", "/wallet/withdrawals", goodBody},
			{"u1", "GET", "/wallet/withdrawals/" + wid, ``},
			{"u1", "POST", "/wallet/withdrawals/" + wid + "/cancel", ``},
			{"a", "POST", "/admin/withdrawals/" + wid + "/complete", `{"reference":"TRX-1"}`},
			{"a", "POST", "/admin/withdrawals/" + wid + "/reject", `{"note":"alasan"}`},
		} {
			svc, st, _ := newFixture()
			st.err = tc.err
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

func TestHandlerInsufficientFundsReturns402WithNumbers(t *testing.T) {
	svc, st, _ := newFixture()
	st.err = &wallet.InsufficientFundsError{Required: 200_000, Balance: 150_000}
	rec, body := serve(svc, "u1", "POST", "/wallet/withdrawals", goodBody)
	if rec.Code != http.StatusPaymentRequired {
		t.Fatalf("status = %d, want 402", rec.Code)
	}
	if body["error"] != "saldo tidak cukup" || body["required"].(float64) != 200_000 || body["balance"].(float64) != 150_000 {
		t.Errorf("body = %v", body)
	}
}

func TestHandlerEmptyListsAreArrays(t *testing.T) {
	svc, st, _ := newFixture()
	st.list, st.admin = []Withdrawal{}, []AdminWithdrawal{}
	if rec, _ := serve(svc, "u", "GET", "/wallet/withdrawals", ""); !strings.Contains(rec.Body.String(), `"withdrawals":[]`) {
		t.Errorf("daftar pengguna kosong: %s", rec.Body)
	}
	if rec, _ := serve(svc, "a", "GET", "/admin/withdrawals", ""); !strings.Contains(rec.Body.String(), `"withdrawals":[]`) {
		t.Errorf("daftar admin kosong: %s", rec.Body)
	}
}
