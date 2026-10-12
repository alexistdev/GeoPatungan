import { HttpErrorResponse } from '@angular/common/http';

/** Mengubah error HTTP menjadi pesan yang layak ditampilkan ke pengguna. */
export function errorMessage(err: unknown): string {
  if (err instanceof HttpErrorResponse) {
    if (err.status === 0) {
      return 'Tidak dapat terhubung ke server. Periksa koneksi Anda.';
    }
    const body = err.error as { error?: unknown } | null;
    if (body && typeof body.error === 'string' && body.error.trim()) {
      return body.error;
    }
    if (err.status >= 500) {
      return 'Terjadi kesalahan pada server. Coba lagi nanti.';
    }
    if (err.status === 429) {
      return 'Terlalu banyak permintaan. Coba lagi sebentar lagi.';
    }
  }
  return 'Terjadi kesalahan. Coba lagi.';
}
