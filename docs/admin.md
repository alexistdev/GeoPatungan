# Admin, pengaturan, dan rekening bank

Kode: `backend/internal/settings/`, `backend/internal/middleware/role.go`,
`backend/internal/audit/`, `backend/cmd/admin/`.

## Menjadikan seseorang admin

Peran admin **tidak bisa diberikan lewat API**; hanya operator server:

```bash
cd backend
go run ./cmd/admin promote user@example.com
go run ./cmd/admin demote  user@example.com
```

Peran dibaca dari **database di setiap permintaan admin**, bukan dari token.
Jadi promosi dan pencabutan hak berlaku seketika, tanpa menunggu token kedaluwarsa
atau login ulang.

## Endpoint admin (`/api/admin/*`, hanya peran admin)

| Endpoint | Fungsi |
|---|---|
| `GET /api/admin/settings` | Lihat pengaturan platform |
| `PUT /api/admin/settings` | Ubah pengaturan (seluruh objek, divalidasi) |
| `GET /api/admin/bank-accounts` | Semua rekening, termasuk yang nonaktif |
| `POST /api/admin/bank-accounts` | Tambah rekening tujuan transfer |
| `PUT /api/admin/bank-accounts/{id}` | Ubah atau nonaktifkan rekening |

Rekening **tidak dihapus**, hanya dinonaktifkan, supaya riwayat tetap utuh.

## Pengaturan platform

| Kunci | Bawaan | Batas | Arti |
|---|---|---|---|
| `platform_fee_bps` | 0 | 0 sampai 1000 | Biaya platform per iuran, basis poin (100 = 1%) |
| `topup_min_amount` | 10.000 | ≥ 1.000 | Top-up minimum (Rp) |
| `topup_max_amount` | 100.000.000 | ≥ min, ≤ 1 miliar | Top-up maksimum (Rp) |
| `withdraw_min_amount` | 50.000 | ≥ 10.000 | Penarikan minimum (Rp) |
| `topup_expiry_hours` | 24 | 1 sampai 168 | Lama permintaan top-up menunggu transfer |

Kunci yang belum pernah diatur memakai nilai bawaan. Nilai tersimpan yang rusak
(bukan angka) menghasilkan error, bukan diam-diam memakai bawaan.

## Untuk pengguna

`GET /api/payment-info` menampilkan **hanya rekening aktif** beserta batas nominal
dan biaya platform, supaya pengguna tahu ke mana harus mentransfer.

## Audit

Setiap perubahan pengaturan dan rekening dicatat di `audit_logs` (pelaku, aksi,
entitas, ringkasan perubahan) **dalam transaksi yang sama** dengan perubahannya:
perubahan yang gagal tidak meninggalkan catatan, dan perubahan yang berhasil
selalu tercatat.
