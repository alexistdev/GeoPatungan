/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package wallet

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

const (
	w1  = "11111111-1111-1111-1111-111111111111"
	w2  = "22222222-2222-2222-2222-222222222222"
	w3  = "33333333-3333-3333-3333-333333333333"
	ref = "99999999-9999-9999-9999-999999999999"
)

func TestValidate(t *testing.T) {
	okRef := Ref{Type: "topup", ID: ref}
	pair := []Leg{{WalletID: w1, Amount: -5}, {WalletID: w2, Amount: 5}}

	valid := []struct {
		name string
		ref  Ref
		legs []Leg
	}{
		{"dua leg", okRef, pair},
		{"tiga leg", okRef, []Leg{{w1, -10}, {w2, 6}, {w3, 4}}},
		{"jenis ref dengan garis bawah", Ref{Type: "withdrawal_request", ID: ref}, pair},
		{"deskripsi 200 karakter", Ref{Type: "x", ID: ref, Description: strings.Repeat("a", 200)}, pair},
		{"nominal besar", okRef, []Leg{{w1, -1 << 50}, {w2, 1 << 50}}},
	}
	for _, tc := range valid {
		t.Run("valid: "+tc.name, func(t *testing.T) {
			if err := validate(tc.ref, tc.legs); err != nil {
				t.Errorf("seharusnya valid: %v", err)
			}
		})
	}

	invalid := []struct {
		name string
		ref  Ref
		legs []Leg
	}{
		{"nil", okRef, nil},
		{"satu leg", okRef, []Leg{{w1, 1}}},
		{"jumlah tidak nol", okRef, []Leg{{w1, -5}, {w2, 4}}},
		{"leg nol", okRef, []Leg{{w1, 0}, {w2, 0}}},
		{"leg nol di antara leg lain", okRef, []Leg{{w1, -5}, {w2, 5}, {w3, 0}}},
		{"dompet ganda", okRef, []Leg{{w1, -5}, {w1, 5}}},
		{"dompet ganda beda huruf", okRef, []Leg{{"aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", -5}, {"AAAAAAAA-AAAA-AAAA-AAAA-AAAAAAAAAAAA", 5}}},
		{"wallet id bukan UUID", okRef, []Leg{{"x", -5}, {w2, 5}}},
		{"jenis ref kosong", Ref{ID: ref}, pair},
		{"jenis ref diawali angka", Ref{Type: "1topup", ID: ref}, pair},
		{"jenis ref huruf besar", Ref{Type: "TopUp", ID: ref}, pair},
		{"jenis ref 41 karakter", Ref{Type: strings.Repeat("a", 41), ID: ref}, pair},
		{"jenis ref berisi spasi", Ref{Type: "top up", ID: ref}, pair},
		{"id ref bukan UUID", Ref{Type: "topup", ID: "1"}, pair},
		{"id ref kosong", Ref{Type: "topup"}, pair},
		{"deskripsi 201 karakter", Ref{Type: "topup", ID: ref, Description: strings.Repeat("a", 201)}, pair},
		// Overflow int64: +MaxInt64 +MaxInt64 +2 = 2^64 = 0 setelah wrap-around, padahal semuanya
		// kredit. Tanpa pengecekan overflow, ini lolos sebagai "seimbang" dan menciptakan uang.
		{"overflow semua kredit", okRef, []Leg{{w1, 1<<63 - 1}, {w2, 1<<63 - 1}, {w3, 2}}},
		{"overflow semua debit", okRef, []Leg{{w1, -(1 << 63)}, {w2, -(1 << 63)}}},
		{"tidak seimbang besar", okRef, []Leg{{w1, -(1 << 40)}, {w2, 1<<40 - 1}}},
	}
	for _, tc := range invalid {
		t.Run("tidak valid: "+tc.name, func(t *testing.T) {
			if err := validate(tc.ref, tc.legs); !errors.Is(err, ErrInvalidLegs) {
				t.Errorf("err = %v, want ErrInvalidLegs", err)
			}
		})
	}
}

// ---- fake store -------------------------------------------------------------

type fakeStore struct {
	wallet  *Wallet
	entries []Entry // sudah urut menurun menurut id
	report  *Report
	err     error

	gotUser   string
	gotLimit  int
	gotBefore int64
}

func (f *fakeStore) ByUser(_ context.Context, userID string) (*Wallet, error) {
	f.gotUser = userID
	return f.wallet, f.err
}

