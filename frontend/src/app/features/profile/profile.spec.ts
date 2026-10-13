import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';

import { AuthService } from '../../core/auth';
import { KycRequest, KycState, ReputationLevel, ReputationProfile, User } from '../../core/models';
import { Profile } from './profile';

const level = (o: Partial<ReputationLevel> = {}): ReputationLevel => ({
  rank: 1, code: 'baru', name: 'Baru', require_verified: false, min_completed_cycles: 0, max_late_count: null, draw_weight: 1,
  can_create_group: true, max_group_value: 500_000, ...o,
});
const rep = (o: Partial<ReputationProfile> = {}): ReputationProfile => ({
  level: level(),
  stats: { kyc_status: 'none', verified: false, completed_cycles: 2, on_time_payments: 5, late_payments: 1, overdue_unpaid: 0, late_total: 1 },
  next_level: {
    level: level({ rank: 2, code: 'terpercaya', name: 'Terpercaya', draw_weight: 2, max_group_value: 5_000_000 }),
    requirements: [
      { key: 'verified', label: 'Akun terverifikasi (KYC)', met: false, current: 0, required: 1 },
      { key: 'completed_cycles', label: 'Menuntaskan 3 arisan', met: false, current: 2, required: 3 },
      { key: 'late', label: 'Tanpa keterlambatan', met: false, current: 1, required: 0 },
    ],
  },
  ...o,
});
const req = (o: Partial<KycRequest> = {}): KycRequest => ({
  id: 'k1', user_id: 'u', status: 'pending', note: '', review_note: '', submitted_at: '2026-10-13T10:00:00+07:00', reviewed_at: null, ...o,
});
const user = { id: 'u', name: 'Andi', email: 'a@x.id', phone: '0812345678', created_at: '2026-10-01T00:00:00Z' } as User;

function mount(kyc: KycState, r: ReputationProfile = rep()) {
  localStorage.setItem('gp.token', 'tok');
  TestBed.configureTestingModule({ providers: [provideRouter([]), provideHttpClient(), provideHttpClientTesting()] });
  TestBed.inject(AuthService).user.set(user);
  const f = TestBed.createComponent(Profile);
  f.detectChanges();
  const ctl = TestBed.inject(HttpTestingController);
  ctl.expectOne('/api/me/reputation').flush(r);
  ctl.expectOne('/api/me/kyc/').flush(kyc);
  f.detectChanges();
  return { f, ctl, el: f.nativeElement as HTMLElement };
}
const button = (el: HTMLElement, text: string) =>
  Array.from(el.querySelectorAll('button')).find((b) => b.textContent?.includes(text)) as HTMLButtonElement | undefined;

