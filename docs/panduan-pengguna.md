# Panduan Pengguna GeoPatungan

Panduan mencoba seluruh alur aplikasi: admin, pemilik grup, dan anggota, dari mengisi saldo sampai kocokan,
pembayaran iuran, dan pencairan.

> Aplikasi ini studi kasus open source. Semua transfer bank bersifat simulasi: tidak ada uang sungguhan yang
> berpindah. Admin yang "memverifikasi" transfer secara manual.

## Prasyarat

Docker (database), backend, dan frontend berjalan:

```bash
# 1. database
docker compose up -d

# 2. backend (terminal terpisah), http://localhost:8080
cd backend && go run ./cmd/api

# 3. frontend (terminal terpisah), http://localhost:4200
cd frontend && npm start
```

Cek backend: `curl http://localhost:8080/health` harus menjawab `{"status":"ok"}`.

Tampilan memakai tema Velzon yang tidak ikut di repositori (komersial). Pasang sekali dari unduhan Velzon Anda,
lalu jalankan ulang bila menambah ikon baru: `cd frontend && npm run theme:install -- /jalur/ke/velzon`
(lihat [tema-velzon.md](tema-velzon.md)).

## Persiapan: siapkan beberapa akun

Token login disimpan per browser, jadi satu browser hanya bisa satu akun sekaligus. Pakai **jendela
private/incognito**, atau browser lain, untuk tiap akun:

| Peran | Cara mendapatkan |
|---|---|
| Admin | daftar di `/daftar`, lalu naikkan perannya (lihat di bawah) |
| Pemilik grup (mis. Andi) | daftar di `/daftar` |
| Anggota (mis. Budi, Cici) | daftar di `/daftar` |

Menjadikan akun sebagai admin (peran hanya bisa diberikan lewat CLI, tidak lewat aplikasi):

```bash
cd backend && go run ./cmd/admin promote email-anda@example.com
# mencabut: go run ./cmd/admin demote email-anda@example.com
```

Perannya langsung berlaku. Admin **tidak boleh memproses permintaannya sendiri** (top-up, penarikan, KYC), jadi
jangan memakai akun admin sebagai anggota yang top-up.

## 1. Admin: atur platform (sekali saja)

Login admin, menu **Admin → Pengaturan**:

1. **Tambah rekening** (bank, nomor, nama pemilik). Tanpa rekening aktif, pengguna tidak bisa top-up.
2. Opsional, **Ubah** biaya platform, mis. 2%. Dari biaya inilah komisi referral dibayarkan. Bawaannya 0%.

## 2. Anggota: isi saldo

Login sebagai tiap pemilik dan anggota, menu **Dompet → Isi saldo**:

1. Masukkan nominal (minimal Rp10.000), lalu **Buat permintaan top-up**. Muncul nominal transfer tepat (termasuk
   kode unik) dan rekening tujuan.
2. Klik **Saya sudah transfer**, isi nama dan bank pengirim, lalu kirim. Transfer sungguhan tidak diperlukan.
3. Login admin, buka **Admin → Top-up** (status "Menunggu verifikasi"), lalu **Setujui**.
4. Saldo bertambah sebesar nominal transfer (termasuk kode unik), dan notifikasi muncul di lencana Notifikasi.

Ulangi untuk semua yang akan membayar iuran: pemilik dan semua anggota.

## 3. Pemilik: buat grup

Login pemilik, menu **Grup → Buat grup**:

- Isi nama, iuran per anggota (Rp10.000 sampai Rp100.000.000), jumlah anggota/slot (2 sampai 50), periode,
  tanggal mulai (paling cepat besok), dan denda keterlambatan (0 sampai sebesar iuran).
- Pilih **cara urutan penerima**: acak, berbobot reputasi, atau manual.
- Untuk uji cepat, pakai 2 atau 3 slot.

Setelah dibuat, halaman detail menampilkan **kode undangan** dan tombol **Salin tautan**. Kode hanya terlihat oleh
pemilik.

## 4. Anggota bergabung, pemilik menerima

1. Anggota login, **Grup → Gabung dengan kode**, tempel kode, lalu **Gabung** (atau buka tautan undangan).
2. Pemilik membuka detail grup. Di panel **Anggota** muncul "permintaan bergabung". Klik **Terima** (atau
   **Tolak**) untuk tiap orang. Pemohon mendapat notifikasi.
3. Saat anggota aktif sama dengan jumlah slot (mis. 3/3), grup penuh dan kocokan bisa dilakukan.

## 5. Kocokan (urutan penerima)

Dilakukan **pemilik** di panel **Urutan penerima** pada detail grup. Hanya bisa saat grup penuh.

**Acak atau berbobot reputasi:**

