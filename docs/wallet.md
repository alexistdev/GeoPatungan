# Wallet dan ledger

Kode: `backend/internal/wallet/`, migrasi `000009_create_wallets_ledger`.

## Model

Semua nominal dalam **rupiah utuh** (`BIGINT`), tanpa pecahan.

Selain dompet pengguna ada empat **dompet sistem** (satu per jenis, dibuat oleh migrasi):

| Dompet | Isi |
|---|---|
| `escrow` | Iuran yang terkumpul dan belum dicairkan ke penerima giliran |
| `platform` | Biaya platform dan denda keterlambatan |
| `holding` | Dana penarikan yang sudah dipotong dari pengguna, menunggu admin mentransfer |
| `external` | "Dunia luar" (rekening bank). Satu-satunya dompet yang boleh bersaldo negatif |

Dompet pengguna dibuat otomatis saat pertama dibutuhkan.

## Pembukuan berpasangan (double-entry)

Setiap peristiwa uang adalah satu `entry_group` berisi dua baris atau lebih
(`ledger_entries`) bertanda +/- yang **berjumlah tepat 0**. Uang yang masuk atau keluar
sistem dicatat terhadap `external`:

```
Top-up disetujui     : external -X   | user +X
Bayar iuran          : user -(iuran+biaya+denda) | escrow +iuran | platform +(biaya+denda)
Pencairan putaran    : escrow -pot   | penerima +pot
Penarikan (diminta)  : user -X       | holding +X
Penarikan (selesai)  : holding -X    | external +X
Penarikan (ditolak)  : holding -X    | user +X
```

Akibatnya **jumlah seluruh saldo di sistem selalu 0**. Uang yang "hilang" atau "muncul"
langsung terlihat sebagai penyimpangan.

## Satu pintu: `wallet.Post`

`Post(ctx, tx, ref, legs...)` adalah satu-satunya cara mengubah saldo. Ia:

- berjalan di **transaksi pemanggil**, jadi bisa digabung atomik dengan perubahan lain
  (mis. menandai iuran lunas). Jika transaksi dibatalkan, catatan uang ikut batal;
- memvalidasi: minimal dua leg, tanpa nominal 0, tiap dompet sekali, **jumlah tepat 0**,
  dan **tanpa overflow `int64`** (tanpa ini `+MaxInt64 +MaxInt64 +2` berjumlah 0 dan
  menciptakan uang);
- mengunci semua dompet terlibat (`FOR UPDATE`) **dalam urutan id yang sama**, supaya dua
  transfer yang saling berlawanan tidak buntu (deadlock) dan tidak terjadi *lost update*;
- menolak saldo negatif selain `external` (`ErrInsufficientFunds`);
- **idempoten per peristiwa**: pasangan (`ref_type`, `ref_id`, dompet) unik, jadi peristiwa
  yang sama tidak bisa tercatat dua kali (`ErrAlreadyPosted`).

## Pengaman di level database

Kode bisa salah; database tetap menjaga:

| Pengaman | Mekanisme |
|---|---|
| Ledger hanya bisa ditambah | Trigger menolak `UPDATE`, `DELETE`, dan `TRUNCATE` |
| Tiap `entry_group` seimbang | *Constraint trigger* tertunda, dicek saat `COMMIT` |
| Saldo pengguna tidak negatif | `CHECK (balance >= 0 OR type = 'external')` |
| Dompet sistem tunggal | Unique index per jenis |
| Satu dompet per pengguna | `UNIQUE (user_id)` dan `CHECK` jenis ↔ `user_id` |
| Tidak ada pencatatan ganda | Unique index `(ref_type, ref_id, wallet_id)` |

## Rekonsiliasi

`wallet.Reconcile` (admin: `GET /api/admin/wallet/reconcile`) memeriksa empat invarian:

1. saldo tiap dompet = jumlah seluruh mutasinya di ledger;
2. tiap `entry_group` berjumlah 0;
3. jumlah seluruh saldo = 0;
4. `balance_after` tiap baris = saldo berjalan dompet pada baris itu.

Hasil `ok: false` berarti ada uang yang tidak cocok dan harus diselidiki.

## Endpoint

| Endpoint | Siapa | Fungsi |
|---|---|---|
| `GET /api/wallet` | Pengguna | Saldo (dompet dibuat otomatis) |
| `GET /api/wallet/ledger?limit=20&before=<id>` | Pengguna | Riwayat mutasi, terbaru lebih dulu, paginasi kursor |
| `GET /api/admin/wallet/reconcile` | Admin | Laporan rekonsiliasi |

Pengguna hanya melihat mutasi dompetnya sendiri; identitas selalu diambil dari token.

## Catatan untuk tes

Ledger sengaja tidak bisa dihapus, jadi pembersihan data tes (`internal/testutil`)
melewati trigger dengan `session_replication_role = replica` (butuh superuser; pada Docker
dev, pengguna `geopatungan` adalah superuser). Entry group yang menyentuh dompet pengguna
uji dihapus utuh, lalu saldo dompet sistem dihitung ulang, supaya invarian "jumlah saldo = 0"
tetap benar setelah tes.
