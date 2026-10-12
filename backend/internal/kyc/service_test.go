/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package kyc

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
)

type fakeStore struct {
	req    *Request
	status *Status
	admin  []AdminRequest
	err    error

	calls    []string
	lastNote string
	decision string
	st       string
	limit    int
}

func (f *fakeStore) rec(format string, a ...any) {
	f.calls = append(f.calls, fmt.Sprintf(format, a...))
}

func (f *fakeStore) Submit(_ context.Context, userID, note string) (*Request, error) {
	f.rec("submit %s", userID)
	f.lastNote = note
	return f.req, f.err
}
func (f *fakeStore) StatusOf(_ context.Context, userID string) (*Status, error) {
	f.rec("status %s", userID)
	return f.status, f.err
}
func (f *fakeStore) ListAdmin(_ context.Context, status string, limit int) ([]AdminRequest, error) {
	f.rec("list")
	f.st, f.limit = status, limit
	return f.admin, f.err
}
func (f *fakeStore) Review(_ context.Context, adminID, id, decision, note string) (*Request, error) {
	f.rec("review %s %s", adminID, id)
	f.decision, f.lastNote = decision, note
	return f.req, f.err
}

const rid = "11111111-1111-1111-1111-111111111111"

func newFixture() (*Service, *fakeStore) {
	st := &fakeStore{
		req:    &Request{ID: rid, UserID: "u1", Status: StatusPending},
		status: &Status{KYCStatus: "none"},
	}
	return NewService(st), st
}

func TestServiceSubmitValidatesNote(t *testing.T) {
	svc, st := newFixture()
	if _, err := svc.Submit(context.Background(), "u1", "  mohon diverifikasi  "); err != nil {
		t.Fatal(err)
	}
	if st.lastNote != "mohon diverifikasi" {
		t.Errorf("catatan = %q (harus dipangkas)", st.lastNote)
	}
	if _, err := svc.Submit(context.Background(), "u1", strings.Repeat("a", 200)); err != nil {
		t.Errorf("batas tepat: %v", err)
	}

	svc, st = newFixture()
	_, err := svc.Submit(context.Background(), "u1", strings.Repeat("a", 201))
	var verr httpx.ValidationError
	if !errors.As(err, &verr) || len(st.calls) != 0 {
		t.Errorf("err = %v calls = %d", err, len(st.calls))
	}
}

func TestServiceDecisions(t *testing.T) {
	svc, st := newFixture()
	if _, err := svc.Approve(context.Background(), "adm", rid, "  data sesuai  "); err != nil {
		t.Fatal(err)
	}
	if st.decision != StatusVerified || st.lastNote != "data sesuai" {
		t.Errorf("approve: decision=%q note=%q", st.decision, st.lastNote)
	}
	if _, err := svc.Approve(context.Background(), "adm", rid, ""); err != nil {
		t.Errorf("approve tanpa catatan seharusnya sah: %v", err)
	}
	if _, err := svc.Approve(context.Background(), "adm", rid, strings.Repeat("a", 201)); err == nil {
		t.Error("catatan 201 karakter seharusnya ditolak")
	}

	svc, st = newFixture()
	if _, err := svc.Reject(context.Background(), "adm", rid, "foto tidak jelas"); err != nil {
		t.Fatal(err)
	}
	if st.decision != StatusRejected {
		t.Errorf("decision = %q", st.decision)
	}
	for name, note := range map[string]string{"kosong": "", "spasi": "   ", "2 huruf": "ab", "201": strings.Repeat("a", 201)} {
		svc, st := newFixture()
		_, err := svc.Reject(context.Background(), "adm", rid, note)
		var verr httpx.ValidationError
		if !errors.As(err, &verr) || len(st.calls) != 0 {
			t.Errorf("%s: err = %v calls = %d, want ValidationError tanpa menyentuh store", name, err, len(st.calls))
		}
	}
}

func TestServiceListAdminValidation(t *testing.T) {
	svc, st := newFixture()
	if _, err := svc.ListAdmin(context.Background(), "", 0); err != nil {
		t.Fatal(err)
	}
	if st.st != StatusPending || st.limit != DefaultListLimit {
		t.Errorf("bawaan = %q/%d", st.st, st.limit)
	}
	for _, s := range []string{"pending", "verified", "rejected", "all"} {
		if _, err := NewService(&fakeStore{}).ListAdmin(context.Background(), s, 5); err != nil {
			t.Errorf("status %q: %v", s, err)
		}
	}
	for _, s := range []string{"approved", "PENDING", "none", "all;DROP"} {
		svc, st := newFixture()
		if _, err := svc.ListAdmin(context.Background(), s, 5); err == nil || len(st.calls) != 0 {
			t.Errorf("status %q: err = %v", s, err)
		}
	}
	for _, l := range []int{-1, 201} {
		if _, err := svc.ListAdmin(context.Background(), "all", l); err == nil {
			t.Errorf("limit %d seharusnya ditolak", l)
		}
	}
}

