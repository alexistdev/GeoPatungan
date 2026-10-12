/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package payment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/alexistdev/geopatungan/internal/contribution"
	"github.com/alexistdev/geopatungan/internal/httpx"
	"github.com/alexistdev/geopatungan/internal/round"
	"github.com/alexistdev/geopatungan/internal/wallet"
)

const cid = "11111111-1111-1111-1111-111111111111"

type fakeStore struct {
	result *Result
	quote  *QuoteResult
	err    error

	calls []string // "op user id"
}

func (f *fakeStore) Pay(_ context.Context, userID, id string) (*Result, error) {
	f.calls = append(f.calls, fmt.Sprintf("pay %s %s", userID, id))
	return f.result, f.err
}

func (f *fakeStore) Quote(_ context.Context, userID, id string) (*QuoteResult, error) {
	f.calls = append(f.calls, fmt.Sprintf("quote %s %s", userID, id))
	return f.quote, f.err
}

func newFake() *fakeStore {
	return &fakeStore{
		result: &Result{
			Receipt: &contribution.Receipt{ID: cid, GroupID: "g", RoundID: "r", UserID: "SECRET-USER",
				Amount: 100_000, PlatformFee: 2_500, LateFee: 0, Total: 102_500, PaidAt: time.Now(), PaidOnTime: true},
			Balance: 397_500,
			Round:   RoundOutcome{Completed: true, GroupDone: false, Payout: &Payout{RecipientName: "Budi", Amount: 300_000}},
		},
		quote: &QuoteResult{
			Quote:   &contribution.Quote{ContributionID: cid, Amount: 100_000, PlatformFee: 2_500, Total: 102_500, Payable: true},
			Balance: 500_000, Sufficient: true,
		},
	}
}

func serve(store Store, user, method, path string) (*httptest.ResponseRecorder, map[string]any) {
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			next.ServeHTTP(w, req.WithContext(httpx.WithUser(req.Context(), user, "member")))
		})
	})
	r.Route("/contributions", NewHandler(store).Routes)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	return rec, body
}

func TestHandlerPayResponseShape(t *testing.T) {
	st := newFake()
	rec, body := serve(st, "u-42", "POST", "/contributions/"+cid+"/pay")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (%s)", rec.Code, rec.Body)
	}
	if want := "pay u-42 " + cid; len(st.calls) != 1 || st.calls[0] != want {
		t.Errorf("store menerima %v, want %q (pembayar harus dari identitas login)", st.calls, want)
	}

	receipt := body["receipt"].(map[string]any)
	for _, key := range []string{"contribution_id", "group_id", "round_id", "amount", "platform_fee", "late_fee", "total", "paid_at", "paid_on_time"} {
		if _, ok := receipt[key]; !ok {
			t.Errorf("receipt.%s hilang", key)
		}
	}
	if receipt["total"].(float64) != 102_500 || body["balance"].(float64) != 397_500 {
		t.Errorf("receipt=%v balance=%v", receipt, body["balance"])
	}
	round := body["round"].(map[string]any)
	payout := round["payout"].(map[string]any)
	if round["completed"] != true || round["group_done"] != false || payout["recipient_name"] != "Budi" || payout["amount"].(float64) != 300_000 {
		t.Errorf("round = %v", round)
	}
	if strings.Contains(rec.Body.String(), "SECRET-USER") {
		t.Error("user_id pembayar tidak boleh bocor di respons")
	}
}

func TestHandlerPayWithoutPayoutHasNullPayout(t *testing.T) {
	st := newFake()
	st.result.Round = RoundOutcome{}
	rec, _ := serve(st, "u1", "POST", "/contributions/"+cid+"/pay")
	if !strings.Contains(rec.Body.String(), `"payout":null`) || !strings.Contains(rec.Body.String(), `"completed":false`) {
		t.Errorf("body = %s", rec.Body)
	}
}

func TestHandlerQuote(t *testing.T) {
	st := newFake()
	rec, body := serve(st, "u-9", "GET", "/contributions/"+cid+"/quote")
	if rec.Code != 200 || body["sufficient"] != true || body["balance"].(float64) != 500_000 {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body)
	}
	q := body["quote"].(map[string]any)
	if q["total"].(float64) != 102_500 || q["payable"] != true {
		t.Errorf("quote = %v", q)
	}
	if want := "quote u-9 " + cid; st.calls[0] != want {
		t.Errorf("store menerima %q, want %q", st.calls[0], want)
	}
}

