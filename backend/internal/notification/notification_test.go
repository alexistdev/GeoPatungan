/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package notification

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

func TestRupiah(t *testing.T) {
	for in, want := range map[int64]string{
		0: "Rp0", 5: "Rp5", 999: "Rp999", 1000: "Rp1.000", 12345: "Rp12.345", 100000: "Rp100.000",
		1234567: "Rp1.234.567", -1500: "-Rp1.500", 9223372036854775807: "Rp9.223.372.036.854.775.807",
		-9223372036854775808: "-Rp9.223.372.036.854.775.808",
	} {
		if got := Rupiah(in); got != want {
			t.Errorf("Rupiah(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestWithNote(t *testing.T) {
	if got := WithNote("Ditolak.", ""); got != "Ditolak." {
		t.Errorf("tanpa catatan: %q", got)
	}
	if got := WithNote("Ditolak.", "  bukti buram  "); got != "Ditolak. Catatan: bukti buram" {
		t.Errorf("dengan catatan: %q", got)
	}
	long := WithNote("Ditolak.", strings.Repeat("é", 2000))
	if n := len([]rune(long)); n != MaxBody {
		t.Errorf("panjang = %d rune, want %d", n, MaxBody)
	}
	if !strings.HasSuffix(long, "…") {
		t.Error("pemotongan harus ditandai")
	}
}

func TestNoticeValidate(t *testing.T) {
	ok := Notice{UserID: "u", Type: "x", Title: "Judul", Body: "Isi"}
	if err := ok.Validate(); err != nil {
		t.Fatalf("valid: %v", err)
	}
	for name, mut := range map[string]func(*Notice){
		"tanpa penerima":  func(n *Notice) { n.UserID = "" },
		"tanpa jenis":     func(n *Notice) { n.Type = " " },
		"judul kosong":    func(n *Notice) { n.Title = "  " },
		"judul kepanjang": func(n *Notice) { n.Title = strings.Repeat("a", MaxTitle+1) },
		"isi kosong":      func(n *Notice) { n.Body = "" },
		"isi kepanjang":   func(n *Notice) { n.Body = strings.Repeat("a", MaxBody+1) },
	} {
		n := ok
		mut(&n)
		if err := n.Validate(); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", name, err)
		}
	}
	// batas tepat diperbolehkan, dihitung dalam rune
	edge := ok
	edge.Title, edge.Body = strings.Repeat("é", MaxTitle), strings.Repeat("é", MaxBody)
	if err := edge.Validate(); err != nil {
		t.Errorf("batas tepat: %v", err)
	}
}

// ---- service & handler (fake store) ---------------------------------------

type fakeStore struct {
	items  []Item
	unread int
	err    error

	gotLimit  int
	gotBefore int64
	gotUnread bool
	read      map[string]map[int64]bool // user -> id
	readAll   int
}

func (f *fakeStore) List(_ context.Context, _ string, limit int, before int64, unreadOnly bool) ([]Item, error) {
	f.gotLimit, f.gotBefore, f.gotUnread = limit, before, unreadOnly
	if f.err != nil {
		return nil, f.err
	}
	out := []Item{}
	for _, it := range f.items {
		if before == 0 || it.ID < before {
			out = append(out, it)
		}
		if len(out) == limit+1 {
			break
		}
	}
	return out, nil
}
func (f *fakeStore) UnreadCount(context.Context, string) (int, error) { return f.unread, f.err }
func (f *fakeStore) MarkRead(_ context.Context, user string, id int64) error {
	if f.err != nil {
		return f.err
	}
	if id != 7 {
		return ErrNotFound
	}
	if f.read == nil {
		f.read = map[string]map[int64]bool{}
	}
	if f.read[user] == nil {
		f.read[user] = map[int64]bool{}
	}
	f.read[user][id] = true
	return nil
}
func (f *fakeStore) MarkAllRead(context.Context, string) (int64, error) {
	f.readAll++
	return 3, f.err
}

func items(n int) []Item {
	out := make([]Item, n)
	for i := range out {
		out[i] = Item{ID: int64(n - i), Type: "x", Title: "T", Body: "B", Data: map[string]any{}, CreatedAt: time.Now()}
	}
	return out
}

func TestServiceListPaging(t *testing.T) {
	st := &fakeStore{items: items(5), unread: 4}
	svc := NewService(st)
	p, err := svc.List(context.Background(), "u", 2, 0, false)
	if err != nil || len(p.Notifications) != 2 || p.NextBefore == nil || *p.NextBefore != p.Notifications[1].ID || p.UnreadCount != 4 {
		t.Fatalf("p = %+v, %v", p, err)
	}
	seen := map[int64]bool{}
	var before int64
	for i := 0; i < 10; i++ {
		pg, _ := svc.List(context.Background(), "u", 2, before, false)
		for _, it := range pg.Notifications {
			if seen[it.ID] {
				t.Fatalf("ganda: %d", it.ID)
			}
			seen[it.ID] = true
		}
		if pg.NextBefore == nil {
			break
		}
		before = *pg.NextBefore
	}
	if len(seen) != 5 {
		t.Errorf("terbaca %d, want 5", len(seen))
	}
	st.items = items(2)
	if p, _ := svc.List(context.Background(), "u", 2, 0, false); p.NextBefore != nil || len(p.Notifications) != 2 {
		t.Errorf("tepat limit: %+v", p)
	}
	st.items = nil
	if p, _ := svc.List(context.Background(), "u", 5, 0, false); p.Notifications == nil || len(p.Notifications) != 0 {
		t.Errorf("kosong harus []: %+v", p)
	}
}

func TestServiceListValidatesParams(t *testing.T) {
	for _, tc := range []struct {
		limit  int
		before int64
		bad    bool
	}{{0, 0, false}, {100, 0, false}, {101, 0, true}, {-1, 0, true}, {5, -1, true}} {
		st := &fakeStore{}
		_, err := NewService(st).List(context.Background(), "u", tc.limit, tc.before, false)
		var v httpx.ValidationError
		if tc.bad != errors.As(err, &v) {
			t.Errorf("limit %d before %d: err = %v", tc.limit, tc.before, err)
		}
		if tc.bad && st.gotLimit != 0 {
			t.Error("input tidak valid tidak boleh sampai ke store")
		}
	}
	st := &fakeStore{}
	_, _ = NewService(st).List(context.Background(), "u", 0, 0, false)
	if st.gotLimit != DefaultListLimit {
		t.Errorf("limit bawaan = %d", st.gotLimit)
	}
}

func call(st *fakeStore, user, method, path string) (*httptest.ResponseRecorder, map[string]any) {
	h := NewHandler(NewService(st))
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			next.ServeHTTP(w, req.WithContext(httpx.WithUser(req.Context(), user, "member")))
		})
	})
	r.Route("/me/notifications", h.Routes)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec, out
}

