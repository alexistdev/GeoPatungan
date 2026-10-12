/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package kyc

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alexistdev/geopatungan/internal/testutil"
)

// Tes ini butuh database Postgres dengan semua migrasi; dilewati jika TEST_DATABASE_URL kosong.

type fx struct {
	e    *testutil.Env
	repo *Repository
	ctx  context.Context
}

func newFx(t *testing.T) *fx {
	t.Helper()
	e := testutil.NewEnv(t)
	return &fx{e: e, repo: NewRepository(e.Pool), ctx: context.Background()}
}

func (f *fx) userKYC(userID string) string {
	f.e.T.Helper()
	return f.e.QueryString(`SELECT kyc_status FROM users WHERE id = $1`, userID)
}

func (f *fx) reqStatus(id string) string {
	f.e.T.Helper()
	return f.e.QueryString(`SELECT status FROM kyc_requests WHERE id = $1`, id)
}

func (f *fx) submit(user string) *Request {
	f.e.T.Helper()
	r, err := f.repo.Submit(f.ctx, user, "mohon diverifikasi")
	if err != nil {
		f.e.T.Fatalf("Submit: %v", err)
	}
	return r
}

func TestSubmit(t *testing.T) {
	f := newFx(t)
	user := f.e.NewUser("a")

	req, err := f.repo.Submit(f.ctx, user, "KTP sudah saya foto")
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if req.ID == "" || req.UserID != user || req.Status != StatusPending || req.Note != "KTP sudah saya foto" ||
		req.ReviewedAt != nil || req.SubmittedAt.IsZero() {
		t.Errorf("hasil = %+v", req)
	}
	if f.userKYC(user) != "pending" {
		t.Errorf("users.kyc_status = %s, want pending", f.userKYC(user))
	}

	if _, err := f.repo.Submit(f.ctx, user, ""); !errors.Is(err, ErrAlreadyPending) {
		t.Errorf("submit ulang: err = %v, want ErrAlreadyPending", err)
	}
	if n := f.e.QueryInt(`SELECT count(*) FROM kyc_requests WHERE user_id = $1`, user); n != 1 {
		t.Errorf("baris permintaan = %d, want 1", n)
	}
}

func TestSubmitBlockedWhenVerifiedButAllowedAfterRejection(t *testing.T) {
	f := newFx(t)
	user, admin := f.e.NewUser("a"), f.e.NewAdmin("admin")

	req := f.submit(user)
	if _, err := f.repo.Review(f.ctx, admin, req.ID, StatusRejected, "foto buram"); err != nil {
		t.Fatal(err)
	}
	// ditolak -> boleh mengajukan lagi
	second := f.submit(user)
	if second.ID == req.ID || f.userKYC(user) != "pending" {
		t.Errorf("pengajuan ulang: %+v, kyc %s", second, f.userKYC(user))
	}
	if _, err := f.repo.Review(f.ctx, admin, second.ID, StatusVerified, "ok"); err != nil {
		t.Fatal(err)
	}
	// terverifikasi -> tidak perlu mengajukan lagi
	if _, err := f.repo.Submit(f.ctx, user, ""); !errors.Is(err, ErrAlreadyVerified) {
		t.Errorf("sudah terverifikasi: err = %v, want ErrAlreadyVerified", err)
	}
	if n := f.e.QueryInt(`SELECT count(*) FROM kyc_requests WHERE user_id = $1`, user); n != 2 {
		t.Errorf("baris permintaan = %d, want 2 (riwayat tetap utuh)", n)
	}
}

// Pengajuan serentak oleh satu pengguna: tepat satu yang lolos.
func TestSubmitConcurrentCreatesOnePendingRequest(t *testing.T) {
	f := newFx(t)
	user := f.e.NewUser("a")

	const n = 10
	results := make([]error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, results[i] = f.repo.Submit(f.ctx, user, "")
		}()
	}
	wg.Wait()

	ok := 0
	for _, err := range results {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, ErrAlreadyPending):
		default:
			t.Errorf("error tak terduga: %v", err)
		}
	}
	if ok != 1 {
		t.Errorf("%d pengajuan berhasil, want tepat 1", ok)
	}
	if n := f.e.QueryInt(`SELECT count(*) FROM kyc_requests WHERE user_id = $1 AND status = 'pending'`, user); n != 1 {
		t.Errorf("permintaan pending = %d, want 1", n)
	}
}

