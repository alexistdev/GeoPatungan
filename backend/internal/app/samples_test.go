/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/alexistdev/geopatungan/internal/auth"
	"github.com/alexistdev/geopatungan/internal/testutil"
)

// Tes ini menjalankan satu alur pemakaian lengkap melalui router ASLI (HTTP sungguhan, database
// sungguhan) dan merekam BENTUK setiap respons (nama field + tipe, bukan nilainya) ke
// contract/samples.json. Tes frontend memeriksa bahwa setiap field yang dibaca UI memang ada
// dengan tipe yang benar. Bentuk yang berubah tanpa memperbarui kontrak membuat tes ini gagal.

// ---- bentuk -----------------------------------------------------------------------------

func shapeOf(v any) any {
	switch x := v.(type) {
	case nil:
		return "null"
	case bool:
		return "boolean"
	case float64:
		return "number"
	case string:
		return "string"
	case []any:
		var merged any
		for _, e := range x {
			if merged == nil {
				merged = shapeOf(e)
			} else {
				merged = mergeShape(merged, shapeOf(e))
			}
		}
		if merged == nil {
			merged = "unknown" // array kosong: isi tidak diketahui
		}
		return []any{merged}
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, e := range x {
			out[k] = shapeOf(e)
		}
		return out
	}
	return fmt.Sprintf("?%T", v)
}

// mergeShape menggabungkan dua bentuk dari respons berbeda pada endpoint yang sama: tipe skalar yang
// berbeda menjadi gabungan ("string|null"), field yang hanya ada di salah satunya ditandai "|absent".
// Objek/array yang kadang null atau tidak ada dibungkus {"$optional": bentuk, "$when": "null"|"absent"|"absent|null"}.
func mergeShape(a, b any) any {
	if reflect.DeepEqual(a, b) {
		return a
	}
	wa, oka := wrapper(a)
	wb, okb := wrapper(b)
	switch {
	case oka && okb:
		return optional(mergeShape(wa.inner, wb.inner), joinWhen(wa.when, wb.when))
	case oka:
		if marker, ok := b.(string); ok && (marker == "null" || marker == "absent") {
			return optional(wa.inner, joinWhen(wa.when, marker))
		}
		return optional(mergeShape(wa.inner, b), wa.when)
	case okb:
		if marker, ok := a.(string); ok && (marker == "null" || marker == "absent") {
			return optional(wb.inner, joinWhen(wb.when, marker))
		}
		return optional(mergeShape(a, wb.inner), wb.when)
	}
	switch x := a.(type) {
	case map[string]any:
		if y, ok := b.(map[string]any); ok {
			out := map[string]any{}
			for k, v := range x {
				if w, ok := y[k]; ok {
					out[k] = mergeShape(v, w)
				} else {
					out[k] = union(v, "absent")
				}
			}
			for k, w := range y {
				if _, ok := x[k]; !ok {
					out[k] = union(w, "absent")
				}
			}
			return out
		}
	case []any:
		if y, ok := b.([]any); ok {
			if x[0] == "unknown" {
				return y
			}
			if y[0] == "unknown" {
				return x
			}
			return []any{mergeShape(x[0], y[0])}
		}
	case string:
		if y, ok := b.(string); ok {
			if x == "unknown" {
				return y
			}
			if y == "unknown" {
				return x
			}
			return union(x, y)
		}
	}
	return union(a, b)
}

type wrapped struct {
	inner any
	when  string
}

func optional(inner any, when string) any {
	return map[string]any{"$optional": inner, "$when": when}
}

func wrapper(v any) (wrapped, bool) {
	m, ok := v.(map[string]any)
	if !ok || len(m) != 2 {
		return wrapped{}, false
	}
	in, ok1 := m["$optional"]
	w, ok2 := m["$when"].(string)
	if !ok1 || !ok2 {
		return wrapped{}, false
	}
	return wrapped{in, w}, true
}

