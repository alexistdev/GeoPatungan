import { DatePipe } from '@angular/common';
import { Component, OnInit, computed, inject, signal } from '@angular/core';
import { RouterLink } from '@angular/router';

import { AuthService } from '../../core/auth';
import { errorMessage } from '../../core/errors';
import { GroupService } from '../../core/groups';
import { MeService } from '../../core/me';
import { Group, MyContribution, ReputationProfile, Wallet } from '../../core/models';
import { PageTitle } from '../../shared/page-title';
import { TONE, groupLabel } from '../../shared/labels';
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
  imports: [RupiahPipe, DatePipe, RouterLink, PageTitle],
  templateUrl: './dashboard.html',
})
export class Dashboard implements OnInit {
  protected readonly auth = inject(AuthService);
  private readonly me = inject(MeService);
  private readonly groupService = inject(GroupService);

  protected readonly wallet = signal<Section<Wallet>>(pending());
  protected readonly reputation = signal<Section<ReputationProfile>>(pending());
  protected readonly dues = signal<Section<MyContribution[]>>(pending());
  protected readonly groups = signal<Section<Group[]>>(pending());
  protected readonly groupLabel = groupLabel;
  /** Grup yang saya ikuti (bukan yang masih menunggu persetujuan atau ditolak). */
  protected readonly activeGroups = computed(() => (this.groups().data ?? []).filter((g) => g.my_status === 'active'));

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
    this.groupService.mine().subscribe({
      next: (data) => this.groups.set({ loading: false, data, error: null }),
      error: (e: unknown) => this.groups.set({ loading: false, data: null, error: errorMessage(e) }),
    });
  }

  protected total(c: MyContribution): number {
    return c.amount + c.platform_fee + c.late_fee;
  }

  protected kycTone(status: string | undefined): string {
    return status === 'verified' ? TONE.ok : status === 'pending' ? TONE.primary : status === 'rejected' ? TONE.danger : TONE.neutral;
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
