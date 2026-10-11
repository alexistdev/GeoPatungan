/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package settings

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

// ---- fake store ------------------------------------------------------------

type fakeStore struct {
	settings Settings
	banks    []BankAccount
	err      error

	updates    int
	lastAdmin  string
	lastActive bool // onlyActive pada ListBanks terakhir
	seq        int
}

func newFakeStore() *fakeStore { return &fakeStore{settings: Defaults()} }

func (f *fakeStore) Get(context.Context) (Settings, error) { return f.settings, f.err }

func (f *fakeStore) Update(_ context.Context, adminID string, s Settings) error {
	if f.err != nil {
		return f.err
	}
	f.updates++
	f.lastAdmin = adminID
	f.settings = s
	return nil
}

func (f *fakeStore) CreateBank(_ context.Context, adminID string, b *BankAccount) error {
	if f.err != nil {
		return f.err
	}
	for _, x := range f.banks {
		if x.BankName == b.BankName && x.AccountNumber == b.AccountNumber {
			return ErrDuplicate
		}
	}
	f.seq++
	b.ID = "bank-" + string(rune('0'+f.seq))
	f.lastAdmin = adminID
	f.banks = append(f.banks, *b)
	return nil
}

func (f *fakeStore) UpdateBank(_ context.Context, adminID, id string, b *BankAccount) error {
	if f.err != nil {
		return f.err
	}
	for i := range f.banks {
		if f.banks[i].ID == id {
			b.ID = id
			f.banks[i] = *b
			f.lastAdmin = adminID
			return nil
		}
	}
	return ErrNotFound
}

func (f *fakeStore) ListBanks(_ context.Context, onlyActive bool) ([]BankAccount, error) {
	f.lastActive = onlyActive
	out := []BankAccount{}
	for _, b := range f.banks {
		if !onlyActive || b.IsActive {
			out = append(out, b)
		}
	}
	return out, f.err
}

// ---- validasi pengaturan ---------------------------------------------------

func TestValidateSettings(t *testing.T) {
	ok := Defaults()
	if err := validateSettings(ok); err != nil {
		t.Fatalf("Defaults() harus valid: %v", err)
	}

	tests := []struct {
		name    string
		mutate  func(*Settings)
		wantErr bool
	}{
		{"fee 0% (default)", func(s *Settings) { s.PlatformFeeBps = 0 }, false},
		{"fee 2,5%", func(s *Settings) { s.PlatformFeeBps = 250 }, false},
		{"fee tepat 10%", func(s *Settings) { s.PlatformFeeBps = 1000 }, false},
		{"fee di atas 10%", func(s *Settings) { s.PlatformFeeBps = 1001 }, true},
		{"fee negatif", func(s *Settings) { s.PlatformFeeBps = -1 }, true},
		{"topup min tepat 1000", func(s *Settings) { s.TopupMinAmount = 1_000 }, false},
		{"topup min di bawah 1000", func(s *Settings) { s.TopupMinAmount = 999 }, true},
		{"topup max sama dengan min", func(s *Settings) { s.TopupMinAmount, s.TopupMaxAmount = 50_000, 50_000 }, false},
		{"topup max lebih kecil dari min", func(s *Settings) { s.TopupMinAmount, s.TopupMaxAmount = 50_000, 49_999 }, true},
		{"topup max 1 miliar", func(s *Settings) { s.TopupMaxAmount = 1_000_000_000 }, false},
		{"topup max di atas 1 miliar", func(s *Settings) { s.TopupMaxAmount = 1_000_000_001 }, true},
		{"withdraw min tepat 10 ribu", func(s *Settings) { s.WithdrawMinAmount = 10_000 }, false},
		{"withdraw min di bawah 10 ribu", func(s *Settings) { s.WithdrawMinAmount = 9_999 }, true},
		{"kedaluwarsa 1 jam", func(s *Settings) { s.TopupExpiryHours = 1 }, false},
		{"kedaluwarsa 168 jam", func(s *Settings) { s.TopupExpiryHours = 168 }, false},
		{"kedaluwarsa 0 jam", func(s *Settings) { s.TopupExpiryHours = 0 }, true},
		{"kedaluwarsa 169 jam", func(s *Settings) { s.TopupExpiryHours = 169 }, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := Defaults()
			tc.mutate(&s)
			err := validateSettings(s)
			var verr httpx.ValidationError
			if tc.wantErr && !errors.As(err, &verr) {
				t.Errorf("err = %v, want ValidationError", err)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("seharusnya valid: %v", err)
			}
		})
	}
}

func TestServiceUpdateRejectsInvalidBeforeStore(t *testing.T) {
	store := newFakeStore()
	bad := Defaults()
	bad.PlatformFeeBps = 5000
	if _, err := NewService(store).Update(context.Background(), "admin-1", bad); err == nil {
		t.Fatal("seharusnya ditolak")
	}
	if store.updates != 0 {
		t.Error("pengaturan tidak valid tidak boleh sampai ke database")
	}
}

