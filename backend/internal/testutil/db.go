/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

// Package testutil berisi bantuan untuk tes integrasi yang memakai database asli.
// Hanya diimpor dari file *_test.go.
package testutil

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/alexistdev/geopatungan/internal/group"
	"github.com/alexistdev/geopatungan/internal/wallet"
)

// Env adalah lingkungan tes: koneksi database dan awalan unik per run, sehingga
// tes hanya menyentuh baris miliknya sendiri dan membersihkannya sesudahnya.
type Env struct {
	T      *testing.T
	Pool   *pgxpool.Pool
	Groups *group.Repository
	Prefix string
	seq    int
}

// NewEnv melewati tes secara otomatis jika TEST_DATABASE_URL tidak diisi:
//
//	TEST_DATABASE_URL="postgres://geopatungan:geopatungan_dev@localhost:5432/geopatungan?sslmode=disable" go test ./...
func NewEnv(t *testing.T) *Env {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL tidak diisi; tes integrasi dilewati")
	}
	ctx := context.Background()
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatalf("TEST_DATABASE_URL tidak valid: %v", err)
	}
	cfg.MaxConns = 16 // cukup untuk tes konkurensi + satu koneksi pemegang kunci
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("gagal konek database: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping database gagal: %v", err)
	}

	lockProcess(t, url)

	e := &Env{T: t, Pool: pool, Groups: group.NewRepository(pool),
		Prefix: fmt.Sprintf("tu-%d", time.Now().UnixNano())}
	t.Cleanup(func() {
		like := e.Prefix + "-%"
		cleanupLedger(ctx, t, pool, like)
		_, _ = pool.Exec(ctx, `DELETE FROM kyc_requests WHERE user_id IN (SELECT id FROM users WHERE email LIKE $1)`, like)
		_, _ = pool.Exec(ctx, `DELETE FROM withdrawals WHERE user_id IN (SELECT id FROM users WHERE email LIKE $1)`, like)
		_, _ = pool.Exec(ctx, `DELETE FROM topups WHERE user_id IN (SELECT id FROM users WHERE email LIKE $1)`, like)
		// audit milik pengguna uji dihapus sebelum penggunanya (FK-nya SET NULL, bukan CASCADE)
		_, _ = pool.Exec(ctx, `DELETE FROM audit_logs WHERE actor_id IN (SELECT id FROM users WHERE email LIKE $1)`, like)
		// grup dihapus dulu (anggota, putaran, iuran ikut via CASCADE), baru user uji.
		_, _ = pool.Exec(ctx, `DELETE FROM groups WHERE owner_id IN (SELECT id FROM users WHERE email LIKE $1)`, like)
		_, _ = pool.Exec(ctx, `DELETE FROM group_members WHERE user_id IN (SELECT id FROM users WHERE email LIKE $1)`, like)
		_, _ = pool.Exec(ctx, `DELETE FROM users WHERE email LIKE $1`, like)

		pool.Close()
	})
	return e
}

// testLockKey adalah kunci advisory yang menyerialkan tes integrasi ("GEOP").
const testLockKey int64 = 0x47454F50

var (
	processLockOnce sync.Once
	processLockErr  error
	processLockConn *pgx.Conn // sengaja tidak pernah ditutup: Postgres melepas kunci saat proses tes berakhir
)

// lockProcess menyerialkan tes integrasi di seluruh paket: kunci advisory Postgres diambil SEKALI
// per proses tes (satu koneksi khusus) dan dipegang sampai proses berakhir.
//
// Dompet sistem (external/escrow/platform) dipakai bersama. Tanpa serialisasi, tes dari paket
// berbeda yang berjalan paralel (perilaku bawaan `go test ./...`) saling mengubah saldo dan
// membuat tes berbasis selisih tidak stabil.
//
// Kunci dipegang per proses, bukan per Env: satu tes boleh memanggil NewEnv berkali-kali
// (mis. dua grup) tanpa menunggu kunci miliknya sendiri. Jika proses mati, kunci lepas otomatis.
func lockProcess(t *testing.T, url string) {
	t.Helper()
	processLockOnce.Do(func() {
		ctx := context.Background()
		conn, err := pgx.Connect(ctx, url)
		if err != nil {
			processLockErr = err
			return
		}
		if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, testLockKey); err != nil {
			_ = conn.Close(ctx)
			processLockErr = err
			return
		}
		processLockConn = conn
	})
	if processLockErr != nil {
		t.Fatalf("gagal mengambil kunci tes integrasi: %v", processLockErr)
	}
}

func (e *Env) NewUser(name string) string {
	e.T.Helper()
	e.seq++
	var id string
	err := e.Pool.QueryRow(context.Background(),
		`INSERT INTO users (name, phone, email, password_hash, referral_code)
		 VALUES ($1, $2, $3, 'x', $4) RETURNING id`,
		name, fmt.Sprintf("%s-%d", e.Prefix, e.seq), fmt.Sprintf("%s-%s-%d@example.com", e.Prefix, name, e.seq),
		fmt.Sprintf("%s-%d", e.Prefix, e.seq)).Scan(&id)
	if err != nil {
		e.T.Fatalf("buat user: %v", err)
	}
	return id
}

