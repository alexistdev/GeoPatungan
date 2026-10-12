import { Component, OnInit, computed, inject, signal } from '@angular/core';
import { FormBuilder, ReactiveFormsModule, Validators } from '@angular/forms';
import { RouterLink } from '@angular/router';

import { errorMessage } from '../../core/errors';
import { PaymentInfo, Topup } from '../../core/models';
import { WalletService } from '../../core/wallet';
import { RupiahPipe } from '../../shared/rupiah-pipe';
import { TopupItem } from './topup-item';

@Component({
  selector: 'app-topup-page',
  imports: [ReactiveFormsModule, RouterLink, RupiahPipe, TopupItem],
  templateUrl: './topup-page.html',
  styleUrl: './wallet.scss',
})
export class TopupPage implements OnInit {
  private readonly wallets = inject(WalletService);

  protected readonly info = signal<PaymentInfo | null>(null);
  protected readonly topups = signal<Topup[]>([]);
  protected readonly loading = signal(true);
  protected readonly error = signal<string | null>(null);
  protected readonly creating = signal(false);
  protected readonly activeBanks = computed(() => (this.info()?.bank_accounts ?? []).filter((b) => b.is_active));

  protected readonly form = inject(FormBuilder).nonNullable.group({
    amount: [0, [Validators.required]],
  });

  ngOnInit(): void {
    this.wallets.paymentInfo().subscribe({
      next: (i) => {
        this.info.set(i);
        this.form.controls.amount.setValidators([Validators.required, Validators.min(i.topup_min_amount), Validators.max(i.topup_max_amount)]);
        this.form.controls.amount.updateValueAndValidity();
      },
      error: (e: unknown) => this.error.set(errorMessage(e)),
    });
    this.wallets.topups().subscribe({
      next: (list) => {
        this.topups.set(list);
        this.loading.set(false);
      },
      error: (e: unknown) => {
        this.error.set(errorMessage(e));
        this.loading.set(false);
      },
    });
  }

  protected create(): void {
    const amount = Math.trunc(Number(this.form.controls.amount.value));
    if (this.form.invalid || !Number.isFinite(amount) || this.creating()) {
      this.form.markAllAsTouched();
      return;
    }
    this.creating.set(true);
    this.error.set(null);
    this.wallets.createTopup(amount).subscribe({
      next: (t) => {
        this.topups.update((cur) => [t, ...cur]);
        this.form.controls.amount.setValue(0);
        this.form.controls.amount.markAsUntouched();
        this.creating.set(false);
      },
      error: (e: unknown) => {
        this.error.set(errorMessage(e));
        this.creating.set(false);
      },
    });
  }

  protected replace(updated: Topup): void {
    this.topups.update((cur) => cur.map((t) => (t.id === updated.id ? updated : t)));
  }
}
