# Tema Velzon (demo "modern")

Antarmuka memakai tema **Velzon 4.3.0** (Themesbrand), demo **modern**: sidebar vertikal gelap yang mengembang saat
disorot, topbar terang/gelap, Bootstrap 5, dan ikon Remix.

## Penting: lisensi

Velzon adalah tema **komersial**. Berkas tema **tidak ikut di repositori** (`frontend/public/velzon/` ada di
`.gitignore`) karena mendistribusikan ulang tema berlisensi dalam repositori publik umumnya melanggar lisensinya.
Yang ada di repositori hanyalah kode GeoPatungan yang memakai kelas CSS tema dan skrip pemasang.

Jika Anda memiliki lisensi yang mengizinkan, hapus dua baris `frontend/public/...` di `.gitignore` untuk
meng-commit asetnya, atau pasang secara lokal seperti di bawah.

## Memasang aset

Butuh unduhan Velzon Anda (folder yang berisi `dist/modern/assets`):

```bash
cd frontend
npm run theme:install -- /jalur/ke/velzon
```

Skrip `scripts/velzon/install.mjs` hanya menyalin set minimal ke `frontend/public/velzon/` (±1,7 MB, dari
±1,7 GB unduhan penuh):

| Berkas | Fungsi |
|---|---|
| `css/bootstrap.min.css`, `css/app.min.css` | gaya inti tema |
| `css/icons.css` | **dibangun ulang**: font + kelas dasar Material Design Icons (panah menu) dan Remix Icon hanya untuk ikon yang dipakai |
| `fonts/remixicon.woff2`, `fonts/materialdesignicons-webfont.woff2` | font ikon (hanya woff2) |
| `images/auth-one-bg.jpg` | latar halaman masuk/daftar |
| `js/bootstrap.bundle.min.js` | dropdown dan collapse (menu pengguna, submenu admin) |

Tidak dipakai dan tidak disalin: demo lain (default, minimal, material, creative, saas, interactive, galaxy,
corporate), ribuan berkas contoh halaman, pustaka grafik/editor/peta, ikon Boxicons/Line Awesome, gambar contoh,
file RTL, dan skrip layout bawaan (perilakunya ditulis ulang di Angular).

**Setelah menambah ikon baru** (`ri-...`) di template, jalankan ulang `npm run theme:install` agar ikon itu
ikut di `icons.css`. Tes `scripts/velzon/install.test.mjs` memeriksa bahwa setiap ikon yang dipakai ada di
`icons.css`.

## Bagaimana tema dipakai di Angular

- `src/index.html` memuat CSS tema dan menetapkan konfigurasi demo modern pada `<html>`:
  `data-layout="vertical" data-sidebar="dark" data-sidebar-size="sm-hover"`.
- `core/layout.ts` (`LayoutService`) meniru perilaku skrip tema: tombol hamburger (lg/sm di desktop, overlay di
  ponsel), tombol pin sidebar, dan mode terang/gelap (`data-bs-theme`, disimpan di `localStorage`, bawaannya
  mengikuti preferensi sistem).
- `layout/shell.*` adalah kerangka (topbar, sidebar, footer). Menu admin hanya tampil untuk admin, dan submenunya
  terbuka otomatis di halaman `/admin/*`.
- `shared/page-title.ts` memberi judul halaman + breadcrumb ala Velzon; `shared/auth-shell.ts` bingkai halaman
  masuk/daftar.
- Seluruh halaman memakai kelas bawaan tema (`card`, `btn btn-primary`, `form-control`, `badge bg-*-subtle`,
  `list-group`, `table`, `alert`, dst.). Gaya tambahan sengaja minim: `src/styles.scss`.

Font tema (IBM Plex Sans, Poppins) dimuat `app.min.css` dari Google Fonts; tanpa internet, peramban memakai font
sistem.
