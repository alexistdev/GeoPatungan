import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { TestBed } from '@angular/core/testing';
import { Router, provideRouter } from '@angular/router';

import { AuthService } from './auth';
import { User } from './models';

const user: User = {
  id: 'u1', name: 'Budi', phone: '0812345678', email: 'b@x.id', role: 'member',
  kyc_status: 'none', referral_code: 'ABCD2345', referred_by: null, created_at: '2026-01-01T00:00:00Z',
};

function setup() {
  TestBed.configureTestingModule({
    providers: [provideRouter([]), provideHttpClient(), provideHttpClientTesting()],
  });
  return { auth: TestBed.inject(AuthService), http: TestBed.inject(HttpTestingController) };
}

describe('AuthService', () => {
  beforeEach(() => localStorage.clear());
  afterEach(() => TestBed.inject(HttpTestingController).verify());

  it('login menyimpan token dan pengguna', () => {
    const { auth, http } = setup();
    expect(auth.isLoggedIn()).toBe(false);
    let got: User | undefined;
    auth.login('b@x.id', 'rahasia123').subscribe((u) => (got = u));
    const req = http.expectOne('/api/auth/login');
    expect(req.request.method).toBe('POST');
    expect(req.request.body).toEqual({ email: 'b@x.id', password: 'rahasia123' });
    req.flush({ token: 'tok', user });
    expect(got).toEqual(user);
    expect(auth.isLoggedIn()).toBe(true);
    expect(auth.token()).toBe('tok');
    expect(localStorage.getItem('gp.token')).toBe('tok');
    expect(auth.isAdmin()).toBe(false);
  });

  it('gagal login tidak menyimpan apa pun', () => {
    const { auth, http } = setup();
    auth.login('b@x.id', 'salah').subscribe({ error: () => undefined });
    http.expectOne('/api/auth/login').flush({ error: 'email atau password salah' }, { status: 401, statusText: 'x' });
    expect(auth.isLoggedIn()).toBe(false);
    expect(localStorage.getItem('gp.token')).toBeNull();
  });

  it('register tidak mengirim kode referral kosong', () => {
    const { auth, http } = setup();
    auth.register({ name: 'B', phone: '0812345678', email: 'b@x.id', password: 'rahasia123', referral_code: '  ' }).subscribe();
    const req = http.expectOne('/api/auth/register');
    expect('referral_code' in req.request.body).toBe(false);
    req.flush({ token: 't', user });
  });

  it('register meneruskan kode referral yang diisi', () => {
    const { auth, http } = setup();
    auth.register({ name: 'B', phone: '0812345678', email: 'b@x.id', password: 'rahasia123', referral_code: 'ABCD2345' }).subscribe();
    const req = http.expectOne('/api/auth/register');
    expect(req.request.body.referral_code).toBe('ABCD2345');
    req.flush({ token: 't', user });
  });

  it('memulihkan token dari penyimpanan dan memuat profil', () => {
    localStorage.setItem('gp.token', 'lama');
    const { auth, http } = setup();
    expect(auth.isLoggedIn()).toBe(true);
    let got: User | null | undefined;
    auth.loadUser().subscribe((u) => (got = u));
    http.expectOne('/api/me').flush({ ...user, role: 'admin' });
    expect(got?.role).toBe('admin');
    expect(auth.isAdmin()).toBe(true);
  });

  it('loadUser tanpa token tidak memanggil server', () => {
    const { auth, http } = setup();
    let got: User | null | undefined;
    auth.loadUser().subscribe((u) => (got = u));
    http.expectNone('/api/me');
    expect(got).toBeNull();
  });

  it('loadUser yang ditolak server mengembalikan null (tanpa melempar)', () => {
    localStorage.setItem('gp.token', 'basi');
    const { auth, http } = setup();
    let got: User | null | undefined = undefined;
    auth.loadUser().subscribe((u) => (got = u));
    http.expectOne('/api/me').flush({}, { status: 401, statusText: 'x' });
    expect(got).toBeNull();
  });

  it('logout menghapus sesi dan mengarahkan ke halaman masuk', () => {
    localStorage.setItem('gp.token', 'tok');
    const { auth } = setup();
    const nav = vi.spyOn(TestBed.inject(Router), 'navigateByUrl').mockResolvedValue(true);
    auth.user.set(user);
    auth.logout();
    expect(auth.isLoggedIn()).toBe(false);
    expect(auth.user()).toBeNull();
    expect(localStorage.getItem('gp.token')).toBeNull();
    expect(nav).toHaveBeenCalledWith('/masuk');
  });

  it('tetap berfungsi bila localStorage melempar error', () => {
    const get = vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new Error('diblokir');
    });
    const set = vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('diblokir');
    });
    const { auth, http } = setup();
    auth.login('b@x.id', 'rahasia123').subscribe();
    http.expectOne('/api/auth/login').flush({ token: 'tok', user });
    expect(auth.isLoggedIn()).toBe(true);
    get.mockRestore();
    set.mockRestore();
  });
});
