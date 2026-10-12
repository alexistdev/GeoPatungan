import { DatePipe } from '@angular/common';
import { Component, OnInit, inject, signal } from '@angular/core';
import { RouterLink } from '@angular/router';

import { AuthService } from '../../core/auth';
import { errorMessage } from '../../core/errors';
import { MeService } from '../../core/me';
import { MyContribution, ReputationProfile, Wallet } from '../../core/models';
import { RupiahPipe } from '../../shared/rupiah-pipe';

/** Bagian halaman yang dimuat sendiri-sendiri: satu gagal tidak menghapus yang lain. */
interface Section<T> {
  loading: boolean;
  data: T | null;
  error: string | null;
}

function pending<T>(): Section<T> {
  return { loading: true, data: null, error: null };
}

@Component({
  selector: 'app-dashboard',
  imports: [RupiahPipe, DatePipe, RouterLink],
  templateUrl: './dashboard.html',
  styleUrl: './dashboard.scss',
})
export class Dashboard implements OnInit {
  protected readonly auth = inject(AuthService);
  private readonly me = inject(MeService);

  protected readonly wallet = signal<Section<Wallet>>(pending());
  protected readonly reputation = signal<Section<ReputationProfile>>(pending());
  protected readonly dues = signal<Section<MyContribution[]>>(pending());

  ngOnInit(): void {
    this.me.wallet().subscribe({
      next: (data) => this.wallet.set({ loading: false, data, error: null }),
      error: (e: unknown) => this.wallet.set({ loading: false, data: null, error: errorMessage(e) }),
    });
    this.me.reputation().subscribe({
      next: (data) => this.reputation.set({ loading: false, data, error: null }),
      error: (e: unknown) => this.reputation.set({ loading: false, data: null, error: errorMessage(e) }),
    });
    this.me.contributions('unpaid').subscribe({
      next: (data) => this.dues.set({ loading: false, data, error: null }),
      error: (e: unknown) => this.dues.set({ loading: false, data: null, error: errorMessage(e) }),
    });
  }

  protected total(c: MyContribution): number {
    return c.amount + c.platform_fee + c.late_fee;
  }

  protected kycLabel(status: string | undefined): string {
    switch (status) {
      case 'verified':
        return 'Terverifikasi';
      case 'pending':
        return 'Menunggu verifikasi';
      case 'rejected':
        return 'Verifikasi ditolak';
      default:
        return 'Belum terverifikasi';
    }
  }
}