// NewAdmin membuat pengguna ber-peran admin.
func (e *Env) NewAdmin(name string) string {
	e.T.Helper()
	id := e.NewUser(name)
	e.Exec(`UPDATE users SET role = 'admin' WHERE id = $1`, id)
	return id
}

// GroupSpec mengatur grup uji. Nilai kosong memakai default.
type GroupSpec struct {
	Slots      int    // default 3
	Active     int    // jumlah anggota aktif termasuk pemilik; default = Slots
	Period     string // weekly | monthly; default monthly
	StartDate  string // YYYY-MM-DD; default 2026-12-01
	Amount     int64  // default 100000
	LateFee    int64  // default 5000
	DrawMethod string // default random
}

func (spec GroupSpec) withDefaults() GroupSpec {
	if spec.Slots == 0 {
		spec.Slots = 3
	}
	if spec.Active == 0 {
		spec.Active = spec.Slots
	}
	if spec.Period == "" {
		spec.Period = "monthly"
	}
	if spec.StartDate == "" {
		spec.StartDate = "2026-12-01"
	}
	if spec.Amount == 0 {
		spec.Amount = 100_000
	}
	if spec.DrawMethod == "" {
		spec.DrawMethod = "random"
	}
	return spec
}

// NewGroup membuat grup beserta anggota aktifnya (pemilik + peserta yang disetujui), semuanya
// pengguna baru. Mengembalikan grup dan anggota aktif berurutan menurut waktu bergabung
// (pemilik pertama).
func (e *Env) NewGroup(spec GroupSpec) (*group.Group, []group.Member) {
	e.T.Helper()
	spec = spec.withDefaults()
	e.seq++
	n := e.seq
	users := []string{e.NewUser(fmt.Sprintf("owner%d", n))}
	for i := 1; i < spec.Active; i++ {
		users = append(users, e.NewUser(fmt.Sprintf("m%d-%d", n, i)))
	}
	return e.NewGroupFor(spec, users...)
}

// NewGroupFor membuat grup dari pengguna yang sudah ada: userIDs[0] pemilik, sisanya peserta
// yang disetujui. Dipakai untuk skenario satu pengguna berada di beberapa grup.
// Spec.Active diabaikan (selalu len(userIDs)); Spec.Slots kosong berarti len(userIDs).
func (e *Env) NewGroupFor(spec GroupSpec, userIDs ...string) (*group.Group, []group.Member) {
	e.T.Helper()
	ctx := context.Background()
	if spec.Slots == 0 {
		spec.Slots = len(userIDs)
	}
	spec.Active = len(userIDs)
	spec = spec.withDefaults()

	e.seq++
	g := &group.Group{
		OwnerID: userIDs[0], Name: "Grup Uji", ContributionAmount: spec.Amount,
		PeriodType: spec.Period, SlotCount: spec.Slots, StartDate: spec.StartDate, LateFee: spec.LateFee,
		DrawMethod: spec.DrawMethod, MinLevelRank: 1, InviteCode: fmt.Sprintf("%s-g%d", e.Prefix, e.seq),
	}
	if err := e.Groups.Create(ctx, g); err != nil {
		e.T.Fatalf("buat grup: %v", err)
	}
	for _, uid := range userIDs[1:] {
		m, err := e.Groups.AddMember(ctx, g.ID, uid)
		if err != nil {
			e.T.Fatal(err)
		}
		if err := e.Groups.ApproveMember(ctx, g.ID, m.ID); err != nil {
			e.T.Fatal(err)
		}
	}
	members, err := e.Groups.ListMembers(ctx, g.ID)
	if err != nil {
		e.T.Fatal(err)
	}
	return g, members
}

// SetTurnOrders menetapkan turn_order 1..n sesuai urutan slice anggota.
func (e *Env) SetTurnOrders(members []group.Member) {
	e.T.Helper()
	for i, m := range members {
		e.Exec(`UPDATE group_members SET turn_order = $2 WHERE id = $1`, m.ID, i+1)
	}
}

func (e *Env) Exec(sql string, args ...any) {
	e.T.Helper()
	if _, err := e.Pool.Exec(context.Background(), sql, args...); err != nil {
		e.T.Fatalf("exec %q: %v", sql, err)
	}
}

func (e *Env) QueryInt(sql string, args ...any) int {
	e.T.Helper()
	var n int
	if err := e.Pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		e.T.Fatalf("query %q: %v", sql, err)
	}
	return n
}

func (e *Env) QueryString(sql string, args ...any) string {
	e.T.Helper()
	var s string
	if err := e.Pool.QueryRow(context.Background(), sql, args...).Scan(&s); err != nil {
		e.T.Fatalf("query %q: %v", sql, err)
	}
	return s
}

