import { Component, OnInit, inject, signal } from '@angular/core';

import { AdminService, BankInput } from '../../core/admin';
import { errorMessage } from '../../core/errors';
import { AdminSettings, BankAccount } from '../../core/models';
import { ActionForm, FieldSpec } from '../../shared/action-form';

/** Persen -> basis poin tanpa galat floating point (2,5 -> 250). */
export function percentToBps(text: string): number {
  return Math.round(Number(text.replace(',', '.')) * 100);
}

@Component({
  selector: 'app-admin-settings',
  imports: [ActionForm],
  templateUrl: './settings.html',
})
export class AdminSettingsPage implements OnInit {
  private readonly admin = inject(AdminService);

  protected readonly settings = signal<AdminSettings | null>(null);
  protected readonly banks = signal<BankAccount[]>([]);
  protected readonly loading = signal(true);
  protected readonly error = signal<string | null>(null);
  protected readonly flash = signal<string | null>(null);

  protected readonly busy = signal(false);
  protected readonly formError = signal<string | null>(null);
  /** 'settings' | 'new' | id rekening yang sedang diubah. */
  protected readonly editing = signal<string | null>(null);

  protected settingsFields(s: AdminSettings): FieldSpec[] {
    return [
      { name: 'fee', label: 'Biaya platform per iuran (%)', type: 'number', required: true, min: 0, max: 10, value: s.platform_fee_bps / 100, hint: '0 sampai 10. Dari biaya inilah komisi referral dibayarkan.' },
      { name: 'topup_min', label: 'Top-up minimal (Rp)', type: 'number', required: true, min: 1000, value: s.topup_min_amount },
      { name: 'topup_max', label: 'Top-up maksimal (Rp)', type: 'number', required: true, max: 1_000_000_000, value: s.topup_max_amount },
      { name: 'withdraw_min', label: 'Penarikan minimal (Rp)', type: 'number', required: true, min: 10_000, value: s.withdraw_min_amount },
      { name: 'expiry', label: 'Batas waktu transfer top-up (jam)', type: 'number', required: true, min: 1, max: 168, value: s.topup_expiry_hours },
    ];
  }

  protected bankFields(b?: BankAccount): FieldSpec[] {
    return [
      { name: 'bank_name', label: 'Nama bank', required: true, minLength: 2, maxLength: 50, value: b?.bank_name },
      { name: 'account_number', label: 'Nomor rekening', required: true, pattern: /^\d{5,30}$/, patternMessage: '5 sampai 30 digit angka.', value: b?.account_number },
      { name: 'account_holder', label: 'Nama pemilik rekening', required: true, minLength: 2, maxLength: 100, value: b?.account_holder },
      { name: 'notes', label: 'Catatan untuk pengguna (opsional)', maxLength: 500, value: b?.notes },
    ];
  }

  ngOnInit(): void {
    this.admin.settings().subscribe({
      next: (s) => this.settings.set(s),
      error: (e: unknown) => this.error.set(errorMessage(e)),
    });
    this.loadBanks();
  }

  private loadBanks(): void {
    this.admin.banks().subscribe({
      next: (b) => {
        this.banks.set(b);
        this.loading.set(false);
      },
      error: (e: unknown) => {
        this.error.set(errorMessage(e));
        this.loading.set(false);
      },
    });
  }

  protected open(which: string): void {
    this.formError.set(null);
    this.editing.set(which);
  }

  protected close(): void {
    this.editing.set(null);
    this.formError.set(null);
  }

  private done(msg: string): void {
    this.busy.set(false);
    this.editing.set(null);
    this.flash.set(msg);
  }

  private fail(e: unknown): void {
    this.busy.set(false);
    this.formError.set(errorMessage(e));
  }

  protected saveSettings(v: Record<string, string>): void {
    if (this.busy()) return;
    this.busy.set(true);
    this.formError.set(null);
    const body: AdminSettings = {
      platform_fee_bps: percentToBps(v['fee']),
      topup_min_amount: Math.trunc(Number(v['topup_min'])),
      topup_max_amount: Math.trunc(Number(v['topup_max'])),
      withdraw_min_amount: Math.trunc(Number(v['withdraw_min'])),
      topup_expiry_hours: Math.trunc(Number(v['expiry'])),
    };
    this.admin.updateSettings(body).subscribe({
      next: (s) => {
        this.settings.set(s);
        this.done('Pengaturan disimpan. Berlaku untuk transaksi berikutnya.');
      },
      error: (e: unknown) => this.fail(e),
    });
  }

  private bankBody(v: Record<string, string>, active: boolean): BankInput {
    return { bank_name: v['bank_name'], account_number: v['account_number'], account_holder: v['account_holder'], notes: v['notes'], is_active: active };
  }

  protected saveBank(b: BankAccount | null, v: Record<string, string>): void {
    if (this.busy()) return;
    this.busy.set(true);
    this.formError.set(null);
    const call = b ? this.admin.updateBank(b.id, this.bankBody(v, b.is_active)) : this.admin.createBank(this.bankBody(v, true));
    call.subscribe({
      next: () => {
        this.done(b ? 'Rekening diperbarui.' : 'Rekening ditambahkan.');
        this.loadBanks();
      },
      error: (e: unknown) => this.fail(e),
    });
  }

  protected toggleBank(b: BankAccount): void {
    if (this.busy()) return;
    this.busy.set(true);
    this.error.set(null);
    this.admin
      .updateBank(b.id, { bank_name: b.bank_name, account_number: b.account_number, account_holder: b.account_holder, notes: b.notes, is_active: !b.is_active })
      .subscribe({
        next: () => {
          this.done(b.is_active ? 'Rekening dinonaktifkan.' : 'Rekening diaktifkan.');
          this.loadBanks();
        },
        error: (e: unknown) => {
          this.busy.set(false);
          this.error.set(errorMessage(e));
        },
      });
  }
}
