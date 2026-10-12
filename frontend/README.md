# GeoPatungan Web (Angular)

Frontend GeoPatungan. Berkomunikasi dengan API backend lewat `/api`.

## Menjalankan

```bash
cd frontend
npm install
npm start          # http://localhost:4200, /api di-proxy ke http://localhost:8080 (proxy.conf.json)
npm test           # unit test (Vitest)
npm run build      # build produksi ke dist/
```

Backend harus berjalan di port 8080 (`cd backend && go run ./cmd/api`).

## Struktur

- `src/app/core`: layanan lintas fitur (auth, interseptor token, guard, model API, penanganan error)
- `src/app/layout`: kerangka halaman (header, navigasi, lencana notifikasi)
- `src/app/features`: halaman per fitur (auth, dashboard, notifications, ...)
- `src/app/shared`: pipe/komponen yang dipakai bersama

Token login disimpan di `localStorage`; peran (admin) selalu dibaca dari server, bukan dari token.
