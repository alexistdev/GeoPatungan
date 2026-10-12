# Pemberitahuan (Notifikasi Dalam Aplikasi)

Pengguna menerima pemberitahuan dalam aplikasi untuk kejadian yang berkaitan dengan dirinya.
Pengiriman email/WhatsApp/push belum ada (di luar cakupan studi kasus); tabel dan API ini menjadi
dasarnya, dan frontend cukup memanggil API di bawah.

## Prinsip

| Prinsip | Cara dijaga |
|---|---|
| Tidak ada pemberitahuan hantu, tidak ada kejadian tanpa pemberitahuan | `notification.Send(ctx, tx, ...)` ditulis di **transaksi yang sama** dengan kejadiannya. Kalau pemberitahuan gagal ditulis, kejadiannya ikut batal (diuji dengan trigger yang memblokir) |
| Hanya pemilik yang bisa melihat/menandai | semua query memakai `user_id` pengguna login; milik orang lain = 404 |
| Tidak membocorkan pihak lain | komisi referral hanya menyebut level dan jumlah, tanpa nama downline |
| Isi terbatas | judul ≤ 120 dan isi ≤ 500 karakter (validasi + CHECK database); catatan admin dipotong dengan `…` |

## Kejadian yang memicu pemberitahuan

| Jenis (`type`) | Penerima | Kapan |
|---|---|---|
| `topup.approved` / `topup.rejected` | pemilik top-up | admin memutuskan |
| `withdrawal.completed` / `withdrawal.rejected` | pemilik penarikan | admin memutuskan (pembatalan sendiri tidak diberi tahu) |
| `kyc.verified` / `kyc.rejected` | pemohon | admin memutuskan |
| `group.member_approved` / `group.member_rejected` | calon anggota | pemilik grup memutuskan |
| `group.started` | semua anggota **aktif** | kocokan/urutan manual selesai dan grup berjalan |
| `round.payout_received` | penerima giliran | iuran terakhir putaran dibayar dan dana dicairkan |
| `referral.commission` | upline penerima komisi | iuran downline dibayar |

Belum ada: pengingat jatuh tempo/keterlambatan. Itu membutuhkan penjadwal (job periodik), bukan kejadian
transaksional, sehingga dibahas terpisah.

## Database (migrasi 000014)

`notifications(id, user_id, type, title, body, data JSONB, read_at, created_at)`; indeks `(user_id, id DESC)`
untuk daftar dan indeks parsial `read_at IS NULL` untuk hitungan belum dibaca. Dihapus bersama penggunanya.

## API (wajib login)

| Metode | Path | Keterangan |
|---|---|---|
| GET | `/api/me/notifications?limit=20&before=<id>&unread=true` | terbaru dulu, paging kursor (`next_before`), sertakan `unread_count` |
| GET | `/api/me/notifications/unread-count` | untuk lencana |
| POST | `/api/me/notifications/{id}/read` | idempoten (waktu baca pertama dipertahankan); bukan milik sendiri = 404 |
| POST | `/api/me/notifications/read-all` | mengembalikan jumlah yang berubah |

`data` berisi referensi untuk tautan di aplikasi (mis. `group_id`, `topup_id`), bukan data pribadi.

## Menambah jenis pemberitahuan baru

Tambahkan konstanta di `model.go`, lalu panggil `notification.Send(ctx, tx, notification.Notice{...})` di
transaksi kejadiannya (atau `SendToGroupMembers` untuk seluruh anggota aktif grup). Tulis tes yang memastikan
penerimanya benar dan tidak ada pemberitahuan ketika kejadiannya gagal.
