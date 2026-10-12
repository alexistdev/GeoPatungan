# Top-up saldo lewat transfer bank manual

Kode: `backend/internal/topup/`, migrasi `000010_create_topups`.

Saldo diisi dengan mentransfer ke rekening yang diatur admin (lihat [admin.md](admin.md)),
lalu admin memverifikasi mutasi bank dan menyetujui. Persetujuan mengkreditkan saldo lewat
`wallet.Post` ([wallet.md](wallet.md)): `external −X`, `pengguna +X`.

## Alur

```
pengguna: POST /api/wallet/topups {amount}
              → status pending, nominal transfer = amount + kode unik, rekening tujuan ditampilkan
pengguna: (transfer lewat bank)
pengguna: POST /api/wallet/topups/{id}/confirm {sender_name, sender_bank, note}
              → status submitted
admin:    GET  /api/admin/topups                   → antrean (pending + submitted, terlama dulu)
admin:    POST /api/admin/topups/{id}/approve      → approved, saldo dikreditkan
          POST /api/admin/topups/{id}/reject {note} → rejected, saldo tidak berubah
```

| Status | Arti |
|---|---|
| `pending` | Menunggu pengguna mentransfer |
| `submitted` | Pengguna menyatakan sudah transfer; menunggu verifikasi admin |
| `approved` / `rejected` | Final, diputuskan admin |
| `cancelled` | Dibatalkan pengguna (hanya saat `pending`) |
| `expired` | `pending` yang melewati `topup_expiry_hours` |

## Kode unik

Nominal transfer = `amount` + kode acak 1..999 (mis. Rp100.000 → Rp100.663). Admin cukup
mencari angka itu di mutasi bank. Indeks unik di database menjamin **dua permintaan terbuka
tidak pernah punya nominal transfer yang sama**, sehingga satu mutasi cocok dengan tepat satu
permintaan. Yang dibandingkan adalah total transfer, bukan kodenya.

Kode dipilih dari 25 kandidat acak; jika semua bentrok, permintaan ditolak dengan "coba lagi".
Kode bebas kembali saat permintaan dibatalkan, ditolak, atau kedaluwarsa.

## Aturan

- Nominal harus dalam rentang `topup_min_amount`..`topup_max_amount` (pengaturan admin).
- **Ditolak (503) jika admin belum mengatur rekening aktif**: pengguna tidak punya tempat mentransfer.
- Maksimal **3 permintaan terbuka** per pengguna (`pending` + `submitted`). Pembuatan serentak
  tidak bisa melewati batas (kunci advisory per pengguna).
- **Kedaluwarsa dilakukan malas** (saat ada permintaan baru), hanya untuk `pending`.
  `submitted` **tidak** kedaluwarsa: uangnya mungkin sudah masuk, jadi admin yang memutuskan.
- Admin **tetap boleh menyetujui** permintaan `expired` (transfer bisa datang terlambat).
- Pengguna **tidak bisa membatalkan** permintaan `submitted`.
- Pengguna hanya melihat permintaannya sendiri (orang lain mendapat 404).

## Pemisahan tugas

**Admin tidak boleh menyetujui atau menolak permintaan top-up miliknya sendiri** (403).
Tanpa aturan ini, satu orang bisa mengisi saldo dirinya tanpa ada transfer sungguhan.

## Nominal yang disesuaikan

Jika pengguna salah mentransfer (mis. lupa kode unik), admin dapat menyetujui dengan
`received_amount` sebesar yang benar-benar diterima (1 sampai Rp1.000.000.000). Saldo
dikreditkan sebesar itu; `transfer_amount` asli tidak berubah sehingga selisihnya terlihat.
Tanpa `received_amount`, yang dikreditkan sebesar `transfer_amount`.

## Keamanan transaksi

Persetujuan berjalan dalam **satu transaksi** dengan baris permintaan terkunci (`FOR UPDATE`):
cek status → `wallet.Post` → ubah status → catat audit. Akibatnya:

- dua persetujuan serentak hanya mengkreditkan sekali;
- admin lain yang menolak pada saat yang sama menang atau kalah secara bersih, tidak mungkin
  berakhir "ditolak" di audit tetapi uangnya sudah masuk;
- jika ada langkah yang gagal, tidak ada saldo atau status yang berubah.

Lapis kedua: ledger menolak referensi `topup:{id}` yang sama dicatat dua kali.

## Endpoint

| Endpoint | Siapa | Fungsi |
|---|---|---|
| `POST /api/wallet/topups` `{amount}` | Pengguna | Buat permintaan; respons memuat rekening tujuan |
| `GET /api/wallet/topups` | Pengguna | Daftar permintaan saya |
| `GET /api/wallet/topups/{id}` | Pengguna | Detail (rekening tujuan hanya selagi terbuka) |
| `POST /api/wallet/topups/{id}/confirm` | Pengguna | Nyatakan sudah transfer |
| `POST /api/wallet/topups/{id}/cancel` | Pengguna | Batalkan (hanya `pending`) |
| `GET /api/admin/topups?status=open\|all\|…&limit=` | Admin | Antrean dan riwayat, dengan identitas pemohon |
| `POST /api/admin/topups/{id}/approve` | Admin | Setujui; body opsional `{received_amount, note}` |
| `POST /api/admin/topups/{id}/reject` `{note}` | Admin | Tolak; alasan wajib |

Setiap persetujuan dan penolakan tercatat di `audit_logs` (pelaku, pemohon, nominal).
