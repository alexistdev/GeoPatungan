import { Component, inject, signal } from '@angular/core';
import { FormBuilder, ReactiveFormsModule, Validators } from '@angular/forms';
import { ActivatedRoute, Router, RouterLink } from '@angular/router';

import { AuthService } from '../../core/auth';
import { errorMessage } from '../../core/errors';
import { AuthShell } from '../../shared/auth-shell';

@Component({
  selector: 'app-register',
  imports: [ReactiveFormsModule, RouterLink, AuthShell],
  templateUrl: './register.html',
})
export class Register {
  private readonly auth = inject(AuthService);
  private readonly router = inject(Router);
  private readonly route = inject(ActivatedRoute);

  protected readonly form = inject(FormBuilder).nonNullable.group({
    name: ['', [Validators.required, Validators.maxLength(100)]],
    phone: ['', [Validators.required, Validators.pattern(/^\+?\d{9,15}$/)]],
    email: ['', [Validators.required, Validators.email]],
    password: ['', [Validators.required, Validators.minLength(8), Validators.maxLength(72)]],
    referral_code: [this.route.snapshot.queryParamMap.get('ref') ?? ''],
  });
  protected readonly loading = signal(false);
  protected readonly showPassword = signal(false);
  protected readonly error = signal<string | null>(null);

  protected submit(): void {
    if (this.form.invalid || this.loading()) {
      this.form.markAllAsTouched();
      return;
    }
    this.loading.set(true);
    this.error.set(null);
    const v = this.form.getRawValue();
    this.auth
      .register({
        name: v.name.trim(),
        phone: v.phone.trim(),
        email: v.email.trim(),
        password: v.password,
        referral_code: v.referral_code.trim().toUpperCase(),
      })
      .subscribe({
        next: () => void this.router.navigateByUrl('/'),
        error: (e: unknown) => {
          this.error.set(errorMessage(e));
          this.loading.set(false);
        },
      });
  }
}
