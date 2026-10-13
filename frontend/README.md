# GeoPatungan Web (Angular)

Frontend GeoPatungan. Berkomunikasi dengan API backend lewat `/api`.

## Menjalankan

```bash
cd frontend
npm install
npm start          # http://localhost:4200, /api di-proxy ke http://localhost:8080 (proxy.conf.json)
npm test           # unit test (Vitest)
npm run build      # build produksi ke dist/
npm run test:ci    # unit test + tes pustaka kontrak + pemeriksa kontrak (lihat docs/contract-testing.md)
npm run theme:install -- /jalur/ke/velzon   # pasang aset tema Velzon (lihat docs/tema-velzon.md)
```

> Tema **Velzon** (komersial) tidak ada di repositori. Pasang secara lokal dengan `npm run theme:install`
> sebelum menjalankan aplikasi; tanpanya halaman tampil tanpa gaya. Detail dan catatan lisensi:
> [docs/tema-velzon.md](../docs/tema-velzon.md).

Backend harus berjalan di port 8080 (`cd backend && go run ./cmd/api`).

## Struktur

- `src/app/core`: layanan lintas fitur (auth, interseptor token, guard, model API, penanganan error)
- `src/app/layout`: kerangka halaman Velzon (topbar, sidebar, footer)
- `src/app/features`: halaman per fitur (auth, dashboard, notifications, ...)
- `src/app/shared`: pipe/komponen yang dipakai bersama (judul halaman, bingkai auth, form tindakan)

Token login disimpan di `localStorage`; peran (admin) selalu dibaca dari server, bukan dari token.
