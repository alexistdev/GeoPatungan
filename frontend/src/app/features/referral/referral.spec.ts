import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';

import { CommissionEntry, ReferralSummary } from '../../core/models';
import { Referral } from './referral';

const summary = (o: Partial<ReferralSummary> = {}): ReferralSummary => ({
  referral_code: 'QUF6YSMA', eligible: true,
  rules: [{ level: 1, share_bps: 4000 }, { level: 2, share_bps: 2500 }, { level: 3, share_bps: 1500 }, { level: 4, share_bps: 1000 }],
  downline: { level_1: 3, level_2: 2, level_3: 0, level_4: 1, total: 6 },
  earned: { total: 1300, by_level: { '1': 800, '2': 500, '3': 0, '4': 0 } }, ...o,
});
const entry = (id: number, o: Partial<CommissionEntry> = {}): CommissionEntry => ({
  id, level: 1, share_bps: 4000, amount: 800, source_name: 'B*** S***', created_at: '2026-10-13T10:00:00+07:00', ...o,
});
const page = (items: CommissionEntry[], next: number | null) => ({ commissions: items, next_before: next });

function mount(s: ReferralSummary, p = page([], null)) {
  TestBed.configureTestingModule({ providers: [provideRouter([]), provideHttpClient(), provideHttpClientTesting()] });
  const f = TestBed.createComponent(Referral);
  f.detectChanges();
  const ctl = TestBed.inject(HttpTestingController);
  ctl.expectOne('/api/me/referral/').flush(s);
  ctl.expectOne((r) => r.url === '/api/me/referral/commissions' && r.params.get('limit') === '20').flush(p);
  f.detectChanges();
  return { f, ctl, el: f.nativeElement as HTMLElement };
}

describe('Referral', () => {
  afterEach(() => TestBed.inject(HttpTestingController).verify());

  it('menampilkan kode, tautan pendaftaran dengan ?ref=, dan tabel per level', () => {
    const { el } = mount(summary());
    expect(el.textContent).toContain('QUF6YSMA');
    expect(el.querySelector<HTMLInputElement>('input.link')!.value).toBe(`${location.origin}/daftar?ref=QUF6YSMA`);
    const rows = Array.from(el.querySelectorAll('tbody tr')).map((r) => r.textContent!.replace(/\s+/g, ' ').trim());
    expect(rows[0]).toContain('Level 1 (langsung)');
    expect(rows[0]).toContain('40%');
    expect(rows[0]).toContain('3');
    expect(rows[0]).toContain('Rp800');
    expect(rows[3]).toContain('10%');
    expect(rows[3]).toContain('1');
    expect(el.querySelector('tfoot')!.textContent).toContain('Rp1.300');
    expect(el.querySelector('tfoot')!.textContent).toContain('6');
    expect(el.textContent).toContain('Total semua level 90%');
  });

  it('akun belum terverifikasi: peringatan komisi tertahan dengan tautan verifikasi', () => {
    const { el } = mount(summary({ eligible: false }));
    expect(el.textContent).toContain('sudah terverifikasi');
    expect(el.querySelector('a[href="/profil"]')).toBeTruthy();
  });

  it('akun terverifikasi: tanpa peringatan', () => {
    const { el } = mount(summary({ eligible: true }));
    expect(el.querySelector('.alert.warn')).toBeNull();
  });

  it('bagian desimal tampil benar (12,5%)', () => {
    const { el } = mount(summary({ rules: [{ level: 1, share_bps: 1250 }, { level: 2, share_bps: 0 }, { level: 3, share_bps: 0 }, { level: 4, share_bps: 0 }] }));
    expect(el.querySelector('tbody tr')!.textContent).toContain('12.5%');
  });

  it('riwayat kosong menjelaskan kapan komisi muncul', () => {
    const { el } = mount(summary());
    expect(el.textContent).toContain('Belum ada komisi');
  });

  it('riwayat: nama tersamarkan, level, bagian, dan jumlah; paging memakai kursor tanpa ganda', () => {
    const { f, ctl, el } = mount(summary(), page([entry(3), entry(2, { level: 2, share_bps: 2500, amount: 500 })], 2));
    expect(el.textContent).toContain('B*** S***');
    expect(el.textContent).toContain('+Rp800');
    expect(el.textContent).toContain('level 2');
    expect(el.textContent).toContain('25% dari biaya platform');
    Array.from(el.querySelectorAll('button')).find((b) => b.textContent?.includes('Muat lebih banyak'))!.click();
    ctl.expectOne((r) => r.url === '/api/me/referral/commissions' && r.params.get('before') === '2').flush(page([entry(1)], null));
    f.detectChanges();
    expect(el.querySelectorAll('ul.list li').length).toBe(3);
    expect(el.textContent).not.toContain('Muat lebih banyak');
  });

  it('galat server tampil sebagai pesan', () => {
    TestBed.configureTestingModule({ providers: [provideRouter([]), provideHttpClient(), provideHttpClientTesting()] });
    const f = TestBed.createComponent(Referral);
    f.detectChanges();
    const ctl = TestBed.inject(HttpTestingController);
    ctl.expectOne('/api/me/referral/').flush({}, { status: 500, statusText: 'x' });
    ctl.expectOne((r) => r.url === '/api/me/referral/commissions').flush(page([], null));
    f.detectChanges();
    expect((f.nativeElement as HTMLElement).querySelector('[role=alert]')?.textContent).toContain('server');
  });
});