// Transaksi A menandai pengguna terverifikasi tetapi belum commit; Submit yang dimulai saat itu
// harus menunggu, membaca ulang status, lalu gagal ErrAlreadyVerified. Tanpa FOR UPDATE ia membaca
// status lama ("none"), membuat permintaan baru, dan menimpa status akun kembali menjadi "pending".
// (Menunggu saja tidak membuktikan apa-apa: FK ke users juga mengunci baris itu secara implisit.)
func TestSubmitLocksUserRow(t *testing.T) {
	f := newFx(t)
	user := f.e.NewUser("a")

	txA, err := f.e.Pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = txA.Rollback(f.ctx) }()
	if _, err := txA.Exec(f.ctx, `UPDATE users SET kyc_status = 'verified' WHERE id = $1`, user); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() { _, err := f.repo.Submit(f.ctx, user, ""); done <- err }()

	select {
	case err := <-done:
		t.Fatalf("Submit selesai (err=%v) padahal baris pengguna sedang diubah; seharusnya menunggu", err)
	case <-time.After(300 * time.Millisecond):
	}
	if err := txA.Commit(f.ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, ErrAlreadyVerified) {
			t.Errorf("err = %v, want ErrAlreadyVerified (akun sudah diverifikasi A)", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Submit tidak selesai setelah A commit")
	}
	if f.userKYC(user) != "verified" {
		t.Errorf("users.kyc_status = %s, want verified (tidak boleh tertimpa)", f.userKYC(user))
	}
	if n := f.e.QueryInt(`SELECT count(*) FROM kyc_requests WHERE user_id = $1`, user); n != 0 {
		t.Errorf("permintaan = %d, want 0", n)
	}
}

// Walau kunci pengguna tidak ada, indeks unik tetap mencegah permintaan pending ganda dan
// kesalahannya dipetakan ke ErrAlreadyPending (bukan error mentah).
func TestSubmitDuplicatePendingMapsToDomainError(t *testing.T) {
	f := newFx(t)
	user := f.e.NewUser("a")
	f.submit(user)
	// simulasi permintaan yang lolos pengecekan status: akun masih tampak 'none'
	f.e.Exec(`UPDATE users SET kyc_status = 'none' WHERE id = $1`, user)
	if _, err := f.repo.Submit(f.ctx, user, ""); !errors.Is(err, ErrAlreadyPending) {
		t.Errorf("err = %v, want ErrAlreadyPending", err)
	}
}

func TestReviewApprove(t *testing.T) {
	f := newFx(t)
	user, admin := f.e.NewUser("a"), f.e.NewAdmin("admin")
	req := f.submit(user)

	got, err := f.repo.Review(f.ctx, admin, req.ID, StatusVerified, "data sesuai")
	if err != nil {
		t.Fatalf("Review: %v", err)
	}
	if got.Status != StatusVerified || got.ReviewNote != "data sesuai" || got.ReviewedAt == nil {
		t.Errorf("hasil = %+v", got)
	}
	if f.userKYC(user) != "verified" {
		t.Errorf("users.kyc_status = %s, want verified", f.userKYC(user))
	}
	if n := f.e.QueryInt(`SELECT count(*) FROM kyc_requests WHERE id = $1 AND reviewed_by = $2`, req.ID, admin); n != 1 {
		t.Error("reviewed_by harus mencatat admin")
	}
	if n := f.e.QueryInt(`SELECT count(*) FROM audit_logs WHERE action = 'kyc.verified' AND entity_id = $1 AND actor_id = $2`, req.ID, admin); n != 1 {
		t.Errorf("audit = %d, want 1", n)
	}
}

func TestReviewReject(t *testing.T) {
	f := newFx(t)
	user, admin := f.e.NewUser("a"), f.e.NewAdmin("admin")
	req := f.submit(user)

	got, err := f.repo.Review(f.ctx, admin, req.ID, StatusRejected, "foto tidak jelas")
	if err != nil || got.Status != StatusRejected || got.ReviewNote != "foto tidak jelas" {
		t.Fatalf("Review: %+v, %v", got, err)
	}
	if f.userKYC(user) != "rejected" {
		t.Errorf("users.kyc_status = %s, want rejected", f.userKYC(user))
	}
	if n := f.e.QueryInt(`SELECT count(*) FROM audit_logs WHERE action = 'kyc.rejected' AND entity_id = $1`, req.ID); n != 1 {
		t.Errorf("audit = %d, want 1", n)
	}
}

