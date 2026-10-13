import { DatePipe } from '@angular/common';
import { Component, OnInit, computed, inject, signal } from '@angular/core';
import { RouterLink } from '@angular/router';

import { AccountService } from '../../core/account';
import { errorMessage } from '../../core/errors';
import { CommissionEntry, ReferralSummary } from '../../core/models';
import { RupiahPipe } from '../../shared/rupiah-pipe';

const PAGE = 20;

@Component({
  selector: 'app-referral',
  imports: [RouterLink, RupiahPipe, DatePipe],
  templateUrl: './referral.html',
})
export class Referral implements OnInit {
  private readonly account = inject(AccountService);

  protected readonly summary = signal<ReferralSummary | null>(null);
  protected readonly error = signal<string | null>(null);
  protected readonly items = signal<CommissionEntry[]>([]);
  protected readonly next = signal<number | null>(null);
  protected readonly loadingList = signal(true);
  protected readonly loadingMore = signal(false);
  protected readonly copied = signal(false);

  protected readonly link = computed(() => {
    const code = this.summary()?.referral_code;
    return code ? `${location.origin}/daftar?ref=${encodeURIComponent(code)}` : '';
  });
  protected readonly totalPercent = computed(() => (this.summary()?.rules ?? []).reduce((a, r) => a + r.share_bps, 0) / 100);

  ngOnInit(): void {
    this.account.referral().subscribe({
      next: (s) => this.summary.set(s),
      error: (e: unknown) => this.error.set(errorMessage(e)),
    });
    this.account.commissions({ limit: PAGE }).subscribe({
      next: (p) => {
        this.items.set(p.commissions);
        this.next.set(p.next_before);
        this.loadingList.set(false);
      },
      error: (e: unknown) => {
        this.error.set(errorMessage(e));
        this.loadingList.set(false);
      },
    });
  }

  protected more(): void {
    const before = this.next();
    if (before === null || this.loadingMore()) return;
    this.loadingMore.set(true);
    this.account.commissions({ limit: PAGE, before }).subscribe({
      next: (p) => {
        this.items.update((cur) => [...cur, ...p.commissions]);
        this.next.set(p.next_before);
        this.loadingMore.set(false);
      },
      error: (e: unknown) => {
        this.error.set(errorMessage(e));
        this.loadingMore.set(false);
      },
    });
  }

  protected downline(level: number): number {
    const d = this.summary()?.downline;
    return d ? [d.level_1, d.level_2, d.level_3, d.level_4][level - 1] : 0;
  }

  protected earned(level: number): number {
    return this.summary()?.earned.by_level[String(level)] ?? 0;
  }

  protected async copy(): Promise<void> {
    try {
      await navigator.clipboard.writeText(this.link());
      this.copied.set(true);
      setTimeout(() => this.copied.set(false), 2000);
    } catch {
      /* clipboard bisa diblokir; tautan tetap terlihat untuk disalin manual */
    }
  }
}
