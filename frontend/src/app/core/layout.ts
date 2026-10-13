import { Injectable, effect, signal } from '@angular/core';

import { safeStorage } from './storage';

const THEME_KEY = 'gp.theme';
export type Theme = 'light' | 'dark';

/**
 * Perilaku kerangka halaman Velzon (demo "modern": sidebar vertikal gelap yang mengembang saat disorot).
 * Tema Velzon dikendalikan lewat atribut data-* pada <html>; di sini atribut itu diatur oleh Angular
 * (skrip bawaan tema tidak dipakai karena DOM dirender dinamis).
 */
@Injectable({ providedIn: 'root' })
export class LayoutService {
  private readonly html = document.documentElement;

  readonly theme = signal<Theme>(this.initialTheme());
  /** Sidebar terbuka di layar kecil (overlay). */
  readonly mobileOpen = signal(false);
  /** Ikon hamburger "terbuka" (hanya dipakai di layar lebar). */
  readonly hamburgerOpen = signal(false);

  constructor() {
    effect(() => {
      const t = this.theme();
      this.html.setAttribute('data-bs-theme', t);
      this.html.setAttribute('data-topbar', t === 'dark' ? 'dark' : 'light');
    });
    effect(() => {
      document.body.classList.toggle('vertical-sidebar-enable', this.mobileOpen());
    });
  }

  private initialTheme(): Theme {
    const saved = safeStorage.get(THEME_KEY);
    if (saved === 'light' || saved === 'dark') return saved;
    try {
      return window.matchMedia?.('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
    } catch {
      return 'light';
    }
  }

  toggleTheme(): void {
    const next: Theme = this.theme() === 'dark' ? 'light' : 'dark';
    this.theme.set(next);
    safeStorage.set(THEME_KEY, next);
  }

  /** Meniru perilaku tombol hamburger Velzon: mengecilkan/melebarkan sidebar, atau membuka overlay di ponsel. */
  toggleSidebar(): void {
    const w = document.documentElement.clientWidth || window.innerWidth;
    if (w > 767) this.hamburgerOpen.update((v) => !v);

    if (w <= 767) {
      this.mobileOpen.set(true);
      this.html.setAttribute('data-sidebar-size', 'lg');
    } else if (w <= 1025) {
      this.mobileOpen.set(false);
      this.html.setAttribute('data-sidebar-size', this.html.getAttribute('data-sidebar-size') === 'sm' ? '' : 'sm');
    } else {
      this.mobileOpen.set(false);
      this.html.setAttribute('data-sidebar-size', this.html.getAttribute('data-sidebar-size') === 'lg' ? 'sm' : 'lg');
    }
  }

  closeMobile(): void {
    this.mobileOpen.set(false);
  }

  /** Tombol "pin" di sidebar: menahan sidebar tetap terbuka pada mode sm-hover. */
  togglePin(): void {
    const cur = this.html.getAttribute('data-sidebar-size');
    this.html.setAttribute('data-sidebar-size', cur === 'sm-hover' ? 'sm-hover-active' : 'sm-hover');
  }
}
