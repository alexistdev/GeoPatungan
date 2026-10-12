# Jadwal putaran dan iuran

Kode: `backend/internal/round/` dan `backend/internal/contribution/`.

## Kapan jadwal dibuat

Saat urutan giliran ditetapkan, yaitu `POST /api/groups/{id}/draw/execute`
atau `POST /api/groups/{id}/draw/manual`. Pembuatan jadwal berada di
**transaksi yang sama** dengan penetapan urutan: jika jadwal gagal dibuat,
kocokan ikut batal. Tidak ada grup `running` tanpa jadwal.

## Aturan

- **Satu putaran per slot.** Grup 5 slot punya 5 putaran. Penerima putaran
  ke-*k* adalah anggota ber-`turn_order` *k*; tiap anggota menerima tepat sekali.
- **Semua anggota membayar di setiap putaran**, termasuk penerima giliran itu.
  Dana yang diterima = jumlah slot × iuran (`pot_amount`).
- **Jatuh tempo putaran *k*** = `start_date` + (*k* − 1) periode.
  - `weekly`: tambah 7 hari per putaran.
  - `monthly`: tambah bulan dihitung dari `start_date` asli oleh Postgres,
    sehingga akhir bulan aman: mulai 31 Jan → 31 Jan, 28 Feb, 31 Mar, 30 Apr
    (29 Feb pada tahun kabisat).
- **Seluruh tagihan dibuat sekaligus** (anggota × putaran) dengan status `unpaid`,
  sehingga jadwal lengkap terlihat sejak awal.

## Status putaran

```
upcoming ──(putaran sebelumnya ditutup)──▶ collecting ──(semua lunas, dicairkan)──▶ paid_out
```

Putaran 1 langsung `collecting`. Hanya putaran `collecting` yang menerima
pembayaran. `round.Complete` menutup putaran yang seluruh iurannya lunas,
membuka putaran berikutnya, dan menjadikan grup `done` setelah putaran terakhir.

## Tepat waktu dan terlambat

- Tenggat berlaku **sampai akhir hari jatuh tempo menurut WIB** (UTC+7).
  Dibayar pukul 23:59 WIB pada hari jatuh tempo masih tepat waktu;
  pukul 00:00 WIB berikutnya sudah terlambat.
- Terlambat dikenai `late_fee` grup (`contributions.late_fee`) dan
  `paid_on_time = false`. Total yang dibayar = `amount + late_fee`.
- "Terlambat" **tidak** disimpan sebagai status. `overdue` pada API adalah
  turunan: belum dibayar dan jatuh tempo sebelum hari ini (WIB).
- `paid_on_time` menjadi bahan reputasi (syarat "tidak pernah terlambat").

## Endpoint (hanya anggota aktif grup)

| Endpoint | Isi |
|---|---|
| `GET /api/groups/{id}/rounds` | Jadwal, penerima, status, `paid_count`/`member_count`, `pot_amount` |
| `GET /api/groups/{id}/rounds/{roundID}/contributions` | Siapa sudah/belum membayar (transparan) |
| `GET /api/me/contributions?status=unpaid\|paid` | Iuran saya di semua grup, urut jatuh tempo |

Orang luar mendapat 404 (keberadaan grup tidak bocor), anggota pending 403.

## Pembayaran dan penutupan putaran

Dipakai oleh modul pembayaran ([payment.md](payment.md)); keduanya menerima transaksi pemanggil
(`pgx.Tx`) supaya digabung atomik dengan pencatatan ledger, dan sudah diuji terhadap race condition:

- `contribution.MarkPaid(ctx, tx, contributionID, payerUserID, paidAt, feeBps)` melunasi iuran
  milik anggota aktif `payerUserID`, menghitung tepat waktu/terlambat, denda, dan biaya
  platform. Menolak iuran orang lain (404), yang sudah lunas, atau pada putaran yang tidak `collecting`.
- `round.Complete(ctx, tx, roundID)` menutup putaran yang sudah lunas dan memajukan putaran berikutnya.