func TestHandlerList(t *testing.T) {
	st := &fakeStore{items: items(3), unread: 2}
	rec, body := call(st, "u1", "GET", "/me/notifications/?limit=2&before=3&unread=true")
	if rec.Code != 200 || st.gotLimit != 2 || st.gotBefore != 3 || !st.gotUnread {
		t.Fatalf("status=%d limit=%d before=%d unread=%v (%s)", rec.Code, st.gotLimit, st.gotBefore, st.gotUnread, rec.Body)
	}
	if body["unread_count"].(float64) != 2 {
		t.Errorf("body = %v", body)
	}
	first := body["notifications"].([]any)[0].(map[string]any)
	for _, k := range []string{"id", "type", "title", "body", "data", "read", "created_at"} {
		if _, ok := first[k]; !ok {
			t.Errorf("field %q hilang", k)
		}
	}
	for name, path := range map[string]string{
		"limit huruf": "/me/notifications/?limit=x", "before huruf": "/me/notifications/?before=x",
		"limit besar": "/me/notifications/?limit=999", "unread aneh": "/me/notifications/?unread=yes",
	} {
		if rec, _ := call(&fakeStore{}, "u1", "GET", path); rec.Code != 400 {
			t.Errorf("%s: %d, want 400", name, rec.Code)
		}
	}
	st = &fakeStore{err: errors.New("pq: rahasia koneksi")}
	if rec, _ := call(st, "u1", "GET", "/me/notifications/"); rec.Code != 500 || strings.Contains(rec.Body.String(), "rahasia") {
		t.Errorf("kegagalan db: %d %s", rec.Code, rec.Body)
	}
}

func TestHandlerUnreadCountAndReads(t *testing.T) {
	st := &fakeStore{unread: 9}
	if rec, b := call(st, "u1", "GET", "/me/notifications/unread-count"); rec.Code != 200 || b["unread_count"].(float64) != 9 {
		t.Errorf("unread-count: %d %v", rec.Code, b)
	}
	if rec, b := call(st, "u1", "POST", "/me/notifications/7/read"); rec.Code != 200 || b["read"] != true || !st.read["u1"][7] {
		t.Errorf("read: %d %v", rec.Code, b)
	}
	for name, path := range map[string]string{"tidak ada": "/me/notifications/8/read", "bukan angka": "/me/notifications/abc/read", "nol": "/me/notifications/0/read", "negatif": "/me/notifications/-3/read", "kebesaran": "/me/notifications/99999999999999999999/read"} {
		if rec, _ := call(st, "u1", "POST", path); rec.Code != 404 {
			t.Errorf("%s: %d, want 404", name, rec.Code)
		}
	}
	if rec, b := call(st, "u1", "POST", "/me/notifications/read-all"); rec.Code != 200 || b["updated"].(float64) != 3 || st.readAll != 1 {
		t.Errorf("read-all: %d %v", rec.Code, b)
	}
	// metode salah
	if rec, _ := call(st, "u1", "GET", "/me/notifications/7/read"); rec.Code != 405 {
		t.Errorf("GET read: %d, want 405", rec.Code)
	}
	if rec, _ := call(st, "u1", "POST", "/me/notifications/unread-count"); rec.Code != 405 {
		t.Errorf("POST unread-count: %d, want 405", rec.Code)
	}
}