func TestHandlerInsufficientFundsReturns402WithNumbers(t *testing.T) {
	st := newFake()
	st.err = &InsufficientFundsError{Required: 102_500, Balance: 100_000}
	rec, body := serve(st, "u1", "POST", "/contributions/"+cid+"/pay")
	if rec.Code != http.StatusPaymentRequired {
		t.Fatalf("status = %d, want 402", rec.Code)
	}
	if body["error"] != "saldo tidak cukup" || body["required"].(float64) != 102_500 || body["balance"].(float64) != 100_000 {
		t.Errorf("body = %v", body)
	}
	// juga ketika dibungkus error lain
	st.err = fmt.Errorf("membayar: %w", &InsufficientFundsError{Required: 5, Balance: 1})
	if rec, _ := serve(st, "u1", "POST", "/contributions/"+cid+"/pay"); rec.Code != http.StatusPaymentRequired {
		t.Errorf("error terbungkus: status = %d, want 402", rec.Code)
	}
}

func TestHandlerMapsErrors(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want int
		msg  string
	}{
		{contribution.ErrNotFound, 404, "iuran tidak ditemukan"},
		{contribution.ErrAlreadyPaid, 409, "sudah dibayar"},
		{round.ErrRoundNotCollecting, 409, "belum dibuka atau sudah ditutup"},
		{wallet.ErrAlreadyPosted, 500, "kesalahan pada server"},
		{errors.New("pq: koneksi putus rahasia"), 500, "kesalahan pada server"},
	} {
		for _, c := range []struct{ method, path string }{
			{"POST", "/contributions/" + cid + "/pay"},
			{"GET", "/contributions/" + cid + "/quote"},
		} {
			st := newFake()
			st.err = tc.err
			rec, _ := serve(st, "u1", c.method, c.path)
			if rec.Code != tc.want || !strings.Contains(rec.Body.String(), tc.msg) {
				t.Errorf("%s %s dengan %v: status=%d body=%s, want %d memuat %q", c.method, c.path, tc.err, rec.Code, rec.Body, tc.want, tc.msg)
			}
			if strings.Contains(rec.Body.String(), "rahasia") {
				t.Errorf("detail internal bocor: %s", rec.Body)
			}
		}
	}
}

func TestHandlerRejectsInvalidIDBeforeStore(t *testing.T) {
	for _, path := range []string{
		"/contributions/abc/pay", "/contributions/abc/quote",
		"/contributions/1'%20OR%20'1'='1/pay", "/contributions/../pay",
	} {
		st := newFake()
		method := "POST"
		if strings.HasSuffix(path, "quote") {
			method = "GET"
		}
		rec, _ := serve(st, "u1", method, path)
		if rec.Code != http.StatusNotFound || len(st.calls) != 0 {
			t.Errorf("%s: status=%d calls=%v, want 404 tanpa menyentuh store", path, rec.Code, st.calls)
		}
	}
}

func TestHandlerOnlyPostPaysAndGetQuotes(t *testing.T) {
	st := newFake()
	if rec, _ := serve(st, "u1", "GET", "/contributions/"+cid+"/pay"); rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET /pay: %d, want 405 (membayar tidak boleh lewat GET)", rec.Code)
	}
	if rec, _ := serve(st, "u1", "POST", "/contributions/"+cid+"/quote"); rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /quote: %d, want 405", rec.Code)
	}
	if len(st.calls) != 0 {
		t.Errorf("store terpanggil: %v", st.calls)
	}
}

func TestInsufficientFundsErrorIs(t *testing.T) {
	err := error(&InsufficientFundsError{Required: 10, Balance: 5})
	if !errors.Is(err, wallet.ErrInsufficientFunds) {
		t.Error("harus bisa dikenali sebagai wallet.ErrInsufficientFunds")
	}
	if !strings.Contains(err.Error(), "10") || !strings.Contains(err.Error(), "5") {
		t.Errorf("pesan = %q", err.Error())
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("abc", 5); got != "abc" {
		t.Errorf("got %q", got)
	}
	if got := truncate(strings.Repeat("x", 300), 200); len(got) != 200 {
		t.Errorf("len = %d, want 200", len(got))
	}
	if got := truncate("abcde", 5); got != "abcde" {
		t.Errorf("tepat batas: %q", got)
	}
}
