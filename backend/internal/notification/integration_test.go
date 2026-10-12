/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package notification_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/alexistdev/geopatungan/internal/draw"
	"github.com/alexistdev/geopatungan/internal/kyc"
	"github.com/alexistdev/geopatungan/internal/notification"
	"github.com/alexistdev/geopatungan/internal/testutil"
	"github.com/alexistdev/geopatungan/internal/topup"
	"github.com/alexistdev/geopatungan/internal/withdrawal"
)

// Tes ini butuh database Postgres dengan semua migrasi; dilewati jika TEST_DATABASE_URL kosong.
// Memakai paket eksternal karena menguji juga modul yang mengimpor notification (hindari siklus).

var ctx = context.Background()

func count(e *testutil.Env, user, typ string) int {
	e.T.Helper()
	return e.QueryInt(`SELECT count(*) FROM notifications WHERE user_id = $1 AND type = $2`, user, typ)
}

func latest(e *testutil.Env, user, typ string) (title, body string) {
	e.T.Helper()
	if err := e.Pool.QueryRow(ctx, `SELECT title, body FROM notifications WHERE user_id = $1 AND type = $2 ORDER BY id DESC LIMIT 1`,
		user, typ).Scan(&title, &body); err != nil {
		e.T.Fatalf("notifikasi %s untuk %s: %v", typ, user, err)
	}
	return
}

func send(e *testutil.Env, user, typ, title string) {
	e.T.Helper()
	if err := notification.Send(ctx, e.Pool, notification.Notice{UserID: user, Type: typ, Title: title, Body: "isi"}); err != nil {
		e.T.Fatal(err)
	}
}

// ---- Send / SendToGroupMembers ---------------------------------------------

func TestSendStoresAndRollsBackWithTransaction(t *testing.T) {
	e := testutil.NewEnv(t)
	u := e.NewUser("u")

	if err := notification.Send(ctx, e.Pool, notification.Notice{UserID: u, Type: "t", Title: "Halo", Body: "Isi", Data: map[string]any{"k": "v"}}); err != nil {
		t.Fatal(err)
	}
	if n := count(e, u, "t"); n != 1 {
		t.Fatalf("tersimpan %d", n)
	}
	if v := e.QueryString(`SELECT data->>'k' FROM notifications WHERE user_id = $1`, u); v != "v" {
		t.Errorf("data = %q", v)
	}
	// data nil -> objek kosong, bukan NULL
	send(e, u, "t2", "Tanpa data")
	if v := e.QueryString(`SELECT data::text FROM notifications WHERE user_id = $1 AND type = 't2'`, u); v != "{}" {
		t.Errorf("data = %q, want {}", v)
	}

	// transaksi dibatalkan -> pemberitahuan ikut batal
	err := e.InTx(func(tx pgx.Tx) error {
		if err := notification.Send(ctx, tx, notification.Notice{UserID: u, Type: "rb", Title: "x", Body: "y"}); err != nil {
			return err
		}
		return errors.New("batal")
	})
	if err == nil || count(e, u, "rb") != 0 {
		t.Errorf("pemberitahuan harus ikut batal (err=%v, n=%d)", err, count(e, u, "rb"))
	}

	// tidak valid: ditolak sebelum menyentuh database
	if err := notification.Send(ctx, e.Pool, notification.Notice{UserID: u, Type: "bad", Title: "", Body: "y"}); !errors.Is(err, notification.ErrInvalid) {
		t.Errorf("err = %v, want ErrInvalid", err)
	}
	// pengguna tidak ada -> FK menolak
	if err := notification.Send(ctx, e.Pool, notification.Notice{UserID: "00000000-0000-0000-0000-000000000000", Type: "t", Title: "x", Body: "y"}); err == nil {
		t.Error("penerima tidak ada harus ditolak")
	}
	// CHECK di database menjaga walau validasi dilewati
	if _, err := e.Pool.Exec(ctx, `INSERT INTO notifications (user_id, type, title, body) VALUES ($1, 't', '   ', 'y')`, u); err == nil {
		t.Error("judul kosong harus ditolak CHECK")
	}
}

