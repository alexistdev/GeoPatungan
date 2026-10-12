# Pembayaran iuran dan pencairan

Kode: `backend/internal/payment/` (orkestrasi), `contribution.MarkPaid`, `round.Complete`,
`wallet.Post` dan `wallet.LockWallets`.

## Alur

```
POST /api/contributions/{id}/pay        (pembayar = pengguna yang login, tanpa body)
  1. verifikasi kepemilikan, lalu kunci baris putaran
  2. kunci SEMUA dompet yang akan disentuh (pembayar, penerima giliran, escrow, platform)
  3. tandai iuran lunas (hitung biaya platform dan denda)
  4. catat uang:  pembayar -total | escrow +iuran | platform +(biaya + denda)
  5. jika ini iuran TERAKHIR putaran itu:
       tutup putaran, majukan putaran berikutnya (atau grup menjadi done),
       cairkan: escrow -dana putaran | penerima giliran +dana putaran
```

Seluruhnya dalam **satu transaksi**. Gagal di mana pun membatalkan semuanya: tidak ada iuran
setengah lunas, saldo terpotong tanpa iuran lunas, atau putaran lunas tanpa pencairan.

## Rincian yang dibayar

`total = iuran + biaya platform + denda`

| Komponen | Aturan | Tujuan dana |
|---|---|---|
| Iuran | `contribution_amount` grup | escrow, lalu penerima giliran |
| Biaya platform | iuran × `platform_fee_bps` / 10.000, dibulatkan setengah ke atas | dompet `platform` |
| Denda | `late_fee` grup jika dibayar setelah jatuh tempo (akhir hari WIB) | dompet `platform` |

Dana yang dicairkan ke penerima giliran adalah **jumlah iuran saja** (slot × iuran); biaya dan
denda tidak ikut. `platform_fee_bps` dibaca dari pengaturan admin di dalam transaksi
pembayaran, lalu disimpan pada iuran (`contributions.platform_fee`), jadi perubahan pengaturan
tidak mengubah iuran yang sudah dibayar.

## Kuotasi

`GET /api/contributions/{id}/quote` menampilkan total yang harus dibayar **saat ini** (termasuk
denda jika sudah lewat jatuh tempo), apakah bisa dibayar (`payable`, atau alasan penolakannya),
saldo pengguna, dan apakah saldo cukup. Tidak mengubah apa pun.

## Saldo tidak cukup

Respons `402 Payment Required` memuat angkanya supaya klien bisa menampilkan kekurangan:

```json
{"error": "saldo tidak cukup", "required": 102500, "balance": 100000}
```

Tidak ada jejak yang tersisa: iuran tetap belum lunas, tanpa biaya atau entri ledger.

## Penerima giliran yang juga pembayar terakhir

Ia membayar iuran lalu langsung menerima dana putaran dalam transaksi yang sama. Saldo pada
respons sudah memuat pencairan (`saldo awal − total + dana putaran`).

## Mengapa ada dua penguncian

**Kunci putaran.** Beberapa anggota membayar bersamaan. Tanpa serialisasi, dua pembayar terakhir
sama-sama menghitung "masih ada yang belum membayar" (tidak melihat pembayaran lawannya yang
belum commit), sehingga tidak ada yang menutup putaran dan dana tertahan di escrow selamanya.
Saat ini kunci dompet escrow kebetulan sudah menyerialkan semua pembayaran; kunci putaran
dipertahankan agar kebenaran tidak bergantung pada detail itu (mis. escrow per grup kelak).

**Kunci dompet di awal.** Langkah 4 dan 5 memanggil `wallet.Post` dua kali. Jika dompet dikunci
bertahap, dua transaksi pada putaran berbeda bisa mengunci dompet dalam urutan berlawanan dan
saling menunggu. Contohnya: R adalah penerima giliran di grup 1 sekaligus pembayar terakhir di
grup 2, sementara A menutup putaran grup 1 pada saat yang sama. Tes `TestNoDeadlockWhen...`
mereproduksi skenario ini; tanpa penguncian di awal, Postgres melaporkan `deadlock detected`.
`wallet.LockWallets` mengunci semua dompet sekaligus dalam urutan id yang tetap.

## Endpoint

| Endpoint | Fungsi |
|---|---|
| `POST /api/contributions/{id}/pay` | Bayar iuran dari saldo; menutup dan mencairkan putaran jika terakhir |
| `GET /api/contributions/{id}/quote` | Rincian pembayaran dan kecukupan saldo |

Iuran milik orang lain atau yang tidak ada mendapat 404 (keberadaannya tidak bocor). Membayar
dua kali 409, putaran belum dibuka atau sudah ditutup 409, saldo kurang 402.