1. **Kunci kocokan.** Hash seed dipublikasikan lebih dulu agar hasil tidak bisa dimanipulasi. Semua anggota bisa
   melihat hash itu.
2. **Jalankan kocokan.** Urutan giliran muncul, grup berstatus **Berjalan**, dan jadwal putaran dibuat. Semua
   anggota mendapat notifikasi.
3. Anggota dapat membuka **Verifikasi kocokan** untuk melihat commit, seed, dan algoritma, lalu memeriksa hasilnya
   sendiri (lihat [draw-algorithm.md](draw-algorithm.md)).

Pada cara **berbobot**, level reputasi yang lebih tinggi memberi peluang lebih besar (bukan jaminan).

**Manual:** susun urutan dengan tombol ↑ ↓, lalu **Tetapkan urutan dan mulai grup**. Setelah ditetapkan, urutan
tidak bisa diubah.

## 6. Putaran dan bayar iuran

Di detail grup, bagian **Jadwal putaran**:

1. Putaran 1 berstatus **Pengumpulan iuran**. Tiap anggota (termasuk pemilik) klik **Bayar iuran**.
2. Muncul rincian: iuran, biaya platform, denda bila terlambat, total, dan saldo Anda. Klik **Bayar Rp…**.
3. Bila saldo kurang, tombol nonaktif dan ada tautan **Isi saldo**.
4. Setelah **semua** anggota membayar, putaran ditutup otomatis dan seluruh dana dicairkan ke penerima giliran.
   Penerima mendapat notifikasi, dan putaran berikutnya langsung dibuka.
5. **Lihat pembayar** menampilkan siapa yang sudah lunas, terlambat, atau menunggak.
6. Setelah putaran terakhir selesai, grup berstatus **Selesai**.

Jatuh tempo berlaku sampai akhir hari (WIB). Pembayaran setelahnya dikenai denda dan tercatat sebagai terlambat
(memengaruhi reputasi).

## 7. Penerima: tarik dana

1. Penerima buka **Dompet → Tarik dana** (minimal Rp50.000), isi rekening, lalu ajukan. Dana langsung ditahan.
   Pengguna masih bisa **Batalkan** selagi diproses.
2. Admin buka **Admin → Penarikan**. Setelah "mentransfer" (simulasi), klik **Sudah ditransfer** dan isi nomor
   referensi. Atau **Tolak** dengan alasan; dana kembali ke saldo pengguna.
3. Pengguna mendapat notifikasi hasilnya.

## Opsional

- **Verifikasi (KYC):** pengguna buka **Profil → Ajukan verifikasi**; admin memutuskan di
  **Admin → Verifikasi KYC**. Ini memenuhi syarat naik level reputasi dan syarat menerima komisi referral.
  Aplikasi tidak menyimpan nomor KTP.
- **Reputasi:** halaman **Profil** menampilkan level, hak, catatan pembayaran, dan syarat naik level.
- **Referral:** bagikan tautan dari halaman **Referral**. Orang yang mendaftar lewat tautan itu menjadi downline.
  Setiap kali mereka membayar iuran, upline yang sudah terverifikasi menerima komisi dari **biaya platform**
  (bukan dari dana arisan). Butuh biaya platform di atas 0%.
- **Aturan komisi dan level:** admin mengubahnya di **Admin → Komisi referral** dan **Admin → Level reputasi**.
- **Kesehatan ledger:** **Admin → Ringkasan** memeriksa bahwa seluruh catatan uang konsisten.

## Urutan cepat untuk uji penuh

Admin tambah rekening → daftar 3 akun → top-up dan setujui untuk ketiganya → pemilik buat grup 3 slot → dua
anggota gabung dan diterima → kunci lalu jalankan kocokan → ketiganya bayar iuran → penerima menerima dana →
penerima tarik, lalu admin menyelesaikan.

## Jika ada masalah

| Gejala | Penyebab dan tindakan |
|---|---|
| Frontend menampilkan 502 | Backend belum menyala atau port tidak sama dengan `frontend/proxy.conf.json` (8080) |
| "DATABASE_URL wajib diisi" / "JWT_SECRET wajib diisi" | `backend/.env` kurang isinya; `JWT_SECRET` minimal 32 karakter |
| "address already in use" | Port 8080 dipakai proses lain; hentikan atau jalankan `PORT=8081 go run ./cmd/api` dan sesuaikan proxy |
| Gagal konek database | `docker ps` harus menampilkan `geopatungan-db` berstatus Up |
| Top-up gagal: "rekening tujuan transfer belum diatur oleh admin" | Admin belum menambah rekening aktif (langkah 1) |
| Tidak bisa membuat kocokan | Grup belum penuh, atau Anda bukan pemilik grup |
| Tombol bayar nonaktif | Saldo kurang, atau putaran belum berstatus Pengumpulan iuran |
