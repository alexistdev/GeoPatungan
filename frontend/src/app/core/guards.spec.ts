import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { TestBed } from '@angular/core/testing';
import { ActivatedRouteSnapshot, CanActivateFn, RouterStateSnapshot, Router, UrlTree, provideRouter } from '@angular/router';
import { Observable, isObservable, firstValueFrom } from 'rxjs';

import { AuthService } from './auth';
import { adminGuard, authGuard, guestGuard } from './guards';
import { User } from './models';

const user = { id: 'u', name: 'B', role: 'member' } as User;

function run(guard: CanActivateFn) {
  return TestBed.runInInjectionContext(() => guard({} as ActivatedRouteSnapshot, {} as RouterStateSnapshot));
}

function setup(token: string | null) {
  localStorage.clear();
  if (token) localStorage.setItem('gp.token', token);
  TestBed.configureTestingModule({ providers: [provideRouter([]), provideHttpClient(), provideHttpClientTesting()] });
  return { auth: TestBed.inject(AuthService), ctl: TestBed.inject(HttpTestingController), router: TestBed.inject(Router) };
}

const path = (r: unknown) => (r as UrlTree).toString();

describe('authGuard', () => {
  it('tanpa token -> /masuk', () => {
    const { router } = setup(null);
    expect(path(run(authGuard))).toBe(router.parseUrl('/masuk').toString());
  });

  it('login dan profil sudah ada -> lolos tanpa panggilan jaringan', () => {
    const { auth, ctl } = setup('tok');
    auth.user.set(user);
    expect(run(authGuard)).toBe(true);
    ctl.expectNone('/api/me');
  });

  it('token ada tapi profil belum dimuat -> memuat lalu lolos', async () => {
    const { ctl } = setup('tok');
    const res = run(authGuard);
    expect(isObservable(res)).toBe(true);
    const p = firstValueFrom(res as Observable<boolean | UrlTree>);
    ctl.expectOne('/api/me').flush(user);
    expect(await p).toBe(true);
  });

  it('token ditolak server -> sesi dibersihkan dan ke /masuk', async () => {
    const { auth, ctl, router } = setup('basi');
    const p = firstValueFrom(run(authGuard) as Observable<boolean | UrlTree>);
    ctl.expectOne('/api/me').flush({}, { status: 401, statusText: 'x' });
    expect(path(await p)).toBe(router.parseUrl('/masuk').toString());
    expect(auth.isLoggedIn()).toBe(false);
  });
});

describe('adminGuard', () => {
  it('member ditolak ke beranda, admin lolos', () => {
    const { auth, router } = setup('tok');
    auth.user.set(user);
    expect(path(run(adminGuard))).toBe(router.parseUrl('/').toString());
    auth.user.set({ ...user, role: 'admin' });
    expect(run(adminGuard)).toBe(true);
  });
});

describe('guestGuard', () => {
  it('yang sudah login diarahkan ke beranda', () => {
    const { router } = setup('tok');
    expect(path(run(guestGuard))).toBe(router.parseUrl('/').toString());
  });
  it('tamu lolos', () => {
    setup(null);
    expect(run(guestGuard)).toBe(true);
  });
});