// ---- normalisasi rekening --------------------------------------------------

func TestNormalizeBank(t *testing.T) {
	good := BankAccount{BankName: "  BCA ", AccountNumber: " 123 4567 890 ", AccountHolder: " PT Geo Patungan ", Notes: "  utama ", IsActive: true}
	got, err := normalizeBank(good)
	if err != nil {
		t.Fatal(err)
	}
	if got.BankName != "BCA" || got.AccountNumber != "1234567890" || got.AccountHolder != "PT Geo Patungan" || got.Notes != "utama" {
		t.Errorf("tidak dinormalisasi: %+v", got)
	}

	invalid := map[string]func(*BankAccount){
		"bank kosong":           func(b *BankAccount) { b.BankName = "  " },
		"bank 1 huruf":          func(b *BankAccount) { b.BankName = "B" },
		"bank terlalu panjang":  func(b *BankAccount) { b.BankName = strings.Repeat("a", 51) },
		"rekening berisi huruf": func(b *BankAccount) { b.AccountNumber = "12ab567" },
		"rekening 4 digit":      func(b *BankAccount) { b.AccountNumber = "1234" },
		"rekening 31 digit":     func(b *BankAccount) { b.AccountNumber = strings.Repeat("1", 31) },
		"rekening kosong":       func(b *BankAccount) { b.AccountNumber = "" },
		"pemilik kosong":        func(b *BankAccount) { b.AccountHolder = "" },
		"pemilik 1 huruf":       func(b *BankAccount) { b.AccountHolder = "A" },
		"catatan 501 karakter":  func(b *BankAccount) { b.Notes = strings.Repeat("a", 501) },
		"rekening dgn tanda -":  func(b *BankAccount) { b.AccountNumber = "123-456-789" },
	}
	for name, mutate := range invalid {
		t.Run(name, func(t *testing.T) {
			b := BankAccount{BankName: "BCA", AccountNumber: "1234567890", AccountHolder: "PT Geo"}
			mutate(&b)
			_, err := normalizeBank(b)
			var verr httpx.ValidationError
			if !errors.As(err, &verr) {
				t.Errorf("err = %v, want ValidationError", err)
			}
		})
	}
}

// ---- service ---------------------------------------------------------------

func TestPaymentInfoShowsOnlyActiveBanks(t *testing.T) {
	store := newFakeStore()
	store.settings.PlatformFeeBps = 150
	store.banks = []BankAccount{
		{ID: "1", BankName: "BCA", AccountNumber: "11111", AccountHolder: "A", IsActive: true},
		{ID: "2", BankName: "BNI", AccountNumber: "22222", AccountHolder: "B", IsActive: false},
	}
	info, err := NewService(store).PaymentInfo(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !store.lastActive {
		t.Error("PaymentInfo harus meminta hanya rekening aktif")
	}
	if len(info.BankAccounts) != 1 || info.BankAccounts[0].BankName != "BCA" {
		t.Errorf("rekening = %+v, want hanya BCA", info.BankAccounts)
	}
	if info.PlatformFeeBps != 150 || info.TopupMinAmount != 10_000 || info.TopupExpiryHours != 24 {
		t.Errorf("batas/fee tidak terbawa: %+v", info)
	}

	// admin melihat semuanya
	all, _ := NewService(store).ListBanks(context.Background())
	if store.lastActive || len(all) != 2 {
		t.Errorf("admin harus melihat semua rekening: len=%d onlyActive=%v", len(all), store.lastActive)
	}
}

// ---- handler ---------------------------------------------------------------

func serve(store *fakeStore, method, path, body string) *httptest.ResponseRecorder {
	h := NewHandler(NewService(store))
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			next.ServeHTTP(w, req.WithContext(httpx.WithUser(req.Context(), "admin-7", "admin")))
		})
	})
	r.Route("/admin", h.AdminRoutes)
	r.Get("/payment-info", h.PaymentInfo)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
	return rec
}