// Ledger meniru Repository: entri ber-id < before (jika before > 0), maksimal limit+1 baris.
func (f *fakeStore) Ledger(_ context.Context, userID string, limit int, before int64) ([]Entry, error) {
	f.gotUser, f.gotLimit, f.gotBefore = userID, limit, before
	if f.err != nil {
		return nil, f.err
	}
	out := []Entry{}
	for _, e := range f.entries {
		if before == 0 || e.ID < before {
			out = append(out, e)
		}
		if len(out) == limit+1 {
			break
		}
	}
	return out, nil
}

func (f *fakeStore) Reconcile(context.Context) (*Report, error) { return f.report, f.err }

func entriesDesc(n int) []Entry {
	out := make([]Entry, n)
	for i := range out {
		out[i] = Entry{ID: int64(n - i), Amount: 10, RefType: "test", RefID: ref}
	}
	return out
}

// ---- service ----------------------------------------------------------------

func TestServiceLedgerPaging(t *testing.T) {
	ctx := context.Background()

	t.Run("limit bawaan 20", func(t *testing.T) {
		store := &fakeStore{entries: entriesDesc(50)}
		page, err := NewService(store).Ledger(ctx, "u1", 0, 0)
		if err != nil || len(page.Entries) != 20 || store.gotLimit != 20 {
			t.Fatalf("len=%d limit ke store=%d err=%v, want 20", len(page.Entries), store.gotLimit, err)
		}
	})
	t.Run("halaman penuh dan masih ada lagi", func(t *testing.T) {
		store := &fakeStore{entries: entriesDesc(10)}
		page, _ := NewService(store).Ledger(ctx, "u1", 4, 0)
		if len(page.Entries) != 4 || page.NextBefore == nil || *page.NextBefore != page.Entries[3].ID {
			t.Errorf("page = %+v next=%v, want 4 entri dan next_before = id entri terakhir", page.Entries, page.NextBefore)
		}
	})
	t.Run("tepat limit entri: tidak ada halaman berikutnya", func(t *testing.T) {
		page, _ := NewService(&fakeStore{entries: entriesDesc(4)}).Ledger(ctx, "u1", 4, 0)
		if len(page.Entries) != 4 || page.NextBefore != nil {
			t.Errorf("len=%d next=%v, want 4 dan nil", len(page.Entries), page.NextBefore)
		}
	})
	t.Run("kurang dari limit", func(t *testing.T) {
		page, _ := NewService(&fakeStore{entries: entriesDesc(2)}).Ledger(ctx, "u1", 4, 0)
		if len(page.Entries) != 2 || page.NextBefore != nil {
			t.Errorf("len=%d next=%v", len(page.Entries), page.NextBefore)
		}
	})
	t.Run("kosong berupa slice, bukan nil", func(t *testing.T) {
		page, _ := NewService(&fakeStore{}).Ledger(ctx, "u1", 5, 0)
		if page.Entries == nil || len(page.Entries) != 0 {
			t.Errorf("Entries = %#v", page.Entries)
		}
	})
	t.Run("berjalan sampai habis tanpa duplikat", func(t *testing.T) {
		svc := NewService(&fakeStore{entries: entriesDesc(23)})
		seen := map[int64]bool{}
		var before int64
		for i := 0; i < 20; i++ {
			page, err := svc.Ledger(ctx, "u1", 5, before)
			if err != nil {
				t.Fatal(err)
			}
			for _, e := range page.Entries {
				if seen[e.ID] {
					t.Fatalf("entri %d muncul dua kali", e.ID)
				}
				seen[e.ID] = true
			}
			if page.NextBefore == nil {
				break
			}
			before = *page.NextBefore
		}
		if len(seen) != 23 {
			t.Errorf("terbaca %d entri, want 23", len(seen))
		}
	})
}

func TestServiceLedgerValidatesParams(t *testing.T) {
	for _, tc := range []struct {
		name          string
		limit         int
		before        int64
		wantErr       bool
		wantStoreCall bool
	}{
		{"limit 1", 1, 0, false, true},
		{"limit 100", 100, 0, false, true},
		{"limit 101", 101, 0, true, false},
		{"limit negatif", -1, 0, true, false},
		{"before negatif", 10, -5, true, false},
		{"before 0", 10, 0, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &fakeStore{entries: entriesDesc(3)}
			_, err := NewService(store).Ledger(context.Background(), "u1", tc.limit, tc.before)
			var verr httpx.ValidationError
			if tc.wantErr != errors.As(err, &verr) {
				t.Errorf("err = %v, wantErr = %v", err, tc.wantErr)
			}
			if called := store.gotUser != ""; called != tc.wantStoreCall {
				t.Errorf("store dipanggil = %v, want %v", called, tc.wantStoreCall)
			}
		})
	}
}

