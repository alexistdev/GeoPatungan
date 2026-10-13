import { DatePipe } from '@angular/common';
import { Component, inject, input, output, signal } from '@angular/core';
import { FormBuilder, ReactiveFormsModule, Validators } from '@angular/forms';

import { errorMessage } from '../../core/errors';
import { BankAccount, Topup } from '../../core/models';
import { WalletService } from '../../core/wallet';
import { topupLabel } from '../../shared/labels';
import { RupiahPipe } from '../../shared/rupiah-pipe';

/** Satu permintaan top-up: instruksi transfer, konfirmasi "sudah transfer", dan pembatalan. */
@Component({
  selector: 'app-topup-item',
  imports: [ReactiveFormsModule, RupiahPipe, DatePipe],
  templateUrl: './topup-item.html',
})
export class TopupItem {
  private readonly wallets = inject(WalletService);

  readonly topup = input.required<Topup>();
  readonly banks = input<BankAccount[]>([]);
  readonly updated = output<Topup>();

  protected readonly confirming = signal(false);
  protected readonly busy = signal(false);
  protected readonly error = signal<string | null>(null);

  protected readonly form = inject(FormBuilder).nonNullable.group({
    sender_name: ['', [Validators.required, Validators.minLength(2), Validators.maxLength(100)]],
    sender_bank: ['', [Validators.required, Validators.minLength(2), Validators.maxLength(50)]],
    note: ['', [Validators.maxLength(200)]],
  });

  protected label() {
    return topupLabel(this.topup().status);
  }

  protected isOpen(): boolean {
    const s = this.topup().status;
    return s === 'pending' || s === 'submitted';
  }

  protected openConfirm(): void {
    const t = this.topup();
    this.form.patchValue({ sender_name: t.sender_name, sender_bank: t.sender_bank, note: t.note });
    this.confirming.set(true);
  }

  protected confirm(): void {
    if (this.form.invalid || this.busy()) {
      this.form.markAllAsTouched();
      return;
    }
    this.busy.set(true);
    this.error.set(null);
    const v = this.form.getRawValue();
    this.wallets
      .confirmTopup(this.topup().id, { sender_name: v.sender_name.trim(), sender_bank: v.sender_bank.trim(), note: v.note.trim() })
      .subscribe({
        next: (t) => {
          this.busy.set(false);
          this.confirming.set(false);
          this.updated.emit(t);
        },
        error: (e: unknown) => {
          this.busy.set(false);
          this.error.set(errorMessage(e));
        },
      });
  }

  protected cancel(): void {
    if (this.busy()) return;
    this.busy.set(true);
    this.error.set(null);
    this.wallets.cancelTopup(this.topup().id).subscribe({
      next: (t) => {
        this.busy.set(false);
        this.updated.emit(t);
      },
      error: (e: unknown) => {
        this.busy.set(false);
        this.error.set(errorMessage(e));
      },
    });
  }
}
