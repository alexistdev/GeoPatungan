import { Component, OnInit, inject, signal } from '@angular/core';
import { RouterLink } from '@angular/router';

import { AdminService } from '../../core/admin';
import { errorMessage } from '../../core/errors';
import { ReconcileReport } from '../../core/models';

@Component({
  selector: 'app-admin-overview',
  imports: [RouterLink],
  templateUrl: './overview.html',
})
export class AdminOverview implements OnInit {
  private readonly admin = inject(AdminService);
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
