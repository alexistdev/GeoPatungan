import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { TestBed } from '@angular/core/testing';
import { Router, provideRouter } from '@angular/router';

import { AuthService } from '../core/auth';
import { LayoutService } from '../core/layout';
import { User } from '../core/models';
import { Shell } from './shell';

function mount(user: Partial<User>, unread = 0) {
  localStorage.setItem('gp.theme', 'light');
  localStorage.setItem('gp.token', 'tok');
  TestBed.configureTestingModule({
    providers: [provideRouter([{ path: '**', children: [] }]), provideHttpClient(), provideHttpClientTesting()],
  });
  TestBed.inject(AuthService).user.set({ id: 'u', name: 'Budi Santoso', role: 'member', ...user } as User);
  const f = TestBed.createComponent(Shell);
  f.detectChanges();
  const ctl = TestBed.inject(HttpTestingController);
  ctl.expectOne('/api/me/notifications/unread-count').flush({ unread_count: unread });
  f.detectChanges();
  return { f, el: f.nativeElement as HTMLElement, ctl };
}
const sidebarTexts = (el: HTMLElement) =>
  Array.from(el.querySelectorAll('.app-menu .nav-link')).map((a) => a.textContent!.replace(/\s+/g, ' ').trim());

describe('Shell (kerangka Velzon)', () => {
  afterEach(() => {
    TestBed.inject(HttpTestingController).verify();
    localStorage.clear();
    document.body.className = '';
    Object.defineProperty(document.documentElement, 'clientWidth', { value: 0, configurable: true });
  });

  it('anggota: menu utama lengkap dan TANPA menu admin', () => {
    const { el } = mount({});
    const t = sidebarTexts(el);
    for (const label of ['Beranda', 'Grup arisan', 'Dompet', 'Referral', 'Profil', 'Notifikasi']) {
      expect(t.some((x) => x.startsWith(label))).toBe(true);
    }
    expect(el.querySelector('#sidebarAdmin')).toBeNull();
    expect(el.textContent).not.toContain('Administrasi');
    expect(el.querySelector('.user-name-sub')!.textContent).toContain('Anggota');
  });

  it('admin: submenu admin dengan semua halaman', () => {
    const { el } = mount({ role: 'admin' });
    const t = sidebarTexts(el);
    for (const label of ['Administrasi', 'Ringkasan', 'Top-up', 'Penarikan', 'Verifikasi KYC', 'Pengaturan', 'Komisi referral', 'Level reputasi']) {
      expect(t.some((x) => x.startsWith(label))).toBe(true);
    }
    const hrefs = Array.from(el.querySelectorAll('#sidebarAdmin a')).map((a) => a.getAttribute('href'));
    expect(hrefs).toEqual(['/admin', '/admin/topup', '/admin/penarikan', '/admin/kyc', '/admin/pengaturan', '/admin/komisi', '/admin/reputasi']);
    expect(el.querySelector('.user-name-sub')!.textContent).toContain('Admin');
  });

  it('submenu admin menutup di halaman biasa dan terbuka otomatis saat masuk ke /admin', async () => {
    const { f, el } = mount({ role: 'admin' });
    const sub = el.querySelector('#sidebarAdmin')!;
    expect(sub.classList.contains('show')).toBe(false);
    await TestBed.inject(Router).navigateByUrl('/admin/topup');
    f.detectChanges();
    expect(sub.classList.contains('show')).toBe(true);
    expect(el.querySelector('.menu-link[href="#sidebarAdmin"]')!.getAttribute('aria-expanded')).toBe('true');
  });

  it('lencana notifikasi hanya muncul bila ada yang belum dibaca; batas 99+', () => {
    expect(mount({}, 0).el.querySelector('.topbar-badge')).toBeNull();
    TestBed.resetTestingModule();
    const a = mount({}, 5);
    expect(a.el.querySelector('.topbar-badge')!.textContent!.trim()).toBe('5');
    expect(a.el.querySelector('.menu-link .badge')!.textContent!.trim()).toBe('5');
    TestBed.resetTestingModule();
    expect(mount({}, 250).el.querySelector('.topbar-badge')!.textContent!.trim()).toBe('99+');
  });

  it('inisial pengguna di avatar (dua kata -> dua huruf; satu kata -> satu huruf)', () => {
    expect(mount({ name: 'Budi Santoso' }).el.querySelector('.avatar-title')!.textContent!.trim()).toBe('BS');
    TestBed.resetTestingModule();
    expect(mount({ name: 'andi' }).el.querySelector('.avatar-title')!.textContent!.trim()).toBe('A');
  });

  it('tombol tema mengganti mode dan label aksesibilitasnya', () => {
    const { f, el } = mount({});
    const btn = el.querySelector<HTMLButtonElement>('.light-dark-mode')!;
    expect(btn.getAttribute('aria-label')).toContain('gelap');
    btn.click();
    TestBed.tick();
    f.detectChanges();
    expect(document.documentElement.getAttribute('data-bs-theme')).toBe('dark');
    expect(btn.getAttribute('aria-label')).toContain('terang');
  });

  it('keluar: menghapus sesi dan menuju halaman masuk', () => {
    const { el } = mount({});
    const nav = vi.spyOn(TestBed.inject(Router), 'navigateByUrl').mockResolvedValue(true);
    const logout = Array.from(el.querySelectorAll('.dropdown-menu button')).find((b) => b.textContent!.includes('Keluar')) as HTMLButtonElement;
    logout.click();
    expect(TestBed.inject(AuthService).isLoggedIn()).toBe(false);
    expect(nav).toHaveBeenCalledWith('/masuk');
  });

  it('overlay sidebar menutup menu ponsel', () => {
    Object.defineProperty(document.documentElement, 'clientWidth', { value: 375, configurable: true });
    const { el } = mount({});
    TestBed.inject(LayoutService).toggleSidebar(); // buka di ponsel
    TestBed.tick();
    expect(document.body.classList.contains('vertical-sidebar-enable')).toBe(true);
    el.querySelector<HTMLElement>('.vertical-overlay')!.click();
    TestBed.tick();
    expect(document.body.classList.contains('vertical-sidebar-enable')).toBe(false);
  });

  it('berpindah halaman menutup sidebar ponsel', async () => {
    Object.defineProperty(document.documentElement, 'clientWidth', { value: 375, configurable: true });
    mount({});
    TestBed.inject(LayoutService).toggleSidebar();
    TestBed.tick();
    expect(document.body.classList.contains('vertical-sidebar-enable')).toBe(true);
    await TestBed.inject(Router).navigateByUrl('/grup');
    TestBed.tick();
    expect(document.body.classList.contains('vertical-sidebar-enable')).toBe(false);
  });
});