// InTx menjalankan fn dalam satu transaksi: commit jika fn mengembalikan nil,
// rollback jika tidak. Error dari fn dikembalikan apa adanya.
func (e *Env) InTx(fn func(tx pgx.Tx) error) error {
	e.T.Helper()
	ctx := context.Background()
	tx, err := e.Pool.Begin(ctx)
	if err != nil {
		e.T.Fatal(err)
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}
	return tx.Commit(ctx)
}

// cleanupLedger menghapus seluruh catatan uang milik pengguna uji. Ledger sengaja tidak
// bisa dihapus (trigger), jadi pembersihan memakai session_replication_role=replica
// (butuh superuser; pada dev Docker pengguna geopatungan adalah superuser).
//
// Seluruh entry_group yang menyentuh dompet pengguna uji dihapus UTUH (termasuk sisi
// dompet sistemnya), lalu saldo dompet sistem dihitung ulang dari ledger. Dompet sistem
// dikunci lebih dulu (urutan id, sama seperti wallet.Post) supaya perhitungan ulang tidak
// membaca keadaan basi saat paket tes lain sedang berjalan paralel.
func cleanupLedger(ctx context.Context, t *testing.T, pool *pgxpool.Pool, like string) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Errorf("cleanup ledger gagal; data tes tertinggal: %v", err)
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `SET LOCAL session_replication_role = replica`); err != nil {
		t.Logf("cleanup ledger dilewati (butuh superuser): %v", err)
		return
	}
	steps := []string{
		`SELECT id FROM wallets WHERE type <> 'user' ORDER BY id FOR UPDATE`,
		// Grup yang menyentuh dompet pengguna uji, ditambah grup yang TIDAK menyentuh dompet pengguna
		// tetapi merupakan akibat peristiwa mereka: penyelesaian penarikan (holding -> external).
		// Tanpa yang kedua, saldo holding hasil hitung ulang bisa negatif dan membatalkan pembersihan.
		`DELETE FROM ledger_entries WHERE entry_group IN (
		     SELECT l.entry_group FROM ledger_entries l JOIN wallets w ON w.id = l.wallet_id
		      WHERE w.user_id IN (SELECT id FROM users WHERE email LIKE $1)
		     UNION
		     SELECT entry_group FROM ledger_entries
		      WHERE ref_id IN (SELECT id FROM withdrawals WHERE user_id IN (SELECT id FROM users WHERE email LIKE $1)))`,
		`UPDATE wallets w SET balance = COALESCE((SELECT SUM(amount) FROM ledger_entries l WHERE l.wallet_id = w.id), 0)
		  WHERE w.type <> 'user'`,
		// Menghapus baris dari tengah riwayat dompet sistem membuat balance_after baris yang tersisa
		// tidak lagi cocok dengan saldo berjalan; hitung ulang agar rantainya tetap benar.
		`UPDATE ledger_entries l SET balance_after = x.running
		   FROM (SELECT id, (SUM(amount) OVER (PARTITION BY wallet_id ORDER BY id))::bigint AS running
		           FROM ledger_entries
		          WHERE wallet_id IN (SELECT id FROM wallets WHERE type <> 'user')) x
		  WHERE l.id = x.id AND l.balance_after <> x.running`,
		`DELETE FROM wallets WHERE user_id IN (SELECT id FROM users WHERE email LIKE $1)`,
	}
	for _, sql := range steps {
		var err error
		if strings.Contains(sql, "$1") {
			_, err = tx.Exec(ctx, sql, like)
		} else {
			_, err = tx.Exec(ctx, sql)
		}
		if err != nil {
			t.Errorf("cleanup ledger gagal; data tes tertinggal: %v", err)
			return
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Errorf("cleanup ledger commit gagal; data tes tertinggal: %v", err)
	}
}

// FundUser menambah saldo dompet pengguna dari dompet "external" (seperti top-up yang disetujui).
func (e *Env) FundUser(userID string, amount int64) {
	e.T.Helper()
	ctx := context.Background()
	err := e.InTx(func(tx pgx.Tx) error {
		uw, err := wallet.EnsureUserWallet(ctx, tx, userID)
		if err != nil {
			return err
		}
		ext, err := wallet.SystemWallet(ctx, tx, wallet.TypeExternal)
		if err != nil {
			return err
		}
		var ref string
		if err := tx.QueryRow(ctx, `SELECT gen_random_uuid()::text`).Scan(&ref); err != nil {
			return err
		}
		_, err = wallet.Post(ctx, tx, wallet.Ref{Type: "test_fund", ID: ref},
			wallet.Leg{WalletID: ext, Amount: -amount}, wallet.Leg{WalletID: uw, Amount: amount})
		return err
	})
	if err != nil {
		e.T.Fatalf("FundUser: %v", err)
	}
}

// Balance mengembalikan saldo dompet pengguna (0 jika belum punya dompet).
func (e *Env) Balance(userID string) int64 {
	e.T.Helper()
	var b int64
	err := e.Pool.QueryRow(context.Background(),
		`SELECT COALESCE((SELECT balance FROM wallets WHERE user_id = $1), 0)`, userID).Scan(&b)
	if err != nil {
		e.T.Fatal(err)
	}
	return b
}
