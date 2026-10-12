import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { Router, provideRouter } from '@angular/router';

import { Login } from './login';

function setup() {
  localStorage.clear();
  TestBed.configureTestingModule({
    imports: [Login],
    providers: [provideRouter([]), provideHttpClient(), provideHttpClientTesting()],
  });
  const fixture: ComponentFixture<Login> = TestBed.createComponent(Login);
  fixture.detectChanges();
  const el = fixture.nativeElement as HTMLElement;
  const nav = vi.spyOn(TestBed.inject(Router), 'navigateByUrl').mockResolvedValue(true);
  const type = (sel: string, v: string) => {
    const i = el.querySelector<HTMLInputElement>(sel)!;
    i.value = v;
    i.dispatchEvent(new Event('input'));
  };
  const submit = () => {
    el.querySelector('form')!.dispatchEvent(new Event('submit'));
    fixture.detectChanges();
  };
  return { fixture, el, nav, type, submit, ctl: TestBed.inject(HttpTestingController) };
}

describe('Login', () => {
  afterEach(() => TestBed.inject(HttpTestingController).verify());

  it('form kosong tidak memanggil server dan menampilkan pesan validasi', () => {
    const { el, submit, ctl } = setup();
    submit();
    ctl.expectNone('/api/auth/login');
    expect(el.textContent).toContain('Masukkan email yang valid');
    expect(el.textContent).toContain('Kata sandi wajib diisi');
  });

  it('email tidak valid ditolak di klien', () => {
    const { type, submit, ctl } = setup();
    type('input[type=email]', 'bukan-email');
    type('input[type=password]', 'x');
    submit();
    ctl.expectNone('/api/auth/login');
  });

  it('berhasil masuk -> ke beranda', () => {
    const { type, submit, ctl, nav } = setup();
    type('input[type=email]', '  b@x.id ');
    type('input[type=password]', 'rahasia123');
    submit();
    const req = ctl.expectOne('/api/auth/login');
    expect(req.request.body).toEqual({ email: 'b@x.id', password: 'rahasia123' });
    req.flush({ token: 't', user: { id: 'u', name: 'B', role: 'member' } });
    expect(nav).toHaveBeenCalledWith('/');
  });

  it('gagal -> pesan server tampil, tombol aktif lagi, tidak berpindah halaman', () => {
    const { fixture, el, type, submit, ctl, nav } = setup();
    type('input[type=email]', 'b@x.id');
    type('input[type=password]', 'salah');
    submit();
    expect(el.querySelector<HTMLButtonElement>('button[type=submit]')!.disabled).toBe(true);
    ctl.expectOne('/api/auth/login').flush({ error: 'email atau password salah' }, { status: 401, statusText: 'x' });
    fixture.detectChanges();
    expect(el.querySelector('[role=alert]')?.textContent).toContain('email atau password salah');
    expect(el.querySelector<HTMLButtonElement>('button[type=submit]')!.disabled).toBe(false);
    expect(nav).not.toHaveBeenCalled();
  });

  it('klik ganda tidak mengirim dua permintaan', () => {
    const { type, submit, ctl } = setup();
    type('input[type=email]', 'b@x.id');
    type('input[type=password]', 'rahasia123');
    submit();
    submit();
    ctl.expectOne('/api/auth/login');
  });
});