describe('Profile', () => {
  afterEach(() => localStorage.clear());
  afterEach(() => TestBed.inject(HttpTestingController).verify());

  it('menampilkan akun, hak level, dan catatan', () => {
    const { el } = mount({ kyc_status: 'none', latest_request: null });
    expect(el.textContent).toContain('a@x.id');
    expect(el.textContent).toContain('×1');
    expect(el.textContent).toContain('Rp500.000');
    expect(el.textContent).toContain('Boleh');
    const nums = Array.from(el.querySelectorAll('.stats .big')).map((n) => n.textContent!.trim());
    expect(nums).toEqual(['2', '5', '1', '0']);
  });

  it('nilai grup tanpa batas tampil sebagai "Tanpa batas"', () => {
    const { el } = mount({ kyc_status: 'none', latest_request: null }, rep({ level: level({ max_group_value: null }) }));
    expect(el.textContent).toContain('Tanpa batas');
  });

  it('syarat level berikutnya: progres dihitung, syarat keterlambatan tanpa bar', () => {
    const { el } = mount({ kyc_status: 'none', latest_request: null });
    expect(el.textContent).toContain('Menuju level 2: Terpercaya');
    const bars = Array.from(el.querySelectorAll('.progress-bar')).map((p) => p.getAttribute('data-percent'));
    expect(bars).toEqual(['0', '67']);
    expect(el.textContent).toContain('2 dari 3');
    expect(el.textContent).toContain('Keterlambatan Anda 1 (batas 0)');
  });

  it('level tertinggi: tanpa daftar syarat', () => {
    const { el } = mount({ kyc_status: 'verified', latest_request: null }, rep({ next_level: null }));
    expect(el.textContent).toContain('level tertinggi');
    expect(el.querySelector('.progress-bar')).toBeNull();
  });

  it('belum terverifikasi: ajukan, tampilkan form, kirim catatan bersih, status berubah menunggu', () => {
    const { f, ctl, el } = mount({ kyc_status: 'none', latest_request: null });
    expect(el.textContent).toContain('Belum terverifikasi');
    button(el, 'Ajukan verifikasi')!.click();
    f.detectChanges();
    const input = el.querySelector<HTMLInputElement>('input[formControlName=note]')!;
    input.value = '  kirim via WA  ';
    input.dispatchEvent(new Event('input'));
    el.querySelector('form')!.dispatchEvent(new Event('submit'));
    const post = ctl.expectOne((r) => r.method === 'POST' && r.url === '/api/me/kyc/');
    expect(post.request.body).toEqual({ note: 'kirim via WA' });
    post.flush({ request: req({ note: 'kirim via WA' }) }, { status: 201, statusText: 'Created' });
    ctl.expectOne('/api/me').flush({ ...user, kyc_status: 'pending' });
    ctl.expectOne('/api/me/reputation').flush(rep());
    f.detectChanges();
    expect(el.textContent).toContain('Menunggu verifikasi');
    expect(button(el, 'Ajukan verifikasi')).toBeUndefined();
  });

  it('catatan kosong tetap boleh diajukan', () => {
    const { f, ctl, el } = mount({ kyc_status: 'none', latest_request: null });
    button(el, 'Ajukan verifikasi')!.click();
    f.detectChanges();
    el.querySelector('form')!.dispatchEvent(new Event('submit'));
    const post = ctl.expectOne((r) => r.method === 'POST');
    expect(post.request.body).toEqual({ note: '' });
    post.flush({ request: req() }, { status: 201, statusText: 'Created' });
    ctl.expectOne('/api/me').flush(user);
    ctl.expectOne('/api/me/reputation').flush(rep());
  });

  it('catatan lebih dari 200 karakter ditolak di klien', () => {
    const { f, ctl, el } = mount({ kyc_status: 'none', latest_request: null });
    button(el, 'Ajukan verifikasi')!.click();
    f.detectChanges();
    const input = el.querySelector<HTMLInputElement>('input[formControlName=note]')!;
    input.value = 'x'.repeat(201);
    input.dispatchEvent(new Event('input'));
    el.querySelector('form')!.dispatchEvent(new Event('submit'));
    f.detectChanges();
    ctl.expectNone((r) => r.method === 'POST');
    expect(el.textContent).toContain('Maksimal 200 karakter');
  });

  it('klik ganda hanya mengirim satu pengajuan', () => {
    const { f, ctl, el } = mount({ kyc_status: 'none', latest_request: null });
    button(el, 'Ajukan verifikasi')!.click();
    f.detectChanges();
    const form = el.querySelector('form')!;
    form.dispatchEvent(new Event('submit'));
    form.dispatchEvent(new Event('submit'));
    const post = ctl.expectOne((r) => r.method === 'POST');
    post.flush({ request: req() }, { status: 201, statusText: 'Created' });
    ctl.expectOne('/api/me').flush(user);
    ctl.expectOne('/api/me/reputation').flush(rep());
  });

  it('galat server (sudah diproses) tampil dan form tetap terbuka', () => {
    const { f, ctl, el } = mount({ kyc_status: 'none', latest_request: null });
    button(el, 'Ajukan verifikasi')!.click();
    f.detectChanges();
    el.querySelector('form')!.dispatchEvent(new Event('submit'));
    ctl.expectOne((r) => r.method === 'POST').flush({ error: 'permintaan verifikasi Anda sedang diproses' }, { status: 409, statusText: 'x' });
    f.detectChanges();
    expect(el.querySelector('[role=alert]')?.textContent).toContain('sedang diproses');
    expect(el.querySelector('form')).toBeTruthy();
  });

  it('ditolak: alasan tampil dan bisa diajukan ulang', () => {
    const { el } = mount({ kyc_status: 'rejected', latest_request: req({ status: 'rejected', review_note: 'foto buram' }) });
    expect(el.textContent).toContain('Ditolak');
    expect(el.textContent).toContain('foto buram');
    expect(button(el, 'Ajukan ulang')).toBeTruthy();
  });

  it('menunggu dan terverifikasi: tidak ada tombol ajukan', () => {
    const a = mount({ kyc_status: 'pending', latest_request: req() });
    expect(a.el.textContent).toContain('sedang diperiksa admin');
    expect(button(a.el, 'Ajukan')).toBeUndefined();
    TestBed.resetTestingModule();
    const b = mount({ kyc_status: 'verified', latest_request: req({ status: 'verified' }) });
    expect(b.el.textContent).toContain('Terverifikasi');
    expect(button(b.el, 'Ajukan')).toBeUndefined();
  });

  it('galat memuat reputasi tidak menghilangkan bagian KYC', () => {
    TestBed.configureTestingModule({ providers: [provideRouter([]), provideHttpClient(), provideHttpClientTesting()] });
    const f = TestBed.createComponent(Profile);
    f.detectChanges();
    const ctl = TestBed.inject(HttpTestingController);
    ctl.expectOne('/api/me/reputation').flush({}, { status: 500, statusText: 'x' });
    ctl.expectOne('/api/me/kyc/').flush({ kyc_status: 'none', latest_request: null });
    f.detectChanges();
    const el = f.nativeElement as HTMLElement;
    expect(el.querySelector('[role=alert]')?.textContent).toContain('server');
    expect(button(el, 'Ajukan verifikasi')).toBeTruthy();
  });
});
