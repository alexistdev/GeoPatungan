import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { TestBed } from '@angular/core/testing';
import { Router, provideRouter } from '@angular/router';

import { Register } from './register';

function setup(url = '/daftar') {
  localStorage.clear();
  TestBed.configureTestingModule({
    imports: [Register],
    providers: [provideRouter([{ path: 'daftar', component: Register }]), provideHttpClient(), provideHttpClientTesting()],
  });
  return { ctl: TestBed.inject(HttpTestingController), router: TestBed.inject(Router), url };
}

async function mount(url: string) {
  const s = setup(url);
  await s.router.navigateByUrl(url);
  const fixture = TestBed.createComponent(Register);
  fixture.detectChanges();
  const el = fixture.nativeElement as HTMLElement;
  const type = (name: string, v: string) => {
    const i = el.querySelector<HTMLInputElement>(`input[formControlName=${name}]`)!;
    i.value = v;
    i.dispatchEvent(new Event('input'));
  };
  const submit = () => {
    el.querySelector('form')!.dispatchEvent(new Event('submit'));
    fixture.detectChanges();
  };
  const nav = vi.spyOn(s.router, 'navigateByUrl').mockResolvedValue(true);
  return { ...s, fixture, el, type, submit, nav };
}

const fill = (type: (n: string, v: string) => void) => {
  type('name', 'Budi Santoso');
  type('phone', '081234567890');
  type('email', 'budi@example.com');
  type('password', 'rahasia123');
};

describe('Register', () => {
  afterEach(() => TestBed.inject(HttpTestingController).verify());

  it('kode referral terisi dari ?ref=', async () => {
    const { el } = await mount('/daftar?ref=ABCD2345');
    expect(el.querySelector<HTMLInputElement>('input[formControlName=referral_code]')!.value).toBe('ABCD2345');
  });

  it('validasi klien: telepon, kata sandi pendek', async () => {
    const { type, submit, ctl, el } = await mount('/daftar');
    fill(type);
    type('phone', '12ab');
    type('password', 'pendek');
    submit();
    ctl.expectNone('/api/auth/register');
    expect(el.textContent).toContain('9–15 digit');
    expect(el.textContent).toContain('8 sampai 72');
  });

  it('mengirim data bersih, kode referral dikapitalkan, lalu ke beranda', async () => {
    const { type, submit, ctl, nav } = await mount('/daftar');
    fill(type);
    type('referral_code', ' abcd2345 ');
    submit();
    const req = ctl.expectOne('/api/auth/register');
    expect(req.request.body).toEqual({
      name: 'Budi Santoso', phone: '081234567890', email: 'budi@example.com',
      password: 'rahasia123', referral_code: 'ABCD2345',
    });
    req.flush({ token: 't', user: { id: 'u' } });
    expect(nav).toHaveBeenCalledWith('/');
  });

  it('kode referral kosong tidak dikirim', async () => {
    const { type, submit, ctl } = await mount('/daftar');
    fill(type);
    submit();
    const req = ctl.expectOne('/api/auth/register');
    expect('referral_code' in req.request.body).toBe(false);
    req.flush({ token: 't', user: { id: 'u' } });
  });

  it('kode referral salah -> pesan server tampil', async () => {
    const { fixture, el, type, submit, ctl, nav } = await mount('/daftar');
    fill(type);
    type('referral_code', 'SALAHKODE');
    submit();
    ctl.expectOne('/api/auth/register').flush({ error: 'kode referral tidak ditemukan' }, { status: 400, statusText: 'x' });
    fixture.detectChanges();
    expect(el.querySelector('[role=alert]')?.textContent).toContain('kode referral tidak ditemukan');
    expect(nav).not.toHaveBeenCalled();
  });
});
