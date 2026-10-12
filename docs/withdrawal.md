# Penarikan dana ke rekening bank

Kode: `backend/internal/withdrawal/`, migrasi `000011_create_withdrawals`.

Pengguna menarik saldo ke rekening banknya; admin mentransfer **secara manual** lalu menandai
selesai. Pergerakan uangnya dicatat lewat `wallet.Post` ([wallet.md](wallet.md)).

## Alur dan perpindahan dana

```
pengguna: POST /api/wallet/withdrawals {amount, bank_name, account_number, account_holder, note}
            user -X → holding +X                      status pending
admin:    (mentransfer manual ke rekening pengguna lewat bank)
admin:    POST /api/admin/withdrawals/{id}/complete {reference, note}
            holding -X → external +X                  status completed   (uang keluar sistem)
admin:    POST /api/admin/withdrawals/{id}/reject {note}
            holding -X → user +X                      status rejected    (dana kembali)
pengguna: POST /api/wallet/withdrawals/{id}/cancel
            holding -X → user +X                      status cancelled   (hanya saat pending)
```

| Status | Arti |
|---|---|
| `pending` | Dana sudah dipotong dari saldo dan tertahan di `holding`; menunggu admin |
| `completed` | Admin sudah mentransfer; `transfer_reference` (nomor bukti bank) wajib diisi |
| `rejected` | Ditolak admin dengan alasan; dana kembali |
| `cancelled` | Dibatalkan pengguna selagi `pending`; dana kembali |

## Mengapa dana langsung dipindahkan ke `holding`

Saat permintaan dibuat, dana **langsung keluar dari saldo pengguna**. Dengan begitu dana tidak
bisa dipakai dua kali (dibayarkan untuk iuran atau ditarik lagi) selama menunggu admin. Pengecekan
saldo dilakukan oleh `wallet.Post` di bawah kunci dompet, sehingga **permintaan serentak tidak bisa
menarik lebih dari saldo**: dari delapan permintaan Rp100.000 serentak pada saldo Rp100.000, hanya
satu yang berhasil (tes `TestCreateConcurrentCannotDoubleSpend`). Permintaan yang gagal tidak
meninggalkan baris karena transaksinya dibatalkan utuh.

## Aturan

- Nominal minimum mengikuti `withdraw_min_amount` (pengaturan admin); maksimum Rp1.000.000.000
  per permintaan.
- Rekening: nama bank 2-50 karakter, nomor 5-30 digit angka (spasi dibuang), nama pemilik 2-100.
- Maksimal **3 permintaan `pending`** per pengguna; pembuatan serentak tidak bisa melewati batas
  (kunci advisory per pengguna).
- **Admin tidak boleh menyelesaikan atau menolak penarikan miliknya sendiri** (403), supaya satu
  orang tidak bisa mengeluarkan uang tanpa orang kedua. Admin tetap boleh **membatalkan**
  permintaannya sendiri (itu bukan peninjauan).
- `reference` wajib saat menyelesaikan, agar ada jejak bahwa uang benar-benar dikirim.
- Penolakan wajib disertai alasan.
- Pengguna hanya melihat permintaannya sendiri (orang lain mendapat 404). Admin melihat identitas
  pemohon dan **rekening tujuan** yang dibutuhkan untuk mentransfer.

## Keamanan transaksi

Setiap perubahan status berjalan dalam satu transaksi dengan baris permintaan terkunci
(`FOR UPDATE`): cek status `pending` → `wallet.Post` → ubah status → audit. Akibatnya:

- admin yang menyelesaikan dan pengguna yang membatalkan pada saat yang sama tidak mungkin
  keduanya berhasil: uang tidak mungkin terkirim ke bank **dan** kembali ke saldo
  (`TestCancelLocksRequestAgainstConcurrentComplete`);
- `Complete`, `Reject`, dan `Cancel` serentak atas satu permintaan menghasilkan tepat satu
  penyelesaian di ledger (`TestConcurrentSettlementsExactlyOneWins`);
- ledger menolak peristiwa (`withdrawal_complete|reject|cancel` + id) yang sama dicatat dua kali.

`CHECK` di database menjaga konsistensi baris: `completed` wajib punya `transfer_reference` dan
`reviewed_at`; `pending` tidak boleh punya `reviewed_at`.

## Endpoint

| Endpoint | Siapa | Fungsi |
|---|---|---|
| `POST /api/wallet/withdrawals` | Pengguna | Minta penarikan; saldo kurang → `402` dengan `required` dan `balance` |
| `GET /api/wallet/withdrawals` | Pengguna | Daftar permintaan saya |
| `GET /api/wallet/withdrawals/{id}` | Pengguna | Detail |
| `POST /api/wallet/withdrawals/{id}/cancel` | Pengguna | Batalkan (hanya `pending`) |
| `GET /api/admin/withdrawals?status=pending\|completed\|rejected\|cancelled\|all&limit=` | Admin | Antrean dan riwayat; bawaan `pending`, terlama dulu |
| `POST /api/admin/withdrawals/{id}/complete` `{reference, note}` | Admin | Tandai sudah ditransfer |
| `POST /api/admin/withdrawals/{id}/reject` `{note}` | Admin | Tolak; dana kembali |

Penyelesaian dan penolakan oleh admin tercatat di `audit_logs`.

## Yang belum ada

Tidak ada verifikasi otomatis bahwa nama pemilik rekening cocok dengan nama akun; itu keputusan
admin saat memproses (alasan penolakan yang umum). Tidak ada notifikasi; pengguna melihat status
dengan memanggil API.