func joinWhen(a, b string) string {
	set := map[string]bool{}
	for _, p := range append(strings.Split(a, "|"), strings.Split(b, "|")...) {
		set[p] = true
	}
	var parts []string
	for p := range set {
		parts = append(parts, p)
	}
	sort.Strings(parts)
	return strings.Join(parts, "|")
}

// union membentuk "a|b" untuk skalar. Jika salah satunya "null"/"absent" dan yang lain objek/array,
// hasilnya pembungkus $optional supaya bentuk bersarang tidak hilang dan alasannya (null/absent) jelas.
func union(a, b any) any {
	sa, oka := a.(string)
	sb, okb := b.(string)
	if w, ok := wrapper(a); ok && okb && (sb == "null" || sb == "absent") {
		return optional(w.inner, joinWhen(w.when, sb)) // sudah terbungkus: tambah alasan, jangan bungkus ganda
	}
	if w, ok := wrapper(b); ok && oka && (sa == "null" || sa == "absent") {
		return optional(w.inner, joinWhen(w.when, sa))
	}
	if oka && !okb && (sa == "null" || sa == "absent") {
		return optional(b, sa)
	}
	if okb && !oka && (sb == "null" || sb == "absent") {
		return optional(a, sb)
	}
	if !oka || !okb {
		return a // dua bentuk bersarang yang tak bisa digabung: pertahankan yang pertama
	}
	return joinWhen(sa, sb)
}

// ---- perekam ----------------------------------------------------------------------------

type journey struct {
	t        *testing.T
	base     string
	shapes   map[string]any // respons: "METODE /rute STATUS" -> bentuk
	requests map[string]any // permintaan: "METODE /rute" -> bentuk body (gabungan semua pemanggilan 2xx)
}

// do memanggil API asli. tmpl adalah pola rute (sama dengan contract/routes.json); status harus
// sesuai harapan. Mengembalikan JSON yang sudah di-decode.
func (j *journey) do(token, method, tmpl, path string, body any, want int) any {
	j.t.Helper()
	var rd io.Reader
	var reqShape any
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
		var decoded any
		_ = json.Unmarshal(b, &decoded) // bentuk dihitung dari JSON, bukan dari tipe Go
		reqShape = shapeOf(decoded)
	}
	req, _ := http.NewRequest(method, j.base+path, rd)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		j.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != want {
		j.t.Fatalf("%s %s: status %d, want %d: %s", method, path, resp.StatusCode, want, raw)
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		j.t.Fatalf("%s %s: bukan JSON: %v (%s)", method, path, err, raw)
	}
	if reqShape != nil && resp.StatusCode < 300 {
		rk := method + " " + tmpl
		if prev, ok := j.requests[rk]; ok {
			reqShape = mergeShape(prev, reqShape)
		}
		j.requests[rk] = reqShape
	}
	key := fmt.Sprintf("%s %s %d", method, tmpl, resp.StatusCode)
	sh := shapeOf(v)
	if prev, ok := j.shapes[key]; ok {
		sh = mergeShape(prev, sh)
	}
	j.shapes[key] = sh
	return v
}

func obj(v any) map[string]any {
	m, ok := v.(map[string]any)
	if !ok {
		panic(fmt.Sprintf("bukan objek: %T", v))
	}
	return m
}
func str(m map[string]any, keys ...string) string {
	var cur any = m
	for _, k := range keys {
		cur = obj(cur)[k]
	}
	s, ok := cur.(string)
	if !ok {
		panic(fmt.Sprintf("bukan string di %v: %v", keys, cur))
	}
	return s
}
func list(v any, key string) []any { return obj(v)[key].([]any) }

// ---- alur -------------------------------------------------------------------------------