func TestReviewRejections(t *testing.T) {
	f := newFx(t)
	user, admin := f.e.NewUser("a"), f.e.NewAdmin("admin")

	t.Run("admin memverifikasi dirinya sendiri", func(t *testing.T) {
		self := f.e.NewAdmin("self")
		req := f.submit(self)
		if _, err := f.repo.Review(f.ctx, self, req.ID, StatusVerified, ""); !errors.Is(err, ErrSelfReview) {
			t.Errorf("err = %v, want ErrSelfReview", err)
		}
		if f.reqStatus(req.ID) != StatusPending || f.userKYC(self) != "pending" {
			t.Errorf("status = %s kyc = %s; tidak boleh berubah", f.reqStatus(req.ID), f.userKYC(self))
		}
		// admin lain boleh
		if _, err := f.repo.Review(f.ctx, admin, req.ID, StatusVerified, ""); err != nil {
			t.Errorf("admin lain: %v", err)
		}
	})
	t.Run("id tidak ada", func(t *testing.T) {
		if _, err := f.repo.Review(f.ctx, admin, "00000000-0000-0000-0000-000000000000", StatusVerified, ""); !errors.Is(err, ErrNotFound) {
			t.Errorf("err = %v, want ErrNotFound", err)
		}
	})
	t.Run("sudah diputuskan", func(t *testing.T) {
		req := f.submit(user)
		if _, err := f.repo.Review(f.ctx, admin, req.ID, StatusRejected, "alasan"); err != nil {
			t.Fatal(err)
		}
		if _, err := f.repo.Review(f.ctx, admin, req.ID, StatusVerified, ""); !errors.Is(err, ErrInvalidState) {
			t.Errorf("err = %v, want ErrInvalidState (penolakan tidak boleh dibalik diam-diam)", err)
		}
		if f.userKYC(user) != "rejected" {
			t.Errorf("kyc = %s, want rejected", f.userKYC(user))
		}
	})
}

// Dua admin memutuskan permintaan yang sama bersamaan: tepat satu yang menang.
func TestReviewConcurrentExactlyOneWins(t *testing.T) {
	f := newFx(t)
	user := f.e.NewUser("a")
	admins := []string{f.e.NewAdmin("a1"), f.e.NewAdmin("a2"), f.e.NewAdmin("a3")}
	req := f.submit(user)

	const n = 9
	results := make([]error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			decision := StatusVerified
			if i%2 == 1 {
				decision = StatusRejected
			}
			_, results[i] = f.repo.Review(f.ctx, admins[i%len(admins)], req.ID, decision, "x")
		}()
	}
	wg.Wait()

	ok := 0
	for _, err := range results {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, ErrInvalidState):
		default:
			t.Errorf("error tak terduga: %v", err)
		}
	}
	if ok != 1 {
		t.Fatalf("%d keputusan berhasil, want tepat 1", ok)
	}
	// status akun harus konsisten dengan keputusan yang menang
	if f.userKYC(user) != f.reqStatus(req.ID) {
		t.Errorf("users.kyc_status = %s tetapi permintaan = %s", f.userKYC(user), f.reqStatus(req.ID))
	}
}

