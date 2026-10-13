import { Component, OnDestroy, OnInit, computed, inject } from '@angular/core';
import { NavigationEnd, Router, RouterLink, RouterLinkActive, RouterOutlet } from '@angular/router';
import { filter } from 'rxjs';

import { AuthService } from '../core/auth';
import { LayoutService } from '../core/layout';
import { NotificationService } from '../core/notifications';

interface MenuItem {
  path: string;
  label: string;
  icon: string;
  exact?: boolean;
}

/** Kerangka halaman Velzon (demo modern): topbar, sidebar vertikal gelap, konten, dan footer. */
@Component({
  selector: 'app-shell',
  imports: [RouterOutlet, RouterLink, RouterLinkActive],
  templateUrl: './shell.html',
})
export class Shell implements OnInit, OnDestroy {
  protected readonly auth = inject(AuthService);
  protected readonly notifications = inject(NotificationService);
  protected readonly layout = inject(LayoutService);
  private readonly router = inject(Router);

  protected readonly year = new Date().getFullYear();

  protected readonly menu: MenuItem[] = [
    { path: '/', label: 'Beranda', icon: 'ri-dashboard-2-line', exact: true },
    { path: '/grup', label: 'Grup arisan', icon: 'ri-group-line' },
    { path: '/dompet', label: 'Dompet', icon: 'ri-wallet-3-line' },
    { path: '/referral', label: 'Referral', icon: 'ri-share-forward-line' },
    { path: '/profil', label: 'Profil', icon: 'ri-user-settings-line' },
    { path: '/notifikasi', label: 'Notifikasi', icon: 'ri-notification-3-line' },
  ];

  protected readonly adminMenu: MenuItem[] = [
    { path: '/admin', label: 'Ringkasan', icon: 'ri-shield-check-line', exact: true },
    { path: '/admin/topup', label: 'Top-up', icon: 'ri-bank-card-line' },
    { path: '/admin/penarikan', label: 'Penarikan', icon: 'ri-money-dollar-circle-line' },
    { path: '/admin/kyc', label: 'Verifikasi KYC', icon: 'ri-shield-user-line' },
    { path: '/admin/pengaturan', label: 'Pengaturan', icon: 'ri-settings-3-line' },
    { path: '/admin/komisi', label: 'Komisi referral', icon: 'ri-percent-line' },
    { path: '/admin/reputasi', label: 'Level reputasi', icon: 'ri-medal-line' },
  ];

  protected readonly initials = computed(() => {
    const name = this.auth.user()?.name?.trim() ?? '';
    const parts = name.split(/\s+/).filter(Boolean);
    return ((parts[0]?.[0] ?? '?') + (parts[1]?.[0] ?? '')).toUpperCase();
  });

  /** Submenu admin terbuka bila halaman saat ini berada di bawah /admin. */
  protected adminOpen = this.router.url.startsWith('/admin');

  private sub = this.router.events.pipe(filter((e) => e instanceof NavigationEnd)).subscribe((e) => {
    this.layout.closeMobile(); // overlay sidebar menutup setelah berpindah halaman
    if ((e as NavigationEnd).urlAfterRedirects.startsWith('/admin')) this.adminOpen = true;
  });

  ngOnInit(): void {
    this.notifications.startPolling();
  }

  ngOnDestroy(): void {
    this.notifications.stopPolling();
    this.sub.unsubscribe();
  }

  protected logout(): void {
    this.auth.logout();
  }
}