func serve(svc *Service, user, method, path, body string) (*httptest.ResponseRecorder, map[string]any) {
	h := NewHandler(svc)
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			next.ServeHTTP(w, req.WithContext(httpx.WithUser(req.Context(), user, "member")))
		})
	})
	r.Route("/me/kyc", h.Routes)
	r.Route("/admin", h.AdminRoutes)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec, out
}

func TestHandlerUserAndAdminFlow(t *testing.T) {
	svc, st := newFixture()

	for _, body := range []string{``, `{}`, `{"note":"mohon"}`} {
		if rec, _ := serve(svc, "u-5", "POST", "/me/kyc/", body); rec.Code != http.StatusCreated {
			t.Errorf("submit body %q: %d %s", body, rec.Code, rec.Body)
		}
	}
	if st.calls[0] != "submit u-5" {
		t.Errorf("pemohon harus dari identitas login: %v", st.calls)
	}

	st.status = &Status{KYCStatus: "pending", Latest: st.req}
	rec, body := serve(svc, "u-5", "GET", "/me/kyc/", "")
	if rec.Code != 200 || body["kyc_status"] != "pending" || body["latest_request"] == nil {
		t.Errorf("status: %d %s", rec.Code, rec.Body)
	}

	st.admin = []AdminRequest{{Request: *st.req, UserName: "Budi", UserEmail: "budi@example.com"}}
	rec, body = serve(svc, "adm", "GET", "/admin/kyc?limit=10", "")
	if rec.Code != 200 || st.st != StatusPending || st.limit != 10 {
		t.Fatalf("list admin: %d %s", rec.Code, rec.Body)
	}
	if first := body["requests"].([]any)[0].(map[string]any); first["user_name"] != "Budi" || first["user_email"] != "budi@example.com" {
		t.Errorf("identitas pemohon: %v", first)
	}

	for _, b := range []string{``, `{}`, `{"note":"data sesuai"}`} {
		if rec, _ := serve(svc, "adm", "POST", "/admin/kyc/"+rid+"/approve", b); rec.Code != 200 {
			t.Errorf("approve body %q: %d %s", b, rec.Code, rec.Body)
		}
	}
	if rec, _ := serve(svc, "adm", "POST", "/admin/kyc/"+rid+"/reject", `{"note":"foto tidak jelas"}`); rec.Code != 200 {
		t.Errorf("reject: %d %s", rec.Code, rec.Body)
	}
}

func TestHandlerRejectsBadInputBeforeStore(t *testing.T) {
	svc, st := newFixture()
	for name, tc := range map[string]struct {
		method, path, body string
		want               int
	}{
		"submit: field asing":  {"POST", "/me/kyc/", `{"note":"x","status":"verified"}`, 400},
		"submit: bukan JSON":   {"POST", "/me/kyc/", `x`, 400},
		"list: status asing":   {"GET", "/admin/kyc?status=approved", ``, 400},
		"list: limit huruf":    {"GET", "/admin/kyc?limit=abc", ``, 400},
		"approve: bukan UUID":  {"POST", "/admin/kyc/abc/approve", `{}`, 404},
		"approve: field asing": {"POST", "/admin/kyc/" + rid + "/approve", `{"status":"verified"}`, 400},
		"reject: tanpa alasan": {"POST", "/admin/kyc/" + rid + "/reject", `{}`, 400},
		"reject: tanpa body":   {"POST", "/admin/kyc/" + rid + "/reject", ``, 400},
		"reject: bukan UUID":   {"POST", "/admin/kyc/abc/reject", `{"note":"alasan"}`, 404},
	} {
		before := len(st.calls)
		rec, body := serve(svc, "u1", tc.method, tc.path, tc.body)
		if rec.Code != tc.want || body["error"] == nil {
			t.Errorf("%s: status = %d body = %s, want %d", name, rec.Code, rec.Body, tc.want)
		}
		if len(st.calls) != before {
			t.Errorf("%s: tidak boleh menyentuh store", name)
		}
	}
}

func TestHandlerMapsDomainErrors(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want int
	}{
		{ErrNotFound, 404}, {ErrSelfReview, 403}, {ErrAlreadyPending, 409}, {ErrAlreadyVerified, 409},
		{ErrInvalidState, 409}, {errors.New("pq: koneksi putus rahasia"), 500},
	} {
		for _, c := range []struct{ user, method, path, body string }{
			{"u1", "POST", "/me/kyc/", `{}`},
			{"u1", "GET", "/me/kyc/", ``},
			{"a", "POST", "/admin/kyc/" + rid + "/approve", `{}`},
			{"a", "POST", "/admin/kyc/" + rid + "/reject", `{"note":"alasan"}`},
		} {
			svc, st := newFixture()
			st.err = tc.err
			rec, _ := serve(svc, c.user, c.method, c.path, c.body)
			if rec.Code != tc.want || strings.Contains(rec.Body.String(), "rahasia") {
				t.Errorf("%s %s dengan %v: status = %d body = %s, want %d", c.method, c.path, tc.err, rec.Code, rec.Body, tc.want)
			}
		}
	}
}
