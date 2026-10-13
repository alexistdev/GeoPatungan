import { TestBed } from '@angular/core/testing';

import { LayoutService } from './layout';

const html = document.documentElement;
const originalMatchMedia = window.matchMedia;

/** jsdom tidak punya matchMedia; sediakan tiruan yang dipulihkan setelah tiap tes. */
function mockMedia(dark: boolean) {
  Object.defineProperty(window, 'matchMedia', {
    configurable: true,
    writable: true,
    value: () => ({ matches: dark }) as MediaQueryList,
  });
}
afterEach(() => {
  vi.restoreAllMocks();
  Object.defineProperty(window, 'matchMedia', { configurable: true, writable: true, value: originalMatchMedia });
});

function width(px: number) {
  Object.defineProperty(html, 'clientWidth', { value: px, configurable: true });
}

function setup(): LayoutService {
  TestBed.resetTestingModule();
  const svc = TestBed.inject(LayoutService);
  TestBed.tick();
  return svc;
}

describe('LayoutService: tema', () => {
  beforeEach(() => {
    localStorage.clear();
    html.removeAttribute('data-bs-theme');
    html.setAttribute('data-sidebar-size', 'sm-hover');
    document.body.className = '';
  });

  it('tanpa pilihan tersimpan memakai preferensi sistem (gelap)', () => {
    mockMedia(true);
    const svc = setup();
    expect(svc.theme()).toBe('dark');
    expect(html.getAttribute('data-bs-theme')).toBe('dark');
    expect(html.getAttribute('data-topbar')).toBe('dark');
  });

  it('pilihan tersimpan menang atas preferensi sistem', () => {
    localStorage.setItem('gp.theme', 'light');
    mockMedia(true);
    expect(setup().theme()).toBe('light');
  });

  it('tanpa matchMedia (peramban lama) jatuh ke mode terang', () => {
    Object.defineProperty(window, 'matchMedia', { configurable: true, writable: true, value: undefined });
    expect(setup().theme()).toBe('light');
  });

  it('nilai tersimpan yang aneh diabaikan', () => {
    localStorage.setItem('gp.theme', 'ungu');
    mockMedia(false);
    expect(setup().theme()).toBe('light');
  });

  it('toggleTheme mengganti atribut <html> dan menyimpan pilihan', () => {
    localStorage.setItem('gp.theme', 'light');
    const svc = setup();
    expect(html.getAttribute('data-bs-theme')).toBe('light');
    expect(html.getAttribute('data-topbar')).toBe('light');
    svc.toggleTheme();
    TestBed.tick();
    expect(svc.theme()).toBe('dark');
    expect(html.getAttribute('data-bs-theme')).toBe('dark');
    expect(html.getAttribute('data-topbar')).toBe('dark');
    expect(localStorage.getItem('gp.theme')).toBe('dark');
    svc.toggleTheme();
    TestBed.tick();
    expect(html.getAttribute('data-bs-theme')).toBe('light');
    expect(localStorage.getItem('gp.theme')).toBe('light');
  });

  it('tetap berfungsi bila penyimpanan diblokir', () => {
    const get = vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new Error('diblokir');
    });
    const set = vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('diblokir');
    });
    mockMedia(false);
    try {
      const svc = setup();
      expect(() => svc.toggleTheme()).not.toThrow();
      expect(svc.theme()).toBe('dark');
    } finally {
      get.mockRestore();
      set.mockRestore();
    }
  });
});

describe('LayoutService: sidebar', () => {
  beforeEach(() => {
    localStorage.setItem('gp.theme', 'light');
    html.setAttribute('data-sidebar-size', 'sm-hover');
    document.body.className = '';
  });
  afterEach(() => localStorage.clear());

  it('desktop lebar: hamburger bergantian lg <-> sm dan ikon terbuka', () => {
    width(1400);
    const svc = setup();
    svc.toggleSidebar();
    expect(html.getAttribute('data-sidebar-size')).toBe('lg');
    expect(svc.hamburgerOpen()).toBe(true);
    expect(svc.mobileOpen()).toBe(false);
    svc.toggleSidebar();
    expect(html.getAttribute('data-sidebar-size')).toBe('sm');
    expect(svc.hamburgerOpen()).toBe(false);
  });

  it('lebar 600 sudah dianggap ponsel (batas 767): overlay, bukan mengecilkan sidebar', () => {
    width(600);
    const svc = setup();
    svc.toggleSidebar();
    TestBed.tick();
    expect(svc.mobileOpen()).toBe(true);
    expect(html.getAttribute('data-sidebar-size')).toBe('lg');
  });

  it('tepat 767 ponsel, 768 tablet', () => {
    width(767);
    let svc = setup();
    svc.toggleSidebar();
    expect(svc.mobileOpen()).toBe(true);
    width(768);
    html.setAttribute('data-sidebar-size', 'sm-hover');
    svc = setup();
    svc.toggleSidebar();
    expect(svc.mobileOpen()).toBe(false);
    expect(html.getAttribute('data-sidebar-size')).toBe('sm');
  });

  it('tablet: bergantian sm <-> kosong', () => {
    width(900);
    const svc = setup();
    svc.toggleSidebar();
    expect(html.getAttribute('data-sidebar-size')).toBe('sm');
    svc.toggleSidebar();
    expect(html.getAttribute('data-sidebar-size')).toBe('');
  });

  it('ponsel: membuka overlay (kelas pada body) dan melebarkan sidebar; closeMobile menutup', () => {
    width(375);
    const svc = setup();
    svc.toggleSidebar();
    TestBed.tick();
    expect(svc.mobileOpen()).toBe(true);
    expect(document.body.classList.contains('vertical-sidebar-enable')).toBe(true);
    expect(html.getAttribute('data-sidebar-size')).toBe('lg');
    expect(svc.hamburgerOpen()).toBe(false); // ikon hamburger hanya berubah di layar lebar
    svc.closeMobile();
    TestBed.tick();
    expect(document.body.classList.contains('vertical-sidebar-enable')).toBe(false);
  });

  it('desktop menutup overlay ponsel yang masih terbuka', () => {
    width(375);
    const svc = setup();
    svc.toggleSidebar();
    TestBed.tick();
    width(1400);
    svc.toggleSidebar();
    TestBed.tick();
    expect(svc.mobileOpen()).toBe(false);
    expect(document.body.classList.contains('vertical-sidebar-enable')).toBe(false);
  });

  it('togglePin: sm-hover <-> sm-hover-active', () => {
    const svc = setup();
    svc.togglePin();
    expect(html.getAttribute('data-sidebar-size')).toBe('sm-hover-active');
    svc.togglePin();
    expect(html.getAttribute('data-sidebar-size')).toBe('sm-hover');
  });
});
