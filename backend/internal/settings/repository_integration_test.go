/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */

package settings

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/alexistdev/geopatungan/internal/testutil"
)

// Tes ini butuh database Postgres dengan semua migrasi; dilewati jika TEST_DATABASE_URL kosong.
//
// platform_settings adalah tabel GLOBAL milik aplikasi, jadi tes pengaturan berjalan
// di dalam transaksi yang selalu di-rollback agar tidak menimpa pengaturan sungguhan.

func inRolledBackTx(t *testing.T, e *testutil.Env, fn func(tx pgx.Tx)) {
	t.Helper()
	ctx := context.Background()
	tx, err := e.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	fn(tx)
}

func TestLoadDefaultsAndPartialRows(t *testing.T) {
	e := testutil.NewEnv(t)
	ctx := context.Background()

	inRolledBackTx(t, e, func(tx pgx.Tx) {
		if _, err := tx.Exec(ctx, `DELETE FROM platform_settings`); err != nil {
			t.Fatal(err)
		}
		got, err := Load(ctx, tx)
		if err != nil || got != Defaults() {
			t.Fatalf("tanpa baris: %+v, %v; want Defaults()", got, err)
		}

		// hanya satu kunci diatur: sisanya tetap bawaan
		if _, err := tx.Exec(ctx, `INSERT INTO platform_settings (key, value) VALUES ('platform_fee_bps', '250')`); err != nil {
			t.Fatal(err)
		}
		got, err = Load(ctx, tx)
		want := Defaults()
		want.PlatformFeeBps = 250
		if err != nil || got != want {
			t.Fatalf("sebagian: %+v, %v; want %+v", got, err, want)
		}

		// kunci asing diabaikan
		if _, err := tx.Exec(ctx, `INSERT INTO platform_settings (key, value) VALUES ('kunci_asing', '1')`); err != nil {
			t.Fatal(err)
		}
		if got, err = Load(ctx, tx); err != nil || got != want {
			t.Fatalf("kunci asing: %+v, %v", got, err)
		}
	})
}

func TestLoadRejectsCorruptValue(t *testing.T) {
	e := testutil.NewEnv(t)
	ctx := context.Background()
	inRolledBackTx(t, e, func(tx pgx.Tx) {
		if _, err := tx.Exec(ctx, `DELETE FROM platform_settings`); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO platform_settings (key, value) VALUES ('topup_min_amount', 'bukan-angka')`); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(ctx, tx); err == nil {
			t.Error("nilai rusak seharusnya error, bukan diam-diam memakai bawaan")
		}
	})
}

func TestSaveRoundTripAndAudit(t *testing.T) {
	e := testutil.NewEnv(t)
	ctx := context.Background()
	admin := e.NewAdmin("admin")

	in := Settings{PlatformFeeBps: 175, TopupMinAmount: 25_000, TopupMaxAmount: 75_000_000, WithdrawMinAmount: 120_000, TopupExpiryHours: 72}
	inRolledBackTx(t, e, func(tx pgx.Tx) {
		if err := save(ctx, tx, admin, in); err != nil {
			t.Fatalf("save: %v", err)
		}
		got, err := Load(ctx, tx)
		if err != nil || got != in {
			t.Fatalf("Load = %+v, %v; want %+v", got, err, in)
		}

		// disimpan ulang -> menimpa, bukan menggandakan baris
		in.PlatformFeeBps = 300
		if err := save(ctx, tx, admin, in); err != nil {
			t.Fatal(err)
		}
		if got, _ := Load(ctx, tx); got.PlatformFeeBps != 300 {
			t.Errorf("PlatformFeeBps = %d, want 300", got.PlatformFeeBps)
		}
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM platform_settings WHERE key = 'platform_fee_bps'`).Scan(&n); err != nil || n != 1 {
			t.Errorf("baris platform_fee_bps = %d, want 1", n)
		}

		// pelaku tercatat di baris pengaturan dan di audit log
		var by string
		if err := tx.QueryRow(ctx, `SELECT updated_by::text FROM platform_settings WHERE key = 'topup_expiry_hours'`).Scan(&by); err != nil || by != admin {
			t.Errorf("updated_by = %q, want %q", by, admin)
		}
		if err := tx.QueryRow(ctx,
			`SELECT count(*) FROM audit_logs WHERE actor_id = $1 AND action = 'settings.update'
			    AND (meta->>'platform_fee_bps')::int = 300`, admin).Scan(&n); err != nil || n != 1 {
			t.Errorf("audit settings.update (fee 300) = %d, want 1", n)
		}
	})

	// setelah rollback, pengaturan sungguhan tidak berubah
	var leaked int
	e.Exec(`SELECT 1`)
	leaked = e.QueryInt(`SELECT count(*) FROM audit_logs WHERE actor_id = $1`, admin)
	if leaked != 0 {
		t.Errorf("audit bocor setelah rollback: %d", leaked)
	}
}