func TestHandlerSettings(t *testing.T) {
	store := newFakeStore()

	rec := serve(store, http.MethodGet, "/admin/settings", "")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"platform_fee_bps":0`) {
		t.Fatalf("GET: %d %s", rec.Code, rec.Body)
	}

	body := `{"platform_fee_bps":200,"topup_min_amount":20000,"topup_max_amount":50000000,"withdraw_min_amount":100000,"topup_expiry_hours":48}`
	rec = serve(store, http.MethodPut, "/admin/settings", body)
	if rec.Code != 200 {
		t.Fatalf("PUT: %d %s", rec.Code, rec.Body)
	}
	if store.settings.PlatformFeeBps != 200 || store.settings.TopupExpiryHours != 48 || store.lastAdmin != "admin-7" {
		t.Errorf("tersimpan %+v oleh %q", store.settings, store.lastAdmin)
	}

	bad := map[string]string{
		"JSON rusak":        `{`,
		"field asing":       `{"platform_fee_bps":0,"role":"admin"}`,
		"fee terlalu besar": `{"platform_fee_bps":9999,"topup_min_amount":20000,"topup_max_amount":50000000,"withdraw_min_amount":100000,"topup_expiry_hours":48}`,
		"field kurang":      `{"platform_fee_bps":100}`, // sisanya 0 -> tidak valid, bukan diam-diam bawaan
	}
	before := store.updates
	for name, b := range bad {
		t.Run(name, func(t *testing.T) {
			rec := serve(store, http.MethodPut, "/admin/settings", b)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400 (%s)", rec.Code, rec.Body)
			}
		})
	}
	if store.updates != before {
		t.Error("permintaan tidak valid tidak boleh menyimpan apa pun")
	}
}

func TestHandlerBankAccounts(t *testing.T) {
	store := newFakeStore()
	create := `{"bank_name":"BCA","account_number":"1234567890","account_holder":"PT Geo","notes":"utama"}`

	rec := serve(store, http.MethodPost, "/admin/bank-accounts", create)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body)
	}
	var created BankAccount
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil || created.ID == "" || !created.IsActive {
		t.Fatalf("respons create: %s (%v); is_active kosong harus berarti aktif", rec.Body, err)
	}
	if store.lastAdmin != "admin-7" {
		t.Errorf("pelaku = %q, want admin-7", store.lastAdmin)
	}

	if rec := serve(store, http.MethodPost, "/admin/bank-accounts", create); rec.Code != http.StatusConflict {
		t.Errorf("duplikat: %d, want 409", rec.Code)
	}
	if rec := serve(store, http.MethodPost, "/admin/bank-accounts", `{"bank_name":"BCA","account_number":"abc","account_holder":"X Y"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("rekening tidak valid: %d, want 400", rec.Code)
	}
	if rec := serve(store, http.MethodPost, "/admin/bank-accounts", `{"bank_name":"BCA","account_number":"12345","account_holder":"X Y","owner":"x"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("field asing: %d, want 400", rec.Code)
	}

	// nonaktifkan
	store.banks[0].ID = "11111111-1111-1111-1111-111111111111"
	url := "/admin/bank-accounts/11111111-1111-1111-1111-111111111111"
	off := `{"bank_name":"BCA","account_number":"1234567890","account_holder":"PT Geo","is_active":false}`
	rec = serve(store, http.MethodPut, url, off)
	if rec.Code != 200 || store.banks[0].IsActive {
		t.Errorf("nonaktifkan: %d aktif=%v", rec.Code, store.banks[0].IsActive)
	}

	if rec := serve(store, http.MethodPut, "/admin/bank-accounts/22222222-2222-2222-2222-222222222222", off); rec.Code != http.StatusNotFound {
		t.Errorf("rekening tidak ada: %d, want 404", rec.Code)
	}
	if rec := serve(store, http.MethodPut, "/admin/bank-accounts/abc", off); rec.Code != http.StatusNotFound {
		t.Errorf("id bukan UUID: %d, want 404", rec.Code)
	}

	// admin melihat semua, pengguna hanya yang aktif
	rec = serve(store, http.MethodGet, "/admin/bank-accounts", "")
	if !strings.Contains(rec.Body.String(), `"BCA"`) {
		t.Errorf("admin harus melihat rekening nonaktif: %s", rec.Body)
	}
	rec = serve(store, http.MethodGet, "/payment-info", "")
	if rec.Code != 200 || strings.Contains(rec.Body.String(), `"BCA"`) || !strings.Contains(rec.Body.String(), `"bank_accounts":[]`) {
		t.Errorf("pengguna tidak boleh melihat rekening nonaktif: %s", rec.Body)
	}
}

func TestHandlerHidesInternalErrors(t *testing.T) {
	store := newFakeStore()
	store.err = errors.New("pq: koneksi putus rahasia")
	for _, c := range []struct{ method, path, body string }{
		{http.MethodGet, "/admin/settings", ""},
		{http.MethodGet, "/admin/bank-accounts", ""},
		{http.MethodGet, "/payment-info", ""},
		{http.MethodPost, "/admin/bank-accounts", `{"bank_name":"BCA","account_number":"12345","account_holder":"X Y"}`},
	} {
		rec := serve(store, c.method, c.path, c.body)
		if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "rahasia") {
			t.Errorf("%s %s: %d %s", c.method, c.path, rec.Code, rec.Body)
		}
	}
}