func TestRecordAPIShapes(t *testing.T) {
	e := testutil.NewEnv(t)
	ctx := context.Background()
	tokens := auth.NewTokenManager("rahasia-uji-yang-cukup-panjang-minimal-32-karakter", time.Hour)
	srv := httptest.NewServer(NewRouter(e.Pool, tokens))
	t.Cleanup(srv.Close)
	j := &journey{t: t, base: srv.URL, shapes: map[string]any{}, requests: map[string]any{}}

	phoneSeq := time.Now().UnixNano() % 1_000_000_000
	register := func(name, tag, ref string) (token, code, id string) {
		phoneSeq++
		body := map[string]any{"name": name, "phone": fmt.Sprintf("08%010d", phoneSeq), "email": fmt.Sprintf("%s-%s@example.com", e.Prefix, tag), "password": "Password123"}
		if ref != "" {
			body["referral_code"] = ref
		}
		r := j.do("", "POST", "/api/auth/register", "/api/auth/register", body, 201)
		return str(obj(r), "token"), str(obj(r), "user", "referral_code"), str(obj(r), "user", "id")
	}

	// ---- registrasi, login, galat umum
	j.do("", "POST", "/api/auth/register", "/api/auth/register", map[string]any{"name": "X", "phone": "1", "email": "x", "password": "1"}, 400)
	j.do("", "POST", "/api/auth/register", "/api/auth/register", map[string]any{"name": "Zz", "phone": "081234567890", "email": e.Prefix + "-ref@example.com", "password": "Password123", "referral_code": "TIDAKADA1"}, 400)
	tA, codeA, idA := register("Andi Referrer", "a", "")
	tB, _, idB := register("Budi Santoso", "b", codeA)
	tC, _, _ := register("Cici", "c", "")
	tD, _, _ := register("Dewi Joiner", "d", "")
	tF, _, _ := register("Fani Tanpa Saldo", "f", "")
	tAdm, _, idAdm := register("Admin Uji", "adm", "")
	e.Exec(`UPDATE users SET role = 'admin' WHERE id = $1`, idAdm)
	_ = idB
	j.do("", "POST", "/api/auth/login", "/api/auth/login", map[string]any{"email": e.Prefix + "-a@example.com", "password": "Password123"}, 200)
	j.do("", "POST", "/api/auth/login", "/api/auth/login", map[string]any{"email": e.Prefix + "-a@example.com", "password": "salah"}, 401)
	j.do("", "GET", "/api/me", "/api/me", nil, 401)
	j.do(tA, "GET", "/api/admin/settings", "/api/admin/settings", nil, 403)

	// ---- keadaan awal pengguna biasa
	j.do(tA, "GET", "/api/me", "/api/me", nil, 200)
	j.do(tA, "GET", "/api/payment-info", "/api/payment-info", nil, 200)
	j.do(tA, "GET", "/api/wallet/", "/api/wallet/", nil, 200)
	j.do(tA, "GET", "/api/wallet/ledger", "/api/wallet/ledger", nil, 200)
	j.do(tA, "GET", "/api/wallet/topups", "/api/wallet/topups", nil, 200)
	j.do(tA, "GET", "/api/wallet/withdrawals", "/api/wallet/withdrawals", nil, 200)
	j.do(tA, "GET", "/api/me/contributions", "/api/me/contributions", nil, 200)
	j.do(tA, "GET", "/api/me/reputation", "/api/me/reputation", nil, 200)
	j.do(tA, "GET", "/api/me/kyc/", "/api/me/kyc/", nil, 200)
	j.do(tA, "GET", "/api/me/referral/", "/api/me/referral/", nil, 200)
	j.do(tA, "GET", "/api/me/referral/commissions", "/api/me/referral/commissions", nil, 200)
	j.do(tA, "GET", "/api/groups/", "/api/groups/", nil, 200)

	// ---- admin: pengaturan, rekening, aturan
	orig := obj(j.do(tAdm, "GET", "/api/admin/settings", "/api/admin/settings", nil, 200))
	t.Cleanup(func() { // pulihkan pengaturan platform dan rekening uji
		b, _ := json.Marshal(orig)
		req, _ := http.NewRequest("PUT", srv.URL+"/api/admin/settings", bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+tAdm)
		if resp, err := http.DefaultClient.Do(req); err == nil {
			resp.Body.Close()
		}
		_, _ = e.Pool.Exec(ctx, `DELETE FROM bank_accounts WHERE account_holder = $1`, e.Prefix)
	})
	settings := map[string]any{}
	for k, v := range orig {
		settings[k] = v
	}
	settings["platform_fee_bps"] = 200 // agar komisi referral terbentuk
	j.do(tAdm, "PUT", "/api/admin/settings", "/api/admin/settings", settings, 200)
	bank := map[string]any{"bank_name": "BCA", "account_number": "1234567890", "account_holder": e.Prefix, "notes": "uji", "is_active": true}
	bk := j.do(tAdm, "POST", "/api/admin/bank-accounts", "/api/admin/bank-accounts", bank, 201)
	bank["notes"] = "uji ubah"
	j.do(tAdm, "PUT", "/api/admin/bank-accounts/{id}", "/api/admin/bank-accounts/"+str(obj(bk), "id"), bank, 200)
	j.do(tAdm, "GET", "/api/admin/bank-accounts", "/api/admin/bank-accounts", nil, 200)
	rules := j.do(tAdm, "GET", "/api/admin/referral/rules", "/api/admin/referral/rules", nil, 200)
	j.do(tAdm, "PUT", "/api/admin/referral/rules", "/api/admin/referral/rules", rules, 200)
	levels := j.do(tAdm, "GET", "/api/admin/reputation/levels", "/api/admin/reputation/levels", nil, 200)
	j.do(tAdm, "PUT", "/api/admin/reputation/levels", "/api/admin/reputation/levels", levels, 200)
	j.do(tAdm, "GET", "/api/admin/wallet/reconcile", "/api/admin/wallet/reconcile", nil, 200)
	j.do(tAdm, "GET", "/api/admin/topups", "/api/admin/topups?status=all", nil, 200)
	j.do(tAdm, "GET", "/api/admin/withdrawals", "/api/admin/withdrawals?status=all", nil, 200)
	j.do(tAdm, "GET", "/api/admin/kyc", "/api/admin/kyc?status=all", nil, 200)

	// ---- top-up: buat, konfirmasi, setujui; batalkan; tolak
	fund := func(token string, amount int, approveBody map[string]any) {
		tp := obj(j.do(token, "POST", "/api/wallet/topups", "/api/wallet/topups", map[string]any{"amount": amount}, 201))
		id := str(tp, "topup", "id")
		j.do(token, "POST", "/api/wallet/topups/{id}/confirm", "/api/wallet/topups/"+id+"/confirm", map[string]any{"sender_name": "Pengirim", "sender_bank": "BCA", "note": ""}, 200)
		j.do(tAdm, "POST", "/api/admin/topups/{id}/approve", "/api/admin/topups/"+id+"/approve", approveBody, 200)
	}
	fund(tA, 1_000_000, map[string]any{"note": ""})
	fund(tB, 1_000_000, map[string]any{"note": ""})
	fund(tC, 1_000_000, map[string]any{"received_amount": 1_000_000, "note": "sesuai"}) // jalur received_amount
	cancel := obj(j.do(tA, "POST", "/api/wallet/topups", "/api/wallet/topups", map[string]any{"amount": 20000}, 201))
	j.do(tA, "POST", "/api/wallet/topups/{id}/cancel", "/api/wallet/topups/"+str(cancel, "topup", "id")+"/cancel", nil, 200)
	rej := obj(j.do(tA, "POST", "/api/wallet/topups", "/api/wallet/topups", map[string]any{"amount": 30000}, 201))
	j.do(tAdm, "POST", "/api/admin/topups/{id}/reject", "/api/admin/topups/"+str(rej, "topup", "id")+"/reject", map[string]any{"note": "bukti tidak ada"}, 200)
	j.do(tA, "GET", "/api/wallet/topups", "/api/wallet/topups", nil, 200)
	j.do(tAdm, "GET", "/api/admin/topups", "/api/admin/topups?status=all", nil, 200)
	j.do(tA, "POST", "/api/wallet/topups", "/api/wallet/topups", map[string]any{"amount": 1}, 400)

	// ---- penarikan: batalkan, selesaikan, tolak, saldo tidak cukup
	wd := func(amount int) string {
		w := j.do(tA, "POST", "/api/wallet/withdrawals", "/api/wallet/withdrawals", map[string]any{"amount": amount, "bank_name": "BCA", "account_number": "1234567890", "account_holder": "Andi", "note": ""}, 201)
		return str(obj(w), "withdrawal", "id")
	}
	w1 := wd(60000)
	j.do(tA, "POST", "/api/wallet/withdrawals/{id}/cancel", "/api/wallet/withdrawals/"+w1+"/cancel", nil, 200)
	w2 := wd(60000)
	j.do(tAdm, "POST", "/api/admin/withdrawals/{id}/complete", "/api/admin/withdrawals/"+w2+"/complete", map[string]any{"reference": "REF-12345", "note": ""}, 200)
	w3 := wd(60000)
	j.do(tAdm, "POST", "/api/admin/withdrawals/{id}/reject", "/api/admin/withdrawals/"+w3+"/reject", map[string]any{"note": "rekening salah"}, 200)
	j.do(tA, "POST", "/api/wallet/withdrawals", "/api/wallet/withdrawals", map[string]any{"amount": 90_000_000, "bank_name": "BCA", "account_number": "1234567890", "account_holder": "Andi", "note": ""}, 402)
	j.do(tA, "GET", "/api/wallet/withdrawals", "/api/wallet/withdrawals", nil, 200)
	j.do(tAdm, "GET", "/api/admin/withdrawals", "/api/admin/withdrawals?status=all", nil, 200)
	j.do(tA, "GET", "/api/wallet/ledger", "/api/wallet/ledger", nil, 200)

	// ---- KYC: ajukan, tolak, ajukan ulang, setujui (Andi terverifikasi agar menerima komisi)
	j.do(tA, "POST", "/api/me/kyc/", "/api/me/kyc/", map[string]any{"note": "ktp"}, 201)
	j.do(tA, "POST", "/api/me/kyc/", "/api/me/kyc/", map[string]any{"note": "lagi"}, 409)
	j.do(tA, "GET", "/api/me/kyc/", "/api/me/kyc/", nil, 200)
	pend := list(j.do(tAdm, "GET", "/api/admin/kyc", "/api/admin/kyc?status=pending", nil, 200), "requests")
	j.do(tAdm, "POST", "/api/admin/kyc/{id}/reject", "/api/admin/kyc/"+str(obj(pend[0]), "id")+"/reject", map[string]any{"note": "foto buram"}, 200)
	j.do(tA, "GET", "/api/me/kyc/", "/api/me/kyc/", nil, 200)
	j.do(tA, "POST", "/api/me/kyc/", "/api/me/kyc/", map[string]any{"note": "ktp baru"}, 201)
	pend = list(j.do(tAdm, "GET", "/api/admin/kyc", "/api/admin/kyc?status=pending", nil, 200), "requests")
	j.do(tAdm, "POST", "/api/admin/kyc/{id}/approve", "/api/admin/kyc/"+str(obj(pend[0]), "id")+"/approve", map[string]any{"note": ""}, 200)
	j.do(tAdm, "GET", "/api/admin/kyc", "/api/admin/kyc?status=all", nil, 200)

	// ---- grup 1 (manual): Budi pemilik, Cici anggota; Budi bayar -> komisi ke Andi; Cici bayar -> pencairan
	start := time.Now().AddDate(0, 0, 2).Format("2006-01-02")
	newGroup := func(token, name string, slots int, method string) (id, invite string) {
		g := obj(j.do(token, "POST", "/api/groups/", "/api/groups/", map[string]any{
			"name": name, "contribution_amount": 100000, "period_type": "monthly", "slot_count": slots, "start_date": start,
			"late_fee": 5000, "draw_method": method, "min_level_rank": 1}, 201))
		id = str(g, "id")
		d := obj(j.do(token, "GET", "/api/groups/{id}", "/api/groups/"+id, nil, 200))
		return id, str(d, "invite_code")
	}
	join := func(token, invite string) {
		j.do(token, "POST", "/api/groups/join", "/api/groups/join", map[string]any{"invite_code": invite}, 201)
	}
	membersOf := func(token, gid string) []any {
		return list(j.do(token, "GET", "/api/groups/{id}/members", "/api/groups/"+gid+"/members", nil, 200), "members")
	}
	approve := func(token, gid, mid string) {
		j.do(token, "POST", "/api/groups/{id}/members/{memberID}/approve", "/api/groups/"+gid+"/members/"+mid+"/approve", nil, 200)
	}
	pendingIDs := func(token, gid string) []string {
		var ids []string
		for _, m := range membersOf(token, gid) {
			if str(obj(m), "status") == "pending" {
				ids = append(ids, str(obj(m), "id"))
			}
		}
		return ids
	}

	g1, inv1 := newGroup(tB, "Grup Manual", 2, "manual")
	join(tC, inv1)
	for _, mid := range pendingIDs(tB, g1) {
		approve(tB, g1, mid)
	}
	j.do(tB, "GET", "/api/groups/{id}/draw", "/api/groups/"+g1+"/draw", nil, 404)
	var order []string
	for _, m := range membersOf(tB, g1) {
		order = append(order, str(obj(m), "id"))
	}
	j.do(tB, "POST", "/api/groups/{id}/draw/manual", "/api/groups/"+g1+"/draw/manual", map[string]any{"order": order}, 201)
	j.do(tB, "GET", "/api/groups/{id}/draw", "/api/groups/"+g1+"/draw", nil, 200)
	rounds := list(j.do(tB, "GET", "/api/groups/{id}/rounds", "/api/groups/"+g1+"/rounds", nil, 200), "rounds")
	r1 := str(obj(rounds[0]), "id")
	j.do(tB, "GET", "/api/groups/{id}/rounds/{roundID}/contributions", "/api/groups/"+g1+"/rounds/"+r1+"/contributions", nil, 200)
	contribOf := func(token, gid string) string {
		for _, c := range list(j.do(token, "GET", "/api/me/contributions", "/api/me/contributions?status=unpaid", nil, 200), "contributions") {
			if str(obj(c), "group_id") == gid {
				return str(obj(c), "id")
			}
		}
		t.Fatalf("iuran grup %s tidak ditemukan", gid)
		return ""
	}
	cB := contribOf(tB, g1)
	j.do(tB, "GET", "/api/contributions/{id}/quote", "/api/contributions/"+cB+"/quote", nil, 200)
	j.do(tB, "POST", "/api/contributions/{id}/pay", "/api/contributions/"+cB+"/pay", nil, 200) // komisi ke Andi
	j.do(tC, "POST", "/api/contributions/{id}/pay", "/api/contributions/"+contribOf(tC, g1)+"/pay", nil, 200)
	j.do(tB, "GET", "/api/groups/{id}/rounds", "/api/groups/"+g1+"/rounds", nil, 200)
	j.do(tB, "POST", "/api/contributions/{id}/pay", "/api/contributions/"+cB+"/pay", nil, 409) // sudah dibayar

	// ---- grup 2 (acak): Cici pemilik; Budi, Fani gabung; Dewi ditolak; commit + execute; Fani tanpa saldo
	g2, inv2 := newGroup(tC, "Grup Acak", 3, "random")
	join(tB, inv2)
	join(tF, inv2)
	join(tD, inv2)
	var rejectID string
	for _, m := range membersOf(tC, g2) {
		mm := obj(m)
		if str(mm, "user_name") == "Dewi Joiner" {
			rejectID = str(mm, "id")
		}
	}
	for _, mid := range pendingIDs(tC, g2) {
		if mid == rejectID {
			j.do(tC, "POST", "/api/groups/{id}/members/{memberID}/reject", "/api/groups/"+g2+"/members/"+mid+"/reject", nil, 200)
		} else {
			approve(tC, g2, mid)
		}
	}
	j.do(tC, "GET", "/api/groups/{id}/draw", "/api/groups/"+g2+"/draw", nil, 404)
	j.do(tC, "POST", "/api/groups/{id}/draw/commit", "/api/groups/"+g2+"/draw/commit", nil, 201)
	j.do(tF, "GET", "/api/groups/{id}/draw", "/api/groups/"+g2+"/draw", nil, 200)
	j.do(tC, "POST", "/api/groups/{id}/draw/execute", "/api/groups/"+g2+"/draw/execute", nil, 200)
	j.do(tC, "GET", "/api/groups/{id}/draw", "/api/groups/"+g2+"/draw", nil, 200)
	j.do(tC, "GET", "/api/groups/", "/api/groups/", nil, 200)
	cF := contribOf(tF, g2)
	j.do(tF, "GET", "/api/contributions/{id}/quote", "/api/contributions/"+cF+"/quote", nil, 200)
	j.do(tF, "POST", "/api/contributions/{id}/pay", "/api/contributions/"+cF+"/pay", nil, 402)

	// ---- hasil: referral Andi, notifikasi, dompet, ledger
	j.do(tA, "GET", "/api/me/referral/", "/api/me/referral/", nil, 200)
	j.do(tA, "GET", "/api/me/referral/commissions", "/api/me/referral/commissions", nil, 200)
	j.do(tA, "GET", "/api/me/reputation", "/api/me/reputation", nil, 200)
	j.do(tA, "GET", "/api/me/kyc/", "/api/me/kyc/", nil, 200)
	notes := list(j.do(tA, "GET", "/api/me/notifications/", "/api/me/notifications/", nil, 200), "notifications")
	j.do(tA, "GET", "/api/me/notifications/unread-count", "/api/me/notifications/unread-count", nil, 200)
	j.do(tA, "POST", "/api/me/notifications/{id}/read", fmt.Sprintf("/api/me/notifications/%v/read", obj(notes[0])["id"]), nil, 200)
	j.do(tA, "POST", "/api/me/notifications/read-all", "/api/me/notifications/read-all", nil, 200)
	j.do(tB, "GET", "/api/wallet/ledger", "/api/wallet/ledger", nil, 200)
	j.do(tAdm, "GET", "/api/admin/wallet/reconcile", "/api/admin/wallet/reconcile", nil, 200)
	_ = idA

	// ---- tulis / bandingkan
	path := filepath.Join(contractDir, "samples.json")
	doc := map[string]any{"responses": j.shapes, "requests": j.requests}
	out, _ := json.MarshalIndent(doc, "", "  ")
	out = append(out, '\n')
	if updating() {
		if err := os.WriteFile(path, out, 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("samples.json diperbarui (%d respons, %d permintaan)", len(j.shapes), len(j.requests))
		return
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("contract/samples.json belum ada; jalankan: UPDATE_CONTRACT=1 go test ./internal/app/ (%v)", err)
	}
	var want, got map[string]map[string]any
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatal(err)
	}
	_ = json.Unmarshal(out, &got)
	var problems []string
	for _, section := range []string{"responses", "requests"} {
		for k := range want[section] {
			if _, ok := got[section][k]; !ok {
				problems = append(problems, section+" hilang dari alur: "+k)
			}
		}
		for k, v := range got[section] {
			w, ok := want[section][k]
			switch {
			case !ok:
				problems = append(problems, section+" baru: "+k)
			case !reflect.DeepEqual(w, v):
				wb, _ := json.Marshal(w)
				gb, _ := json.Marshal(v)
				problems = append(problems, fmt.Sprintf("%s berubah: %s\n    kontrak: %s\n    sekarang: %s", section, k, wb, gb))
			}
		}
	}
	if len(problems) > 0 {
		sort.Strings(problems)
		t.Fatalf("bentuk API berubah dari kontrak:\n  %s\nPerbarui: UPDATE_CONTRACT=1 go test ./internal/app/ lalu commit contract/samples.json dan periksa frontend",
			strings.Join(problems, "\n  "))
	}
}