func TestBankAccountLifecycle(t *testing.T) {
	e := testutil.NewEnv(t)
	repo := NewRepository(e.Pool)
	ctx := context.Background()
	admin := e.NewAdmin("admin")

	bankName := "Bank-" + e.Prefix
	t.Cleanup(func() {
		_, _ = e.Pool.Exec(context.Background(), `DELETE FROM bank_accounts WHERE bank_name = $1`, bankName)
	})

	a := &BankAccount{BankName: bankName, AccountNumber: "1111111111", AccountHolder: "PT Geo", Notes: "utama", IsActive: true}
	if err := repo.CreateBank(ctx, admin, a); err != nil {
		t.Fatalf("CreateBank: %v", err)
	}
	if a.ID == "" || a.CreatedAt.IsZero() || a.UpdatedAt.IsZero() {
		t.Errorf("ID/timestamp tidak terisi: %+v", a)
	}

	// duplikat (bank + nomor sama)
	dup := &BankAccount{BankName: bankName, AccountNumber: "1111111111", AccountHolder: "Lain", IsActive: true}
	if err := repo.CreateBank(ctx, admin, dup); !errors.Is(err, ErrDuplicate) {
		t.Errorf("duplikat: err = %v, want ErrDuplicate", err)
	}
	// nomor sama di bank lain boleh; bank sama nomor lain boleh
	other := &BankAccount{BankName: bankName, AccountNumber: "2222222222", AccountHolder: "PT Geo", IsActive: false}
	if err := repo.CreateBank(ctx, admin, other); err != nil {
		t.Fatalf("rekening kedua: %v", err)
	}

	// filter aktif vs semua (hanya melihat milik tes ini)
	mine := func(onlyActive bool) map[string]bool {
		list, err := repo.ListBanks(ctx, onlyActive)
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]bool{}
		for _, b := range list {
			if b.BankName == bankName {
				out[b.AccountNumber] = true
			}
		}
		return out
	}
	if got := mine(false); !got["1111111111"] || !got["2222222222"] {
		t.Errorf("admin harus melihat semua: %v", got)
	}
	if got := mine(true); !got["1111111111"] || got["2222222222"] {
		t.Errorf("pengguna hanya rekening aktif: %v", got)
	}

	// ubah + nonaktifkan
	upd := &BankAccount{BankName: bankName, AccountNumber: "1111111111", AccountHolder: "PT Geo Baru", Notes: "ditutup", IsActive: false}
	if err := repo.UpdateBank(ctx, admin, a.ID, upd); err != nil {
		t.Fatalf("UpdateBank: %v", err)
	}
	if upd.ID != a.ID || upd.AccountHolder != "PT Geo Baru" || upd.IsActive || !upd.UpdatedAt.After(a.CreatedAt) && !upd.UpdatedAt.Equal(a.CreatedAt) {
		t.Errorf("hasil update: %+v", upd)
	}
	if got := mine(true); len(got) != 0 {
		t.Errorf("setelah dinonaktifkan tidak boleh tampil bagi pengguna: %v", got)
	}

	// update menabrak rekening lain -> duplikat; tidak ada -> not found
	clash := &BankAccount{BankName: bankName, AccountNumber: "2222222222", AccountHolder: "X Y", IsActive: true}
	if err := repo.UpdateBank(ctx, admin, a.ID, clash); !errors.Is(err, ErrDuplicate) {
		t.Errorf("update bentrok: err = %v, want ErrDuplicate", err)
	}
	if err := repo.UpdateBank(ctx, admin, "00000000-0000-0000-0000-000000000000", upd); !errors.Is(err, ErrNotFound) {
		t.Errorf("update tidak ada: err = %v, want ErrNotFound", err)
	}

	// audit tercatat untuk create & update, lengkap dengan pelaku
	if n := e.QueryInt(`SELECT count(*) FROM audit_logs WHERE entity = 'bank_account' AND entity_id = $1 AND actor_id = $2`, a.ID, admin); n != 2 {
		t.Errorf("audit untuk rekening = %d, want 2 (create + update)", n)
	}
	// update yang gagal tidak meninggalkan audit
	if n := e.QueryInt(`SELECT count(*) FROM audit_logs WHERE entity = 'bank_account' AND actor_id = $1`, admin); n != 3 {
		t.Errorf("total audit = %d, want 3 (2 untuk A, 1 untuk rekening kedua); kegagalan tidak boleh tercatat", n)
	}
}

func TestBankAccountCreateRollsBackWithAudit(t *testing.T) {
	e := testutil.NewEnv(t)
	repo := NewRepository(e.Pool)
	admin := e.NewAdmin("admin")

	bankName := "Bank-" + e.Prefix
	t.Cleanup(func() {
		_, _ = e.Pool.Exec(context.Background(), `DELETE FROM bank_accounts WHERE bank_name = $1`, bankName)
	})
	a := &BankAccount{BankName: bankName, AccountNumber: "3333333333", AccountHolder: "PT Geo", IsActive: true}
	if err := repo.CreateBank(context.Background(), admin, a); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateBank(context.Background(), admin, &BankAccount{BankName: bankName, AccountNumber: "3333333333", AccountHolder: "Lain", IsActive: true}); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("err = %v", err)
	}
	if n := e.QueryInt(`SELECT count(*) FROM audit_logs WHERE actor_id = $1 AND action = 'bank_account.create'`, admin); n != 1 {
		t.Errorf("audit create = %d, want 1 (percobaan duplikat tidak boleh tercatat)", n)
	}
}