func TestSendToGroupMembersOnlyActive(t *testing.T) {
	e := testutil.NewEnv(t)
	g, members := e.NewGroup(testutil.GroupSpec{Slots: 4, Active: 3})
	pendingUser := e.NewUser("pending")
	if _, err := e.Groups.AddMember(ctx, g.ID, pendingUser); err != nil { // status pending
		t.Fatal(err)
	}
	outsider := e.NewUser("outsider")

	if err := notification.SendToGroupMembers(ctx, e.Pool, g.ID, notification.Notice{Type: "gm", Title: "Halo", Body: "Isi"}); err != nil {
		t.Fatal(err)
	}
	for _, m := range members {
		if count(e, m.UserID, "gm") != 1 {
			t.Errorf("anggota aktif %s harus menerima tepat 1", m.UserID)
		}
	}
	if count(e, pendingUser, "gm") != 0 || count(e, outsider, "gm") != 0 {
		t.Error("anggota pending dan orang luar tidak boleh menerima")
	}
	// grup kosong/tidak ada: tidak error, tidak ada apa-apa
	if err := notification.SendToGroupMembers(ctx, e.Pool, "00000000-0000-0000-0000-000000000000", notification.Notice{Type: "gm", Title: "a", Body: "b"}); err != nil {
		t.Errorf("grup tak ada: %v", err)
	}
	if err := notification.SendToGroupMembers(ctx, e.Pool, g.ID, notification.Notice{Type: "gm", Title: "", Body: "b"}); !errors.Is(err, notification.ErrInvalid) {
		t.Errorf("err = %v, want ErrInvalid", err)
	}
}

// ---- repository ------------------------------------------------------------

