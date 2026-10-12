import { HttpClient, provideHttpClient, withInterceptors } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { TestBed } from '@angular/core/testing';
import { Router, provideRouter } from '@angular/router';

import { AuthService } from './auth';
import { authInterceptor } from './auth-interceptor';

function setup(token: string | null) {
  localStorage.clear();
  if (token) localStorage.setItem('gp.token', token);
  TestBed.configureTestingModule({
    providers: [provideRouter([]), provideHttpClient(withInterceptors([authInterceptor])), provideHttpClientTesting()],
  });
  const nav = vi.spyOn(TestBed.inject(Router), 'navigateByUrl').mockResolvedValue(true);
  return { http: TestBed.inject(HttpClient), ctl: TestBed.inject(HttpTestingController), auth: TestBed.inject(AuthService), nav };
}

describe('authInterceptor', () => {
  afterEach(() => TestBed.inject(HttpTestingController).verify());

  it('menyisipkan Bearer token ke API', () => {
    const { http, ctl } = setup('tok');
    http.get('/api/me').subscribe();
    expect(ctl.expectOne('/api/me').request.headers.get('Authorization')).toBe('Bearer tok');
  });

  it('tidak mengirim token ke login/register', () => {
    const { http, ctl } = setup('tok');
    http.post('/api/auth/login', {}).subscribe();
    expect(ctl.expectOne('/api/auth/login').request.headers.has('Authorization')).toBe(false);
  });

  it('tidak mengirim token ke alamat di luar API', () => {
    const { http, ctl } = setup('tok');
    http.get('https://pihak-lain.example/data').subscribe();
    expect(ctl.expectOne('https://pihak-lain.example/data').request.headers.has('Authorization')).toBe(false);
  });

  it('401 pada API mengeluarkan pengguna', () => {
    const { http, ctl, auth, nav } = setup('tok');
    http.get('/api/me').subscribe({ error: () => undefined });
    ctl.expectOne('/api/me').flush({}, { status: 401, statusText: 'x' });
    expect(auth.isLoggedIn()).toBe(false);
    expect(nav).toHaveBeenCalledWith('/masuk');
  });

  it('401 saat login salah TIDAK mengalihkan (pesan error tetap tampil)', () => {
    const { http, ctl, nav } = setup(null);
    let failed = false;
    http.post('/api/auth/login', {}).subscribe({ error: () => (failed = true) });
    ctl.expectOne('/api/auth/login').flush({ error: 'salah' }, { status: 401, statusText: 'x' });
    expect(failed).toBe(true);
    expect(nav).not.toHaveBeenCalled();
  });

  it('error selain 401 tidak mengeluarkan pengguna', () => {
    const { http, ctl, auth } = setup('tok');
    http.get('/api/wallet/').subscribe({ error: () => undefined });
    ctl.expectOne('/api/wallet/').flush({}, { status: 500, statusText: 'x' });
    expect(auth.isLoggedIn()).toBe(true);
  });
});
