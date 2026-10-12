import { Component } from '@angular/core';
import { RouterLink, RouterLinkActive, RouterOutlet } from '@angular/router';

@Component({
  selector: 'app-admin-layout',
  imports: [RouterOutlet, RouterLink, RouterLinkActive],
  template: `
    <div class="page">
      <h1>Admin</h1>
      <nav class="tabs" aria-label="Menu admin">
        @for (t of tabs; track t.path) {
          <a [routerLink]="t.path" routerLinkActive="active" [routerLinkActiveOptions]="{ exact: t.exact ?? false }">{{ t.label }}</a>
        }
      </nav>
      <router-outlet />
    </div>
  `,
  styles: `
    .tabs { display: flex; gap: 4px; flex-wrap: wrap; margin: 4px 0 16px; border-bottom: 1px solid var(--border); padding-bottom: 8px; }
    .tabs a { padding: 8px 12px; border-radius: 8px; text-decoration: none; color: var(--muted); font-weight: 600; }
    .tabs a:hover { background: var(--surface-2); }
    .tabs a.active { color: var(--primary); background: var(--primary-soft); }
  `,
})
export class AdminLayout {
  protected readonly tabs = [
    { path: '/admin', label: 'Ringkasan', exact: true },
    { path: '/admin/topup', label: 'Top-up' },
    { path: '/admin/penarikan', label: 'Penarikan' },
    { path: '/admin/kyc', label: 'Verifikasi KYC' },
    { path: '/admin/pengaturan', label: 'Pengaturan' },
    { path: '/admin/komisi', label: 'Komisi referral' },
    { path: '/admin/reputasi', label: 'Level reputasi' },
  ];
}