func TestRepositoryListReadAndOwnership(t *testing.T) {
	e := testutil.NewEnv(t)
	repo := notification.NewRepository(e.Pool)
	a, b := e.NewUser("a"), e.NewUser("b")
	for i := 0; i < 5; i++ {
		send(e, a, "n", "A"+string(rune('1'+i)))
	}
	send(e, b, "n", "milik B")

	items, err := repo.List(ctx, a, 3, 0, false)
	if err != nil || len(items) != 4 { // limit+1
		t.Fatalf("items = %d, %v", len(items), err)
	}
	if items[0].Title != "A5" || items[0].ID <= items[1].ID {
		t.Errorf("harus terbaru dulu: %+v", items[0])
	}
	for _, it := range items {
		if it.Title == "milik B" {
			t.Error("pemberitahuan orang lain bocor")
		}
	}
	older, err := repo.List(ctx, a, 10, items[2].ID, false)
	if err != nil || len(older) != 2 || older[0].Title != "A2" {
		t.Errorf("halaman berikutnya = %+v, %v", older, err)
	}

	if n, _ := repo.UnreadCount(ctx, a); n != 5 {
		t.Errorf("unread = %d", n)
	}
	// B tidak bisa menandai milik A; milik A tetap belum dibaca
	if err := repo.MarkRead(ctx, b, items[0].ID); !errors.Is(err, notification.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
	if n, _ := repo.UnreadCount(ctx, a); n != 5 {
		t.Errorf("unread A berubah oleh B: %d", n)
	}
	if err := repo.MarkRead(ctx, a, 9_000_000_000); !errors.Is(err, notification.ErrNotFound) {
		t.Errorf("tidak ada: %v", err)
	}

	// idempoten: waktu baca pertama dipertahankan
	if err := repo.MarkRead(ctx, a, items[0].ID); err != nil {
		t.Fatal(err)
	}
	first := e.QueryString(`SELECT read_at::text FROM notifications WHERE id = $1`, items[0].ID)
	if err := repo.MarkRead(ctx, a, items[0].ID); err != nil {
		t.Fatalf("baca ulang: %v", err)
	}
	if again := e.QueryString(`SELECT read_at::text FROM notifications WHERE id = $1`, items[0].ID); again != first {
		t.Errorf("read_at berubah: %s -> %s", first, again)
	}

	unread, _ := repo.List(ctx, a, 10, 0, true)
	if len(unread) != 4 {
		t.Errorf("hanya belum dibaca = %d, want 4", len(unread))
	}
	all, _ := repo.List(ctx, a, 10, 0, false)
	reads := 0
	for _, it := range all {
		if it.Read {
			reads++
		}
	}
	if reads != 1 {
		t.Errorf("dibaca = %d, want 1", reads)
	}

	// read-all hanya milik sendiri, mengembalikan jumlah yang berubah
	n, err := repo.MarkAllRead(ctx, a)
	if err != nil || n != 4 {
		t.Errorf("read-all = %d, %v, want 4", n, err)
	}
	if n, _ := repo.MarkAllRead(ctx, a); n != 0 {
		t.Errorf("read-all kedua = %d, want 0", n)
	}
	if n, _ := repo.UnreadCount(ctx, b); n != 1 {
		t.Errorf("milik B ikut terbaca: %d", n)
	}
}

// ---- kejadian -> pemberitahuan ---------------------------------------------

func TestTopupNotifications(t *testing.T) {
	e := testutil.NewEnv(t)
	repo := topup.NewRepository(e.Pool)
	admin, u := e.NewAdmin("adm"), e.NewUser("u")

	tp, err := repo.Create(ctx, u, 100_000, []int{347}, 24, 3)
	if err != nil {
		t.Fatal(err)
	}
	if count(e, u, notification.TypeTopupApproved)+count(e, u, notification.TypeTopupRejected) != 0 {
		t.Error("membuat permintaan belum boleh memberi tahu hasil")
	}
	if _, err := repo.Approve(ctx, admin, tp.ID, nil, ""); err != nil {
		t.Fatal(err)
	}
	title, body := latest(e, u, notification.TypeTopupApproved)
	if title != "Top-up disetujui" || !strings.Contains(body, "Rp100.347") {
		t.Errorf("approved: %q / %q", title, body)
	}

	tp2, _ := repo.Create(ctx, u, 50_000, []int{201}, 24, 3)
	if _, err := repo.Reject(ctx, admin, tp2.ID, "bukti tidak jelas"); err != nil {
		t.Fatal(err)
	}
	_, body = latest(e, u, notification.TypeTopupRejected)
	if !strings.Contains(body, "bukti tidak jelas") || !strings.Contains(body, "Rp50.201") {
		t.Errorf("rejected body = %q", body)
	}
	// admin tidak menerima apa pun
	if n := e.QueryInt(`SELECT count(*) FROM notifications WHERE user_id = $1`, admin); n != 0 {
		t.Errorf("admin menerima %d pemberitahuan", n)
	}
	// persetujuan ulang (gagal) tidak menambah pemberitahuan
	if _, err := repo.Approve(ctx, admin, tp.ID, nil, ""); err == nil {
		t.Fatal("persetujuan ulang harus gagal")
	}
	if n := count(e, u, notification.TypeTopupApproved); n != 1 {
		t.Errorf("pemberitahuan approved = %d, want 1", n)
	}
}

// Jika pemberitahuan gagal ditulis, kejadiannya ikut batal (satu transaksi).
func TestEventRollsBackWhenNotificationFails(t *testing.T) {
	e := testutil.NewEnv(t)
	admin, u := e.NewAdmin("adm"), e.NewUser("u")
	repo := topup.NewRepository(e.Pool)
	tp, _ := repo.Create(ctx, u, 100_000, []int{347}, 24, 3)
	g, _ := e.NewGroup(testutil.GroupSpec{Slots: 3, Active: 2})
	joiner := e.NewUser("joiner")
	m, err := e.Groups.AddMember(ctx, g.ID, joiner)
	if err != nil {
		t.Fatal(err)
	}

	e.Exec(`CREATE OR REPLACE FUNCTION zz_block_notif() RETURNS trigger LANGUAGE plpgsql AS $$
	        BEGIN RAISE EXCEPTION 'notifikasi diblokir untuk uji'; END $$`)
	e.Exec(`CREATE TRIGGER zz_block_notif BEFORE INSERT ON notifications
	        FOR EACH ROW WHEN (NEW.type IN ('topup.approved', 'group.member_approved')) EXECUTE FUNCTION zz_block_notif()`)
	t.Cleanup(func() {
		_, _ = e.Pool.Exec(ctx, `DROP TRIGGER IF EXISTS zz_block_notif ON notifications`)
		_, _ = e.Pool.Exec(ctx, `DROP FUNCTION IF EXISTS zz_block_notif()`)
	})

	if _, err := repo.Approve(ctx, admin, tp.ID, nil, ""); err == nil {
		t.Fatal("Approve harus gagal")
	}
	if st := e.QueryString(`SELECT status FROM topups WHERE id = $1`, tp.ID); st == "approved" {
		t.Error("top-up tidak boleh berstatus approved")
	}
	if e.Balance(u) != 0 {
		t.Errorf("saldo = %d; kredit harus ikut batal", e.Balance(u))
	}

	if err := e.Groups.ApproveMember(ctx, g.ID, m.ID); err == nil {
		t.Fatal("ApproveMember harus gagal")
	}
	if st := e.QueryString(`SELECT status FROM group_members WHERE id = $1`, m.ID); st != "pending" {
		t.Errorf("status anggota = %s, want pending", st)
	}
}

func TestWithdrawalNotifications(t *testing.T) {
	e := testutil.NewEnv(t)
	repo := withdrawal.NewRepository(e.Pool)
	admin, u := e.NewAdmin("adm"), e.NewUser("u")
	e.FundUser(u, 1_000_000)
	in := func(a int64) withdrawal.Input {
		return withdrawal.Input{Amount: a, BankName: "BCA", AccountNumber: "1234567890", AccountHolder: "Budi"}
	}

	w1, err := repo.Create(ctx, u, in(300_000), 3)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Complete(ctx, admin, w1.ID, "REF-998877", ""); err != nil {
		t.Fatal(err)
	}
	title, body := latest(e, u, notification.TypeWithdrawalDone)
	if title != "Penarikan selesai" || !strings.Contains(body, "Rp300.000") || !strings.Contains(body, "REF-998877") {
		t.Errorf("done: %q / %q", title, body)
	}

	w2, _ := repo.Create(ctx, u, in(200_000), 3)
	if _, err := repo.Reject(ctx, admin, w2.ID, "rekening tidak cocok"); err != nil {
		t.Fatal(err)
	}
	_, body = latest(e, u, notification.TypeWithdrawalRejected)
	if !strings.Contains(body, "rekening tidak cocok") || !strings.Contains(body, "Rp200.000") {
		t.Errorf("rejected: %q", body)
	}
	// pembatalan oleh pengguna sendiri tidak memberi notifikasi (ia sendiri yang melakukannya)
	w3, _ := repo.Create(ctx, u, in(100_000), 3)
	if _, err := repo.Cancel(ctx, u, w3.ID); err != nil {
		t.Fatal(err)
	}
	if n := e.QueryInt(`SELECT count(*) FROM notifications WHERE user_id = $1 AND type LIKE 'withdrawal.%'`, u); n != 2 {
		t.Errorf("pemberitahuan penarikan = %d, want 2", n)
	}
}

func TestKYCNotifications(t *testing.T) {
	e := testutil.NewEnv(t)
	repo := kyc.NewRepository(e.Pool)
	admin, a, b := e.NewAdmin("adm"), e.NewUser("a"), e.NewUser("b")

	ra, err := repo.Submit(ctx, a, "ktp saya")
	if err != nil {
		t.Fatal(err)
	}
	rb, _ := repo.Submit(ctx, b, "ktp saya")
	if _, err := repo.Review(ctx, admin, ra.ID, kyc.StatusVerified, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Review(ctx, admin, rb.ID, kyc.StatusRejected, "foto buram"); err != nil {
		t.Fatal(err)
	}
	if title, _ := latest(e, a, notification.TypeKYCVerified); title != "Verifikasi identitas disetujui" {
		t.Errorf("title = %q", title)
	}
	if _, body := latest(e, b, notification.TypeKYCRejected); !strings.Contains(body, "foto buram") {
		t.Errorf("body = %q", body)
	}
	if count(e, a, notification.TypeKYCRejected) != 0 || count(e, b, notification.TypeKYCVerified) != 0 {
		t.Error("jenis pemberitahuan tertukar")
	}
	// keputusan ulang (gagal) tidak menggandakan
	if _, err := repo.Review(ctx, admin, ra.ID, kyc.StatusVerified, ""); err == nil {
		t.Fatal("keputusan ulang harus gagal")
	}
	if count(e, a, notification.TypeKYCVerified) != 1 {
		t.Error("pemberitahuan ganda")
	}
}

func TestMemberApprovalNotifications(t *testing.T) {
	e := testutil.NewEnv(t)
	g, _ := e.NewGroup(testutil.GroupSpec{Slots: 4, Active: 2})
	ok, no := e.NewUser("ok"), e.NewUser("no")
	mOK, _ := e.Groups.AddMember(ctx, g.ID, ok)
	mNo, _ := e.Groups.AddMember(ctx, g.ID, no)

	if err := e.Groups.ApproveMember(ctx, g.ID, mOK.ID); err != nil {
		t.Fatal(err)
	}
	if err := e.Groups.RejectMember(ctx, g.ID, mNo.ID); err != nil {
		t.Fatal(err)
	}
	if _, body := latest(e, ok, notification.TypeMemberApproved); !strings.Contains(body, "Grup Uji") {
		t.Errorf("approved body = %q", body)
	}
	if _, body := latest(e, no, notification.TypeMemberRejected); !strings.Contains(body, "Grup Uji") {
		t.Errorf("rejected body = %q", body)
	}
	// tindakan ulang pada anggota yang sudah diputuskan: ErrNotFound, tanpa pemberitahuan baru
	if err := e.Groups.RejectMember(ctx, g.ID, mOK.ID); err == nil {
		t.Fatal("menolak anggota aktif harus gagal")
	}
	if err := e.Groups.RejectMember(ctx, g.ID, mNo.ID); err == nil {
		t.Fatal("menolak ulang harus gagal")
	}
	if count(e, ok, notification.TypeMemberRejected) != 0 || count(e, no, notification.TypeMemberRejected) != 1 {
		t.Error("pemberitahuan penolakan salah")
	}
	// grup yang salah tidak mengubah apa pun
	other, _ := e.NewGroup(testutil.GroupSpec{Slots: 3, Active: 2})
	m3, _ := e.Groups.AddMember(ctx, other.ID, e.NewUser("z"))
	if err := e.Groups.RejectMember(ctx, g.ID, m3.ID); err == nil {
		t.Error("anggota grup lain tidak boleh ditolak lewat grup ini")
	}
}

func TestGroupStartedNotifiesActiveMembers(t *testing.T) {
	e := testutil.NewEnv(t)
	g, members := e.NewGroup(testutil.GroupSpec{Slots: 3})
	late := e.NewUser("late") // pending, bukan anggota aktif
	_, _ = e.Groups.AddMember(ctx, g.ID, late)

	entries := make([]draw.Entry, len(members))
	for i, m := range members {
		pos := len(members) - i
		entries[i] = draw.Entry{MemberID: m.ID, UserName: m.UserName, Weight: 1, Order: &pos}
	}
	if err := draw.NewRepository(e.Pool).Manual(ctx, &draw.Draw{GroupID: g.ID, Algorithm: "manual", Entries: entries}); err != nil {
		t.Fatalf("Manual: %v", err)
	}
	for _, m := range members {
		if count(e, m.UserID, notification.TypeGroupStarted) != 1 {
			t.Errorf("anggota %s harus menerima tepat 1", m.UserID)
		}
	}
	if count(e, late, notification.TypeGroupStarted) != 0 {
		t.Error("anggota pending tidak boleh menerima")
	}
	// kocokan kedua ditolak dan tidak menambah pemberitahuan
	if err := draw.NewRepository(e.Pool).Manual(ctx, &draw.Draw{GroupID: g.ID, Algorithm: "manual", Entries: entries}); err == nil {
		t.Fatal("kocokan ulang harus gagal")
	}
	if count(e, members[0].UserID, notification.TypeGroupStarted) != 1 {
		t.Error("pemberitahuan ganda")
	}
}
