import { HttpErrorResponse } from '@angular/common/http';

import { errorMessage } from './errors';

const http = (status: number, error: unknown) => new HttpErrorResponse({ status, error });

describe('errorMessage', () => {
  it('memakai pesan dari server bila ada', () => {
    expect(errorMessage(http(401, { error: 'email atau password salah' }))).toBe('email atau password salah');
  });

  it('server mati (status 0) -> pesan koneksi', () => {
    expect(errorMessage(http(0, null))).toContain('terhubung');
  });

  it('500 tanpa pesan tidak membocorkan detail', () => {
    const msg = errorMessage(http(500, '<html>stack trace</html>'));
    expect(msg).not.toContain('stack');
    expect(msg).toContain('server');
  });

  it('429 dan error asing punya pesan umum', () => {
    expect(errorMessage(http(429, null))).toContain('Terlalu banyak');
    expect(errorMessage(new Error('x'))).toBe('Terjadi kesalahan. Coba lagi.');
    expect(errorMessage(http(400, { error: '   ' }))).toBe('Terjadi kesalahan. Coba lagi.');
  });
});