// ---- handler ----------------------------------------------------------------

func serve(store *fakeStore, user, path string) *httptest.ResponseRecorder {
	h := NewHandler(NewService(store))
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			next.ServeHTTP(w, req.WithContext(httpx.WithUser(req.Context(), user, "member")))
		})
	})
	r.Route("/wallet", h.Routes)
	r.Route("/admin", h.AdminRoutes)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestHandlerGetWallet(t *testing.T) {
	store := &fakeStore{wallet: &Wallet{ID: w1, Balance: 12345}}
	rec := serve(store, "u-9", "/wallet/")
	if rec.Code != 200 {
		t.Fatalf("status = %d (%s)", rec.Code, rec.Body)
	}
	var body struct {
		Wallet map[string]any `json:"wallet"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Wallet["balance"].(float64) != 12345 || body.Wallet["id"] != w1 {
		t.Errorf("wallet = %v", body.Wallet)
	}
	if store.gotUser != "u-9" {
		t.Errorf("store menerima user %q, want u-9 (harus dari identitas login, bukan dari parameter)", store.gotUser)
	}
}

func TestHandlerLedger(t *testing.T) {
	t.Run("parameter diteruskan", func(t *testing.T) {
		store := &fakeStore{entries: entriesDesc(10)}
		rec := serve(store, "u-1", "/wallet/ledger?limit=3&before=8")
		if rec.Code != 200 || store.gotLimit != 3 || store.gotBefore != 8 {
			t.Fatalf("status=%d limit=%d before=%d (%s)", rec.Code, store.gotLimit, store.gotBefore, rec.Body)
		}
		var page struct {
			Entries    []map[string]any `json:"entries"`
			NextBefore *int64           `json:"next_before"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil || len(page.Entries) != 3 || page.NextBefore == nil {
			t.Fatalf("body = %s (%v)", rec.Body, err)
		}
		for _, key := range []string{"id", "amount", "balance_after", "ref_type", "ref_id", "description", "created_at"} {
			if _, ok := page.Entries[0][key]; !ok {
				t.Errorf("field %q hilang", key)
			}
		}
		if _, leaked := page.Entries[0]["entry_group"]; leaked {
			t.Error("entry_group tidak boleh bocor ke pengguna")
		}
	})
	t.Run("halaman terakhir: next_before null", func(t *testing.T) {
		rec := serve(&fakeStore{entries: entriesDesc(2)}, "u-1", "/wallet/ledger")
		if !strings.Contains(rec.Body.String(), `"next_before":null`) {
			t.Errorf("body = %s", rec.Body)
		}
	})

	bad := map[string]string{
		"limit bukan angka":   "/wallet/ledger?limit=abc",
		"before bukan angka":  "/wallet/ledger?before=1.5",
		"limit terlalu besar": "/wallet/ledger?limit=1000",
		"limit negatif":       "/wallet/ledger?limit=-3",
		"before negatif":      "/wallet/ledger?before=-1",
		"sisipan SQL":         "/wallet/ledger?before=1%20OR%201=1",
	}
	for name, path := range bad {
		t.Run(name, func(t *testing.T) {
			store := &fakeStore{entries: entriesDesc(3)}
			rec := serve(store, "u-1", path)
			if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `"error"`) {
				t.Errorf("status = %d body = %s, want 400", rec.Code, rec.Body)
			}
			if store.gotUser != "" {
				t.Error("input tidak valid tidak boleh sampai ke store")
			}
		})
	}
}

func TestHandlerReconcile(t *testing.T) {
	store := &fakeStore{report: &Report{Ok: false, Problems: []string{"x"}, Wallets: 3, Entries: 9}}
	rec := serve(store, "admin", "/admin/wallet/reconcile")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"ok":false`) || !strings.Contains(rec.Body.String(), `"wallets_checked":3`) {
		t.Errorf("status=%d body=%s", rec.Code, rec.Body)
	}
}

func TestHandlerHidesInternalErrors(t *testing.T) {
	store := &fakeStore{err: errors.New("pq: koneksi putus rahasia")}
	for _, path := range []string{"/wallet/", "/wallet/ledger", "/admin/wallet/reconcile"} {
		rec := serve(store, "u", path)
		if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "rahasia") {
			t.Errorf("%s: status=%d body=%s", path, rec.Code, rec.Body)
		}
	}
}
