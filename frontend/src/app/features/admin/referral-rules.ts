import { Component, OnInit, computed, inject, signal } from '@angular/core';

import { AdminService } from '../../core/admin';
import { errorMessage } from '../../core/errors';
import { CommissionRule } from '../../core/models';
import { percentToBps } from './settings';
import { PageTitle } from '../../shared/page-title';

@Component({
  imports: [PageTitle],
  selector: 'app-admin-referral-rules',
  templateUrl: './referral-rules.html',
})
export class AdminReferralRules implements OnInit {
  private readonly admin = inject(AdminService);

  /** Teks isian persen per level (1..4), supaya "12,5" yang sedang diketik tidak dipaksa berubah. */
  protected readonly values = signal<string[]>(['', '', '', '']);
  protected readonly loading = signal(true);
  protected readonly saving = signal(false);
  protected readonly error = signal<string | null>(null);
  protected readonly flash = signal<string | null>(null);

  protected readonly bps = computed(() => this.values().map((v) => (v.trim() === '' ? NaN : percentToBps(v))));
  protected readonly valid = computed(() => this.bps().every((b) => Number.isInteger(b) && b >= 0 && b <= 10000));
  protected readonly totalBps = computed(() => this.bps().reduce((a, b) => a + (Number.isFinite(b) ? b : 0), 0));
  protected readonly over = computed(() => this.totalBps() > 10000);
  protected readonly platformKeeps = computed(() => (10000 - this.totalBps()) / 100);

  ngOnInit(): void {
    this.admin.referralRules().subscribe({
      next: (rules) => {
        this.values.set(rules.map((r) => String(r.share_bps / 100)));
        this.loading.set(false);
      },
      error: (e: unknown) => {
        this.error.set(errorMessage(e));
        this.loading.set(false);
      },
    });
  }

  protected bad(i: number): boolean {
    const b = this.bps()[i];
    return !Number.isInteger(b) || b < 0 || b > 10000;
  }

  protected set(i: number, v: string): void {
    this.values.update((cur) => cur.map((x, j) => (j === i ? v : x)));
    this.flash.set(null);
  }

  protected save(): void {
    if (this.saving() || !this.valid() || this.over()) return;
    this.saving.set(true);
    this.error.set(null);
    const rules: CommissionRule[] = this.bps().map((b, i) => ({ level: i + 1, share_bps: b }));
    this.admin.saveReferralRules(rules).subscribe({
      next: (saved) => {
        this.values.set(saved.map((r) => String(r.share_bps / 100)));
        this.saving.set(false);
        this.flash.set('Aturan komisi disimpan. Berlaku untuk pembayaran berikutnya; komisi yang sudah dibayar tidak berubah.');
      },
      error: (e: unknown) => {
        this.error.set(errorMessage(e));
        this.saving.set(false);
      },
    });
  }
}
