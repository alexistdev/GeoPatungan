# Kocokan giliran arisan (`weighted_es_v1`)

Dokumen ini menjelaskan cara urutan giliran ditentukan dan cara siapa pun
memverifikasinya. Kodenya ada di `backend/internal/draw/`.

## Alur

1. **Grup penuh.** Semua slot terisi anggota aktif.
2. **Commit** (`POST /api/groups/{id}/draw/commit`, pemilik grup).
   Server membuat seed acak 32 byte, menyimpannya rahasia, dan mengumumkan
   `seed_commit = sha256(seed)`. Bobot tiap anggota dikunci pada saat ini
   dan terlihat oleh semua anggota (`GET /api/groups/{id}/draw`).
3. **Execute** (`POST /api/groups/{id}/draw/execute`, pemilik grup).
   Urutan dihitung dari seed, disimpan, dan seed dibuka (`seed_reveal`).
   Grup berubah menjadi `running`.
4. **Verifikasi** oleh siapa pun yang punya `seed_reveal` dan daftar `entries`.

Hanya ada **satu kocokan per grup**. Tidak ada ulang kocok, supaya hasil tidak
bisa dicoba berkali-kali sampai "bagus".

Grup ber-`draw_method = manual` memakai `POST /api/groups/{id}/draw/manual`
(urutan ditentukan pemilik) dan tidak memakai seed.

## Bobot

| Metode grup | Bobot anggota |
|---|---|
| `random` | semuanya 1 |
| `weighted` | level reputasi 1 → 1, level 2 → 2, level 3 → 3 |

Rasio maksimal 3:1. Anggota berbobot lebih tinggi **cenderung** mendapat giliran
awal, tetapi tidak dijamin. Untuk dua anggota berbobot 3 dan 1, peluang yang
berbobot 3 menjadi pertama adalah 3/(3+1) = 75%.

## Perhitungan urutan

Untuk tiap anggota `i` dengan `member_id` dan `weight`:

```
mac   = HMAC-SHA256(key = seed (byte mentah), message = member_id (UTF-8))
x     = 8 byte pertama mac sebagai uint64 big-endian, digeser kanan 11 bit
u     = (x + 0.5) / 2^53                  # tepat di dalam (0, 1)
score = ln(u) / weight
```

Urutkan **menurun** berdasarkan `score` (seri, yang praktis mustahil, dipecah
dengan `member_id` naik). Posisi pertama = giliran 1, dan seterusnya.

Ini sampling berbobot tanpa pengembalian (Efraimidis-Spirakis): mengurutkan
`ln(u)/w` sama dengan mengurutkan `u^(1/w)`.

## Verifikasi mandiri (Python)

```python
import hashlib, hmac, math, struct

def verify(seed_hex, seed_commit, entries):
    """entries: daftar (member_id, weight) dari GET /draw. Mengembalikan urutan member_id."""
    seed = bytes.fromhex(seed_hex)
    assert hashlib.sha256(seed).hexdigest() == seed_commit.lower(), "seed tidak cocok dengan komitmen"

    scored = []
    for member_id, weight in entries:
        digest = hmac.new(seed, member_id.encode(), hashlib.sha256).digest()
        x = struct.unpack(">Q", digest[:8])[0] >> 11
        u = (x + 0.5) / 2**53
        scored.append((math.log(u) / weight, member_id))

    scored.sort(key=lambda t: (-t[0], t[1]))
    return [member_id for _, member_id in scored]
```

Bandingkan hasilnya dengan urutan `turn_order` dari server. Perbedaan hanya
mungkin pada selisih pembulatan `ln` di tingkat bit terakhir antar pustaka
matematika, yang tidak mengubah urutan kecuali dua skor nyaris identik.

## Batasan yang perlu diketahui

- Seed dibuat dan disimpan **server**. Komitmen membuktikan bahwa seed tidak
  diganti setelah diumumkan dan bahwa hasil bisa dihitung ulang. Ia **tidak**
  membuktikan operator tidak mencoba banyak seed sebelum mengumumkan salah satunya.
  Untuk menutup celah itu diperlukan sumber keacakan eksternal yang tak bisa
  diprediksi operator (misalnya beacon publik seperti drand) atau kontribusi
  entropi dari anggota.
- Seed tersimpan di database sampai dibuka; siapa pun yang punya akses langsung
  ke database sebelum `execute` bisa melihatnya.
