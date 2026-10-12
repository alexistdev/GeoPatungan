# Referral Multi-Level (4 Level)

Pengguna mengajak pengguna lain lewat **kode referral**. Setiap kali orang yang diajak (sampai 4
tingkat ke bawah) membayar iuran, upline-nya menerima **komisi dari biaya platform**.

> Studi kasus open source. Ini bukan skema piramida/Ponzi: tidak ada komisi dari biaya masuk atau dana
> anggota, tidak ada komisi tanpa transaksi nyata, dan total komisi dibatasi oleh pendapatan platform.

## Prinsip

| Prinsip | Cara dijaga |
|---|---|
| Sumber komisi hanya **biaya platform** atas iuran yang benar-benar dibayar | `referral.Amounts(plan, receipt.PlatformFee)`; denda keterlambatan dan dana arisan tidak pernah dikomisikan |
| Total bagian semua level ≤ 100% dari biaya | validasi service + trigger database (`trg_commission_rules_total`, ditunda ke commit) + pagar di `Amounts` |
| Platform tidak pernah defisit | komisi dibulatkan **ke bawah** per level; sisa tetap di platform |
| Hanya yang terverifikasi (KYC) yang menerima | `Plan` hanya memilih upline `kyc_status = 'verified'`; bagian upline lain tetap di platform |
| Maksimal 4 level | tabel `referrals` (closure) diisi saat registrasi, level 1..4 |
| Tidak bisa dimanipulasi sesudah fakta | aturan berlaku untuk pembayaran berikutnya; komisi yang sudah dibayar menyimpan `share_bps`-nya sendiri |

## Aturan bawaan

| Level | Siapa | Bagian dari biaya platform |
|---|---|---|
| 1 | yang mengajak langsung | 40% |
| 2 | upline dari level 1 | 25% |
| 3 | | 15% |
| 4 | | 10% |
| | **Total** | **90%** (platform menyimpan ≥ 10%) |

Contoh: iuran Rp100.000, biaya 2% → biaya platform Rp2.000 → komisi Rp800 / Rp500 / Rp300 / Rp200
(Rp1.800), platform menyimpan Rp200. Biaya Rp999 → 399 / 249 / 149 / 99 (sisa 103 di platform).

## Alur uang

Terjadi di **transaksi yang sama** dengan pembayaran iuran (`internal/payment`):

1. Kunci semua dompet yang terlibat sekaligus, urut id: pembayar, penerima pencairan, escrow, platform,
   **dan dompet semua upline penerima**. Ini mencegah deadlock ketika upline sedang membayar di grup lain.
2. `wallet.Post` pembayaran: pembayar → escrow (iuran) + platform (biaya + denda).
3. `wallet.Post` komisi (ref `referral_commission`, id = id iuran): platform → dompet tiap upline.
4. `referral.Record` menyimpan baris di `referral_commissions` (UNIQUE `(contribution_id, level)`).
5. Bila ini pembayaran terakhir, putaran ditutup dan escrow dicairkan seperti biasa.

Jika salah satu langkah gagal (mis. referensi komisi sudah pernah tercatat), seluruh pembayaran
dibatalkan: pembayar tidak terpotong, tidak ada upline yang menerima apa pun.

## Database (migrasi 000013)

- `commission_rules(level 1..4, share_bps 0..10000)`: aturan, dapat diubah admin.
- `referral_commissions`: riwayat komisi (penerima, sumber, level, bagian, jumlah > 0, penerima ≠ sumber).

## API

Pengguna (login):

| Metode | Path | Keterangan |
|---|---|---|
| GET | `/api/me/referral` | kode referral, status eligible (KYC), aturan, jumlah downline per level, total komisi per level |
| GET | `/api/me/referral/commissions?limit=20&before=<id>` | riwayat komisi, terbaru dulu, paging kursor. Nama sumber disamarkan (`B*** S***`) |

Admin:

| Metode | Path | Keterangan |
|---|---|---|
| GET | `/api/admin/referral/rules` | aturan saat ini |
| PUT | `/api/admin/referral/rules` | `{"rules":[{"level":1,"share_bps":4000}, ... 4 level]}`; total > 100% ditolak (400); tercatat di audit (`referral.rules.update`) |

## Pengujian

- Unit: `Amounts` (pembulatan, pagar total, fee 0/negatif), `ValidateRules`, `maskName`, service, handler.
- Integrasi (butuh `TEST_DATABASE_URL`): rantai A→B→C→D→E membayar 800/500/300/200; hanya upline
  terverifikasi; tanpa biaya = tanpa komisi; denda tidak dikomisikan; aturan berlaku saat pembayaran;
  atomisitas; **deadlock** (stres 25× + uji deterministik bahwa dompet upline dikunci sebelum tulisan
  pertama); pembayaran bersamaan ke upline yang sama; trigger total 100%; audit.