// Admin A menolak (belum commit); persetujuan oleh admin B yang dimulai saat itu harus menunggu lalu
// gagal. Tanpa FOR UPDATE, B membaca status lama (pending), menandai akun terverifikasi, dan
// menimpa penolakan A: audit mencatat "ditolak" tetapi pengguna tetap tampil terverifikasi.
func TestReviewLocksRequest(t *testing.T) {
	f := newFx(t)
	user := f.e.NewUser("a")
	adminA, adminB := f.e.NewAdmin("adminA"), f.e.NewAdmin("adminB")
	req := f.submit(user)

	txA, err := f.e.Pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = txA.Rollback(f.ctx) }()
	if _, err := txA.Exec(f.ctx, `SELECT 1 FROM kyc_requests WHERE id = $1 FOR UPDATE`, req.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := txA.Exec(f.ctx,
		`UPDATE kyc_requests SET status = 'rejected', reviewed_by = $2, reviewed_at = now(), review_note = 'ditolak A' WHERE id = $1`,
		req.ID, adminA); err != nil {
		t.Fatal(err)
	}
	if _, err := txA.Exec(f.ctx, `UPDATE users SET kyc_status = 'rejected' WHERE id = $1`, user); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() { _, err := f.repo.Review(f.ctx, adminB, req.ID, StatusVerified, ""); done <- err }()

	select {
	case err := <-done:
		t.Fatalf("Review selesai (err=%v) padahal A belum commit; seharusnya menunggu", err)
	case <-time.After(300 * time.Millisecond):
	}
	if err := txA.Commit(f.ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, ErrInvalidState) {
			t.Errorf("err = %v, want ErrInvalidState", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Review tidak selesai setelah A commit")
	}
	if f.reqStatus(req.ID) != StatusRejected || f.userKYC(user) != "rejected" {
		t.Errorf("permintaan = %s akun = %s, want rejected keduanya", f.reqStatus(req.ID), f.userKYC(user))
	}
}

func TestStatusOf(t *testing.T) {
	f := newFx(t)
	user, admin := f.e.NewUser("a"), f.e.NewAdmin("admin")

	st, err := f.repo.StatusOf(f.ctx, user)
	if err != nil || st.KYCStatus != "none" || st.Latest != nil {
		t.Fatalf("pengguna baru: %+v, %v", st, err)
	}

	first := f.submit(user)
	if _, err := f.repo.Review(f.ctx, admin, first.ID, StatusRejected, "buram"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	second := f.submit(user)

	st, _ = f.repo.StatusOf(f.ctx, user)
	if st.KYCStatus != "pending" || st.Latest == nil || st.Latest.ID != second.ID {
		t.Errorf("harus menampilkan permintaan TERBARU: %+v", st)
	}
	if _, err := f.repo.StatusOf(f.ctx, "00000000-0000-0000-0000-000000000000"); err == nil {
		t.Error("pengguna tidak ada seharusnya error")
	}
}

func TestListAdmin(t *testing.T) {
	f := newFx(t)
	admin := f.e.NewAdmin("admin")
	u1, u2, u3 := f.e.NewUser("u1"), f.e.NewUser("u2"), f.e.NewUser("u3")

	r1 := f.submit(u1)
	time.Sleep(5 * time.Millisecond)
	r2 := f.submit(u2)
	time.Sleep(5 * time.Millisecond)
	r3 := f.submit(u3)
	if _, err := f.repo.Review(f.ctx, admin, r2.ID, StatusVerified, ""); err != nil {
		t.Fatal(err)
	}

	mine := func(status string) []AdminRequest {
		t.Helper()
		all, err := f.repo.ListAdmin(f.ctx, status, 200)
		if err != nil {
			t.Fatal(err)
		}
		var out []AdminRequest
		for _, x := range all {
			if strings.Contains(x.UserEmail, f.e.Prefix) {
				out = append(out, x)
			}
		}
		return out
	}
	ids := func(l []AdminRequest) string {
		var s []string
		for _, x := range l {
			s = append(s, x.ID)
		}
		return strings.Join(s, ",")
	}

	if got := mine(StatusPending); ids(got) != r1.ID+","+r3.ID {
		t.Errorf("pending = %v, want terlama dulu", ids(got))
	}
	if got := mine(StatusVerified); ids(got) != r2.ID {
		t.Errorf("verified = %v", ids(got))
	}
	if got := mine("all"); len(got) != 3 || got[0].ID != r3.ID {
		t.Errorf("all = %v, want 3 baris terbaru dulu", ids(got))
	}
	if got := mine(StatusRejected); len(got) != 0 {
		t.Errorf("rejected = %v", ids(got))
	}
	v := mine(StatusVerified)[0]
	if v.UserName == "" || !strings.Contains(v.UserEmail, f.e.Prefix) || v.ReviewedBy == nil || *v.ReviewedBy != admin {
		t.Errorf("identitas: %+v", v)
	}
}

func TestDatabaseConstraints(t *testing.T) {
	f := newFx(t)
	user := f.e.NewUser("a")
	exec := func(sql string, args ...any) error {
		_, err := f.e.Pool.Exec(f.ctx, sql, args...)
		return err
	}

	f.submit(user)
	if err := exec(`INSERT INTO kyc_requests (user_id) VALUES ($1)`, user); err == nil {
		t.Error("dua permintaan pending untuk satu pengguna harus ditolak indeks unik")
	}
	if err := exec(`UPDATE kyc_requests SET status = 'verified' WHERE user_id = $1`, user); err == nil {
		t.Error("verified tanpa reviewed_at harus ditolak")
	}
	if err := exec(`UPDATE kyc_requests SET reviewed_at = now() WHERE user_id = $1`, user); err == nil {
		t.Error("pending dengan reviewed_at harus ditolak")
	}
	if err := exec(`UPDATE kyc_requests SET status = 'disetujui', reviewed_at = now() WHERE user_id = $1`, user); err == nil {
		t.Error("status asing harus ditolak")
	}
	// banyak permintaan non-pending untuk satu pengguna diperbolehkan (riwayat)
	for i := 0; i < 3; i++ {
		if err := exec(`INSERT INTO kyc_requests (user_id, status, reviewed_at) VALUES ($1, 'rejected', now())`, user); err != nil {
			t.Errorf("riwayat penolakan ke-%d: %v", i+1, err)
		}
	}
}
