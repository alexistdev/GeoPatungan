# Tes Kontrak Frontend–Backend

Tes unit frontend memakai respons buatan sendiri, sehingga tidak bisa mendeteksi URL atau bentuk data yang
salah terhadap backend sungguhan. Dua bug seperti itu pernah lolos (rute admin top-up salah, respons `quote`
yang bersarang). Tes kontrak menutup celah itu secara otomatis.

## Cara kerja

```
backend (sumber kebenaran)                          frontend
┌──────────────────────────────────┐   contract/    ┌─────────────────────────────────┐
│ router asli (internal/app)       │──routes.json──▶│ setiap this.http.<metode>(url)  │
│ alur lengkap lewat HTTP + DB asli│──samples.json─▶│ di src/app/core diperiksa:      │
│ merekam BENTUK respons & body    │                │  1. rute ada                    │
└──────────────────────────────────┘                │  2. tipe respons cocok          │
                                                    │  3. field body dikenal backend  │
                                                    └─────────────────────────────────┘
```

- `contract/routes.json`: daftar `METODE /rute` dari router asli (`chi.Walk`).
- `contract/samples.json`: bentuk (nama field + tipe, bukan nilai) dari setiap respons dan body permintaan yang
  terekam saat alur pemakaian lengkap dijalankan melalui router asli. Tipe gabungan ditulis `"null|string"`;
  field yang kadang tidak ada `"absent|number"`; objek yang kadang null dibungkus `{"$optional":…, "$when":"null"}`.
- Frontend membaca **tipe TypeScript yang sungguh dipakai** pada tiap panggilan HTTP (lewat TypeScript compiler
  API), jadi tidak ada daftar ekspektasi yang harus dirawat terpisah: mengubah `models.ts` otomatis ikut diperiksa.

## Menjalankan

```bash
# backend: rute selalu diperiksa; sampel respons butuh database (TEST_DATABASE_URL)
cd backend && go test ./internal/app/

# frontend: seluruh pemeriksaan sekaligus (unit test, tes pustaka kontrak, pemeriksa kontrak)
cd frontend && npm run test:ci

# hanya pemeriksa kontrak
cd frontend && npm run contract
```

## Jika tes gagal

| Gejala | Artinya | Tindakan |
|---|---|---|
| Backend: "daftar rute berubah" / "bentuk API berubah dari kontrak" | Anda mengubah rute atau bentuk respons | Pastikan perubahan disengaja, jalankan `UPDATE_CONTRACT=1 go test ./internal/app/`, commit `contract/*.json`, lalu jalankan `npm run contract` dan sesuaikan frontend |
| Frontend: "rute TIDAK ADA di backend" | URL atau metode salah | Perbaiki URL di `src/app/core` |
| Frontend: "tidak dikirim backend" / "mengirim number, frontend mengharapkan string" | Tipe di `models.ts` tidak sesuai kenyataan | Perbaiki tipe |
| Frontend: "backend dapat mengirim null" / "kadang tidak mengirim field" | Tipe harus `T \| null` atau `field?: T` | Perbaiki tipe, lalu pastikan UI menangani null |
| Frontend: "frontend dapat mengirim field ini, tetapi alur kontrak backend tidak pernah mengirimnya" | Body memuat field yang tidak dikenal (server menolak field asing) atau alur backend belum mencakupnya | Perbaiki body, atau tambahkan pemanggilan itu ke alur di `internal/app/samples_test.go` |
| Frontend: "tidak ada sampel respons 2xx" | Endpoint baru belum dicakup alur | Tambahkan ke alur di `internal/app/samples_test.go` lalu perbarui kontrak |
| Frontend: "URL tidak statis" | URL dibangun dari variabel | Tulis URL sebagai literal/template; query lewat `HttpParams` |

## Menambah endpoint baru

1. Backend: tambahkan pemanggilan ke alur `TestRecordAPIShapes` (isi semua field body yang bisa dikirim frontend).
2. `UPDATE_CONTRACT=1 go test ./internal/app/` lalu commit `contract/routes.json` dan `contract/samples.json`.
3. Frontend: tulis panggilan dengan tipe generik respons (`this.http.get<Tipe>(...)`) dan jalankan `npm run contract`.

## Batas yang perlu diketahui

- Yang dicocokkan adalah **bentuk**, bukan nilai atau aturan bisnis.
- Pemeriksaan hanya mencakup field yang dideklarasikan di tipe frontend; field tambahan dari backend diabaikan.
- Respons galat: setiap non-2xx harus memuat `error: string`; 402 pada bayar iuran/penarikan juga `required` dan `balance`.
- Alur backend hanya merekam skenario yang dijalankannya; cabang respons yang tak pernah muncul di alur tidak terekam.
