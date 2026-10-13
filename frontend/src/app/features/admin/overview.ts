import { Component, OnInit, inject, signal } from '@angular/core';
import { RouterLink } from '@angular/router';

import { AdminService } from '../../core/admin';
import { errorMessage } from '../../core/errors';
import { ReconcileReport } from '../../core/models';
import { PageTitle } from '../../shared/page-title';

@Component({
  selector: 'app-admin-overview',
  imports: [RouterLink, PageTitle],
  templateUrl: './overview.html',
})
export class AdminOverview implements OnInit {
  private readonly admin = inject(AdminService);
  protected readonly tiles = [
    { path: '/admin/topup', title: 'Top-up', text: 'Verifikasi transfer masuk.', icon: 'ri-bank-card-line', tone: 'success' },
    { path: '/admin/penarikan', title: 'Penarikan', text: 'Proses permintaan tarik dana.', icon: 'ri-money-dollar-circle-line', tone: 'primary' },
    { path: '/admin/kyc', title: 'Verifikasi KYC', text: 'Putuskan pengajuan verifikasi.', icon: 'ri-shield-user-line', tone: 'warning' },
    { path: '/admin/pengaturan', title: 'Pengaturan', text: 'Biaya, batas nominal, rekening bank.', icon: 'ri-settings-3-line', tone: 'info' },
  ];
  protected readonly report = signal<ReconcileReport | null>(null);
  protected readonly loading = signal(false);
  protected readonly error = signal<string | null>(null);

  ngOnInit(): void {
    this.check();
  }

  protected check(): void {
    if (this.loading()) return;
    this.loading.set(true);
    this.error.set(null);
    this.admin.reconcile().subscribe({
      next: (r) => {
        this.report.set(r);
        this.loading.set(false);
      },
      error: (e: unknown) => {
        this.error.set(errorMessage(e));
        this.loading.set(false);
      },
    });
  }
}
