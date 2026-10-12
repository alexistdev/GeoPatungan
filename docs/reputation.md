# Reputasi dan verifikasi identitas (KYC)

Kode: `backend/internal/reputation/`, `backend/internal/kyc/`, migrasi `000012_create_reputation_kyc`.

## Level dihitung, bukan disimpan

Level seorang pengguna **tidak disimpan di database**. Ia dihitung setiap kali dibutuhkan dari tiga
sumber data nyata, sehingga tidak mungkin melenceng dari kenyataan:

| Fakta | Sumber |
|---|---|
| Terverifikasi | `users.kyc_status = 'verified'` |
| Arisan tuntas | grup berstatus `done` yang diikuti sebagai anggota **aktif** |
| Keterlambatan | iuran dibayar terlambat (`contributions.paid_on_time = false`) **+** iuran yang kini menunggak |

"Menunggak" = iuran belum dibayar pada putaran yang sedang `collecting` dan sudah lewat jatuh tempo
(hari ini menurut WIB). Iuran pada putaran yang belum dibuka tidak dihitung. Karena tunggakan ikut
dihitung, level **turun seketika** saat jatuh tempo terlewat, tanpa menunggu pembayaran.

## Level bawaan

Ambang ada di tabel `reputation_levels` dan bisa diubah admin tanpa deploy ulang.

| Level | Syarat | Bobot kocokan | Batas iuran grup |
|---|---|---|---|
| 1 Baru | (dasar, semua orang) | 1 | Rp500.000 |
| 2 Terpercaya | terverifikasi, ≥1 arisan tuntas, 0 keterlambatan | 2 | Rp5.000.000 |
| 3 Senior | terverifikasi, ≥3 arisan tuntas, 0 keterlambatan | 3 | tanpa batas |

Keterlambatan dihitung **seumur hidup**: satu keterlambatan menurunkan pengguna ke level 1 sampai
admin melonggarkan batasnya. Itu sengaja keras; ubah `max_late_count` jika ingin lebih longgar.

## Apa yang dipengaruhi level

| Hal | Aturan |
|---|---|
| Bobot kocokan `weighted` | `draw_weight` level pemilik slot, dikunci saat `commit` ([draw-algorithm.md](draw-algorithm.md)). Kocokan `random` mengabaikan level |
| Bergabung ke grup | level harus ≥ `min_level_rank` grup, dan iuran grup ≤ `max_group_value` level |
| Membuat grup | `can_create_group` harus true dan iuran ≤ `max_group_value` level (403 jika tidak) |

Validasi input (400) didahulukan dari pengecekan izin (403). Batas nilai membuat pengguna baru tidak
bisa langsung membuka grup bernilai besar, yang merupakan risiko penipuan terbesar.

## Konsistensi definisi level

`PUT /api/admin/reputation/levels` menolak definisi yang tidak masuk akal: level yang lebih tinggi
tidak boleh lebih mudah dicapai (verifikasi, arisan, batas keterlambatan) dan tidak boleh lebih
terbatas haknya (bobot, izin membuat grup, batas nilai). Level 1 tidak boleh punya syarat. Bobot
dibatasi 1 sampai 3 (rasio maksimal 3:1). Aturan dasarnya juga dijaga `CHECK` di database.
Setiap perubahan tercatat di `audit_logs`, dan **berlaku seketika untuk semua pengguna**.

## KYC (verifikasi identitas manual)

Admin memutuskan secara manual, selaras dengan top-up dan penarikan. Sistem **sengaja tidak menyimpan
nomor identitas atau dokumen**: hanya permintaan, keputusan, dan catatannya.

```
pengguna: POST /api/me/kyc {note}               → pending
admin:    POST /api/admin/kyc/{id}/approve      → verified
          POST /api/admin/kyc/{id}/reject {note}→ rejected (pengguna boleh mengajukan lagi)
```

- Satu permintaan `pending` per pengguna (kunci baris pengguna + indeks unik).
- Pengguna yang sudah `verified` tidak perlu mengajukan lagi.
- **Admin tidak boleh memverifikasi dirinya sendiri**, kalau tidak seorang admin bisa menaikkan
  reputasinya sendiri.
- Keputusan final tidak bisa dibalik diam-diam, dan status akun selalu cocok dengan keputusan yang menang.

## Endpoint

| Endpoint | Siapa | Fungsi |
|---|---|---|
| `GET /api/me/reputation` | Pengguna | Level, statistik, dan kemajuan menuju level berikutnya |
| `GET /api/me/kyc` | Pengguna | Status verifikasi dan permintaan terakhir |
| `POST /api/me/kyc` | Pengguna | Ajukan verifikasi |
| `GET /api/admin/kyc?status=pending\|verified\|rejected\|all` | Admin | Antrean KYC, terlama dulu |
| `POST /api/admin/kyc/{id}/approve`, `/reject` | Admin | Putuskan permintaan |
| `GET /api/admin/reputation/levels` | Admin | Definisi level |
| `PUT /api/admin/reputation/levels` | Admin | Ubah definisi level (divalidasi) |

`GET /api/me/reputation` menampilkan tiap syarat menuju level berikutnya beserta kemajuannya,
mis. `[v] Akun terverifikasi (1/1) ; [ ] Menuntaskan 3 arisan (1/3)`.

## Yang belum ada

Pencabutan verifikasi (admin membatalkan status `verified`) belum tersedia. Tidak ada riwayat
perubahan level per pengguna karena level selalu dihitung; jejak perubahan ambang ada di audit log.
