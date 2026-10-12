import { DatePipe } from '@angular/common';
import { Component, OnInit, inject, signal } from '@angular/core';
import { RouterLink } from '@angular/router';

import { errorMessage } from '../../core/errors';
import { MeService } from '../../core/me';
import { LedgerEntry } from '../../core/models';
import { WalletService } from '../../core/wallet';
import { RupiahPipe } from '../../shared/rupiah-pipe';

const PAGE = 20;

@Component({
  selector: 'app-wallet-page',
  imports: [RouterLink, RupiahPipe, DatePipe],
  templateUrl: './wallet-page.html',
  styleUrl: './wallet.scss',
})
export class WalletPage implements OnInit {
  private readonly me = inject(MeService);
  private readonly wallets = inject(WalletService);

  protected readonly balance = signal<number | null>(null);
  protected readonly entries = signal<LedgerEntry[]>([]);
  protected readonly next = signal<number | null>(null);
  protected readonly loading = signal(true);
  protected readonly loadingMore = signal(false);
  protected readonly error = signal<string | null>(null);

  ngOnInit(): void {
    this.me.wallet().subscribe({
      next: (w) => this.balance.set(w.balance),
      error: (e: unknown) => this.error.set(errorMessage(e)),
    });
    this.wallets.ledger({ limit: PAGE }).subscribe({
      next: (p) => {
        this.entries.set(p.entries);
        this.next.set(p.next_before);
        this.loading.set(false);
      },
      error: (e: unknown) => {
        this.error.set(errorMessage(e));
        this.loading.set(false);
      },
    });
  }

  protected more(): void {
    const before = this.next();
    if (before === null || this.loadingMore()) return;
    this.loadingMore.set(true);
    this.wallets.ledger({ limit: PAGE, before }).subscribe({
      next: (p) => {
        this.entries.update((cur) => [...cur, ...p.entries]);
        this.next.set(p.next_before);
        this.loadingMore.set(false);
      },
      error: (e: unknown) => {
        this.error.set(errorMessage(e));
        this.loadingMore.set(false);
      },
    });
  }
}
