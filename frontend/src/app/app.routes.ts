import { Routes } from '@angular/router';

import { adminGuard, authGuard, guestGuard } from './core/guards';

export const routes: Routes = [
  {
    path: 'masuk',
    canActivate: [guestGuard],
    loadComponent: () => import('./features/auth/login').then((m) => m.Login),
    title: 'Masuk - GeoPatungan',
  },
  {
    path: 'daftar',
    canActivate: [guestGuard],
    loadComponent: () => import('./features/auth/register').then((m) => m.Register),
    title: 'Daftar - GeoPatungan',
  },
  {
    path: '',
    canActivate: [authGuard],
    loadComponent: () => import('./layout/shell').then((m) => m.Shell),
    children: [
      {
        path: '',
        pathMatch: 'full',
        loadComponent: () => import('./features/dashboard/dashboard').then((m) => m.Dashboard),
        title: 'Beranda - GeoPatungan',
      },
      {
        path: 'notifikasi',
        loadComponent: () => import('./features/notifications/notifications').then((m) => m.Notifications),
        title: 'Notifikasi - GeoPatungan',
      },
      {
        path: 'dompet',
        loadComponent: () => import('./features/wallet/wallet-page').then((m) => m.WalletPage),
        title: 'Dompet - GeoPatungan',
      },
      {
        path: 'dompet/isi-saldo',
        loadComponent: () => import('./features/wallet/topup-page').then((m) => m.TopupPage),
        title: 'Isi saldo - GeoPatungan',
      },
      {
        path: 'dompet/tarik',
        loadComponent: () => import('./features/wallet/withdraw-page').then((m) => m.WithdrawPage),
        title: 'Tarik dana - GeoPatungan',
      },
      {
        path: 'admin',
        canActivate: [adminGuard],
        loadComponent: () => import('./features/admin/admin-layout').then((m) => m.AdminLayout),
        children: [
          { path: '', pathMatch: 'full', loadComponent: () => import('./features/admin/overview').then((m) => m.AdminOverview), title: 'Admin - GeoPatungan' },
          { path: 'topup', loadComponent: () => import('./features/admin/topups').then((m) => m.AdminTopups), title: 'Top-up - Admin' },
          { path: 'penarikan', loadComponent: () => import('./features/admin/withdrawals').then((m) => m.AdminWithdrawals), title: 'Penarikan - Admin' },
          { path: 'kyc', loadComponent: () => import('./features/admin/kyc').then((m) => m.AdminKyc), title: 'KYC - Admin' },
          { path: 'pengaturan', loadComponent: () => import('./features/admin/settings').then((m) => m.AdminSettingsPage), title: 'Pengaturan - Admin' },
          { path: 'komisi', loadComponent: () => import('./features/admin/referral-rules').then((m) => m.AdminReferralRules), title: 'Komisi referral - Admin' },
          { path: 'reputasi', loadComponent: () => import('./features/admin/reputation-levels').then((m) => m.AdminReputationLevels), title: 'Level reputasi - Admin' },
        ],
      },
    ],
  },
  { path: '**', redirectTo: '' },
];

