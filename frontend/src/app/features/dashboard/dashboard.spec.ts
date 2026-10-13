import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';

import { AuthService } from '../../core/auth';
import { Group, User } from '../../core/models';
import { Dashboard } from './dashboard';

const group = (o: Partial<Group>): Group =>
  ({
    id: 'g1', name: 'Arisan Uji', contribution_amount: 50_000, period_type: 'monthly', slot_count: 3, status: 'open',
    active_members: 2, my_status: 'active', ...o,
  }) as Group;

const render = (role: 'admin' | 'member', groups: Group[] | 'error') => {
  TestBed.configureTestingModule({ providers: [provideRouter([]), provideHttpClient(), provideHttpClientTesting()] });
  TestBed.inject(AuthService).user.set({ id: 'u', name: 'Budi', role, kyc_status: 'none', referral_code: 'ABCD2345' } as User);
  const f = TestBed.createComponent(Dashboard);
  f.detectChanges();
  const http = TestBed.inject(HttpTestingController);
  http.expectOne('/api/wallet/').flush({ wallet: { balance: 125_000 } });
  http.expectOne('/api/me/reputation').flush({
    level: { rank: 1, name: 'Pemula', draw_weight: 1 }, stats: { completed_cycles: 0, late_total: 0 }, next_level: null,
  });
  http.expectOne((r) => r.url === '/api/me/contributions').flush({ contributions: [] });
  const g = http.expectOne('/api/groups/');
  if (groups === 'error') g.flush({}, { status: 500, statusText: 'x' });
  else g.flush({ groups });
  f.detectChanges();
  http.verify();
  return f.nativeElement as HTMLElement;
};

describe('Dashboard', () => {
  it('kartu sambutan, saldo, dan grup yang diikuti (yang menunggu tidak dihitung)', () => {
    const el = render('member', [group({}), group({ id: 'g2', name: 'Menunggu', my_status: 'pending' })]);
    expect(el.querySelector('[data-testid=hero]')?.textContent).toContain('Halo, Budi!');
    expect(el.textContent).toContain('Rp125.000');
    expect(el.querySelector('[data-testid=active-groups]')?.textContent?.trim()).toBe('1');
    const cards = el.querySelectorAll('[data-testid=group-card]');
    expect(cards.length).toBe(1);
    expect(cards[0].textContent).toContain('Arisan Uji');
    expect(cards[0].textContent).toContain('2 dari 3 anggota');
  });

  it('pintasan Buat grup hanya untuk admin', () => {
    const link = (el: HTMLElement) => el.querySelector('[data-testid=hero] a[href="/grup/baru"]');
    expect(link(render('admin', []))).not.toBeNull();
    TestBed.resetTestingModule();
    const m = render('member', []);
    expect(link(m)).toBeNull();
    expect(m.querySelector('[data-testid=hero] a[href="/grup/gabung"]')).not.toBeNull();
  });

  it('belum punya grup dan galat memuat grup', () => {
    expect(render('member', []).querySelector('[data-testid=my-groups]')?.textContent).toContain('belum bergabung');
    TestBed.resetTestingModule();
    const el = render('member', 'error');
    expect(el.querySelector('[data-testid=my-groups] [role=alert]')).not.toBeNull();
    expect(el.querySelector('[data-testid=active-groups]')).toBeNull();
  });
});
