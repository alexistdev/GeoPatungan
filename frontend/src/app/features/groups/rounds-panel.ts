import { DatePipe } from '@angular/common';
import { HttpErrorResponse } from '@angular/common/http';
import { Component, OnInit, inject, input, output, signal } from '@angular/core';
import { RouterLink } from '@angular/router';

import { errorMessage } from '../../core/errors';
import { GroupService } from '../../core/groups';
import { MeService } from '../../core/me';
import { Group, MyContribution, PayResult, Quote, Round, RoundContribution } from '../../core/models';
import { roundLabel } from '../../shared/labels';
import { formatRupiah, RupiahPipe } from '../../shared/rupiah-pipe';

@Component({
  selector: 'app-rounds-panel',
  imports: [DatePipe, RupiahPipe, RouterLink],
  templateUrl: './rounds-panel.html',
})
export class RoundsPanel implements OnInit {
  private readonly groups = inject(GroupService);
  private readonly me = inject(MeService);

  readonly group = input.required<Group>();
  readonly changed = output<void>();

  protected readonly rounds = signal<Round[]>([]);
  protected readonly mine = signal<Map<string, MyContribution>>(new Map());
  protected readonly loading = signal(true);
  protected readonly error = signal<string | null>(null);
  protected readonly label = roundLabel;

  /** Pembayaran yang sedang diperiksa/dikonfirmasi. */
  protected readonly quote = signal<Quote | null>(null);
  protected readonly quoting = signal<string | null>(null);
  protected readonly paying = signal(false);
  protected readonly payError = signal<string | null>(null);
  /** Galat saat memuat rincian (sebelum panel rincian ada). */
  protected readonly quoteError = signal<string | null>(null);
  protected readonly short = signal<{ required: number; balance: number } | null>(null);
  protected readonly receipt = signal<PayResult | null>(null);

  protected readonly expanded = signal<string | null>(null);
  protected readonly detail = signal<RoundContribution[] | null>(null);
  protected readonly detailError = signal<string | null>(null);

  ngOnInit(): void {
    this.load();
  }

  protected load(): void {
    this.loading.set(true);
    this.error.set(null);
    this.groups.rounds(this.group().id).subscribe({
      next: (r) => {
        this.rounds.set(r);
        this.me.contributions().subscribe({
          next: (list) => {
            this.mine.set(new Map(list.filter((c) => c.group_id === this.group().id).map((c) => [c.round_id, c])));
            this.loading.set(false);
          },
          error: (e: unknown) => {
            this.error.set(errorMessage(e));
            this.loading.set(false);
          },
        });
      },
      error: (e: unknown) => {
        this.error.set(errorMessage(e));
        this.loading.set(false);
      },
    });
  }

  protected myContribution(r: Round): MyContribution | undefined {
    return this.mine().get(r.id);
  }

  protected startPay(c: MyContribution): void {
    this.payError.set(null);
    this.quoteError.set(null);
    this.short.set(null);
    this.receipt.set(null);
    this.quote.set(null);
    this.quoting.set(c.id);
    this.groups.quote(c.id).subscribe({
      next: (q) => {
        this.quote.set(q);
        this.quoting.set(null);
      },
      error: (e: unknown) => {
        this.quoting.set(null);
        this.quoteError.set(errorMessage(e));
      },
    });
  }

  protected cancelPay(): void {
    this.quote.set(null);
    this.payError.set(null);
    this.short.set(null);
  }

  protected confirmPay(): void {
    const q = this.quote();
    if (!q || this.paying() || !q.payable) return;
    this.paying.set(true);
    this.payError.set(null);
    this.short.set(null);
    this.groups.pay(q.contribution_id).subscribe({
      next: (res) => {
        this.paying.set(false);
        this.quote.set(null);
        this.receipt.set(res);
        this.load();
        this.changed.emit();
      },
      error: (e: unknown) => {
        this.paying.set(false);
        if (e instanceof HttpErrorResponse && e.status === 402) {
          const b = e.error as { required?: number; balance?: number };
          if (typeof b?.required === 'number' && typeof b?.balance === 'number') {
            this.short.set({ required: b.required, balance: b.balance });
            return;
          }
        }
        this.payError.set(errorMessage(e));
      },
    });
  }

  protected toggle(r: Round): void {
    if (this.expanded() === r.id) {
      this.expanded.set(null);
      return;
    }
    this.expanded.set(r.id);
    this.detail.set(null);
    this.detailError.set(null);
    this.groups.roundContributions(this.group().id, r.id).subscribe({
      next: (list) => this.detail.set(list),
      error: (e: unknown) => this.detailError.set(errorMessage(e)),
    });
  }

  protected rupiah = formatRupiah;
}
