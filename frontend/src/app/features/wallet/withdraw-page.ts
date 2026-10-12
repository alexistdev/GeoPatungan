import { DatePipe } from '@angular/common';
import { HttpErrorResponse } from '@angular/common/http';
import { Component, OnInit, inject, signal } from '@angular/core';
import { FormBuilder, ReactiveFormsModule, Validators } from '@angular/forms';
import { RouterLink } from '@angular/router';

import { errorMessage } from '../../core/errors';
import { MeService } from '../../core/me';
import { PaymentInfo, Withdrawal } from '../../core/models';
import { WalletService } from '../../core/wallet';
import { withdrawalLabel } from '../../shared/labels';
import { formatRupiah, RupiahPipe } from '../../shared/rupiah-pipe';

@Component({
  selector: 'app-withdraw-page',
  imports: [ReactiveFormsModule, RouterLink, RupiahPipe, DatePipe],
  templateUrl: './withdraw-page.html',
  styleUrl: './wallet.scss',
})
export class WithdrawPage implements OnInit {
  private readonly wallets = inject(WalletService);
  private readonly me = inject(MeService);

  protected readonly info = signal<PaymentInfo | null>(null);
  protected readonly balance = signal<number | null>(null);
  protected readonly items = signal<Withdrawal[]>([]);
  protected readonly loading = signal(true);
  protected readonly submitting = signal(false);
  protected readonly cancelling = signal<string | null>(null);
  protected readonly error = signal<string | null>(null);

  protected readonly form = inject(FormBuilder).nonNullable.group({
    amount: [0, [Validators.required]],
    bank_name: ['', [Validators.required, Validators.minLength(2), Validators.maxLength(50)]],
    account_number: ['', [Validators.required, Validators.pattern(/^\d{5,30}$/)]],
    account_holder: ['', [Validators.required, Validators.minLength(2), Validators.maxLength(100)]],
    note: ['', [Validators.maxLength(200)]],
  });

  protected readonly label = withdrawalLabel;

  ngOnInit(): void {
    this.wallets.paymentInfo().subscribe({
      next: (i) => {
        this.info.set(i);
        this.form.controls.amount.setValidators([Validators.required, Validators.min(i.withdraw_min_amount)]);
        this.form.controls.amount.updateValueAndValidity();
      },
      error: (e: unknown) => this.error.set(errorMessage(e)),
    });
    this.refreshBalance();
    this.wallets.withdrawals().subscribe({
      next: (list) => {
        this.items.set(list);
        this.loading.set(false);
      },
      error: (e: unknown) => {
        this.error.set(errorMessage(e));
        this.loading.set(false);
      },
    });
  }

  private refreshBalance(): void {
    this.me.wallet().subscribe({ next: (w) => this.balance.set(w.balance), error: () => undefined });
  }

  protected submit(): void {
    const amount = Math.trunc(Number(this.form.controls.amount.value));
    if (this.form.invalid || !Number.isFinite(amount) || this.submitting()) {
      this.form.markAllAsTouched();
      return;
    }
    this.submitting.set(true);
    this.error.set(null);
    const v = this.form.getRawValue();
    this.wallets
      .createWithdrawal({
        amount,
        bank_name: v.bank_name.trim(),
        account_number: v.account_number.trim(),
        account_holder: v.account_holder.trim(),
        note: v.note.trim(),
      })
      .subscribe({
        next: (w) => {
          this.items.update((cur) => [w, ...cur]);
          this.form.reset({ amount: 0, bank_name: '', account_number: '', account_holder: '', note: '' });
          this.refreshBalance();
          this.submitting.set(false);
        },
        error: (e: unknown) => {
          this.error.set(this.explain(e));
          this.submitting.set(false);
        },
      });
  }

  protected cancel(w: Withdrawal): void {
    if (this.cancelling()) return;
    this.cancelling.set(w.id);
    this.error.set(null);
    this.wallets.cancelWithdrawal(w.id).subscribe({
      next: (updated) => {
        this.items.update((cur) => cur.map((x) => (x.id === updated.id ? updated : x)));
        this.refreshBalance();
        this.cancelling.set(null);
      },
      error: (e: unknown) => {
        this.error.set(errorMessage(e));
        this.cancelling.set(null);
      },
    });
  }

  /** Saldo tidak cukup (402) disertai angka dari server; ditampilkan lengkap. */
  private explain(e: unknown): string {
    if (e instanceof HttpErrorResponse && e.status === 402) {
      const b = e.error as { required?: number; balance?: number };
      if (typeof b?.required === 'number' && typeof b?.balance === 'number') {
        return `Saldo tidak cukup: dibutuhkan ${formatRupiah(b.required)}, saldo Anda ${formatRupiah(b.balance)}.`;
      }
    }
    return errorMessage(e);
  }
}
