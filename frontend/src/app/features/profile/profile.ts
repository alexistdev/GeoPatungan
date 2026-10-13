import { DatePipe } from '@angular/common';
import { Component, OnInit, inject, signal } from '@angular/core';
import { FormBuilder, ReactiveFormsModule, Validators } from '@angular/forms';

import { AccountService } from '../../core/account';
import { AuthService } from '../../core/auth';
import { errorMessage } from '../../core/errors';
import { MeService } from '../../core/me';
import { KycState, ReputationProfile } from '../../core/models';
import { RupiahPipe } from '../../shared/rupiah-pipe';

@Component({
  selector: 'app-profile',
  imports: [ReactiveFormsModule, RupiahPipe, DatePipe],
  templateUrl: './profile.html',
  styleUrl: './profile.scss',
})
export class Profile implements OnInit {
  protected readonly auth = inject(AuthService);
  private readonly me = inject(MeService);
  private readonly account = inject(AccountService);

  protected readonly reputation = signal<ReputationProfile | null>(null);
  protected readonly repError = signal<string | null>(null);
  protected readonly kyc = signal<KycState | null>(null);
  protected readonly kycError = signal<string | null>(null);
  protected readonly submitting = signal(false);
  protected readonly submitError = signal<string | null>(null);
  /** Form pengajuan dibuka bila belum pernah mengajukan, atau setelah ditolak. */
  protected readonly formOpen = signal(false);

  protected readonly form = inject(FormBuilder).nonNullable.group({
    note: ['', [Validators.maxLength(200)]],
  });

  ngOnInit(): void {
    this.me.reputation().subscribe({
      next: (r) => this.reputation.set(r),
      error: (e: unknown) => this.repError.set(errorMessage(e)),
    });
    this.account.kyc().subscribe({
      next: (k) => this.kyc.set(k),
      error: (e: unknown) => this.kycError.set(errorMessage(e)),
    });
  }

  protected canSubmit(): boolean {
    const s = this.kyc()?.kyc_status;
    return s === 'none' || s === 'rejected';
  }

  protected percent(r: { current: number; required: number }): number {
    if (r.required <= 0) return 100;
    return Math.min(100, Math.round((r.current / r.required) * 100));
  }

  protected submit(): void {
    if (this.submitting()) return;
    if (this.form.invalid) {
      this.form.markAllAsTouched();
      return;
    }
    this.submitting.set(true);
    this.submitError.set(null);
    this.account.submitKyc(this.form.controls.note.value.trim()).subscribe({
      next: (req) => {
        this.kyc.set({ kyc_status: 'pending', latest_request: req });
        this.formOpen.set(false);
        this.form.reset({ note: '' });
        this.submitting.set(false);
        this.auth.loadUser().subscribe(); // status di header/beranda ikut diperbarui
        this.me.reputation().subscribe({ next: (r) => this.reputation.set(r), error: () => undefined });
      },
      error: (e: unknown) => {
        this.submitError.set(errorMessage(e));
        this.submitting.set(false);
      },
    });
  }
}
