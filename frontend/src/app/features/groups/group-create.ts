import { Component, inject, signal } from '@angular/core';
import { FormBuilder, ReactiveFormsModule, Validators } from '@angular/forms';
import { Router, RouterLink } from '@angular/router';

import { errorMessage } from '../../core/errors';
import { GroupService } from '../../core/groups';
import { PageTitle } from '../../shared/page-title';
import { RupiahPipe } from '../../shared/rupiah-pipe';

/** Tanggal lokal YYYY-MM-DD untuk besok (batas awal yang diizinkan input tanggal). */
export function tomorrow(now = new Date()): string {
  const d = new Date(now.getFullYear(), now.getMonth(), now.getDate() + 1);
  const p = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`;
}

@Component({
  selector: 'app-group-create',
  imports: [ReactiveFormsModule, RupiahPipe, PageTitle],
  templateUrl: './group-create.html',
})
export class GroupCreate {
  private readonly groups = inject(GroupService);
  private readonly router = inject(Router);

  protected readonly minDate = tomorrow();
  protected readonly saving = signal(false);
  protected readonly error = signal<string | null>(null);

  protected readonly form = inject(FormBuilder).nonNullable.group({
    name: ['', [Validators.required, Validators.minLength(3), Validators.maxLength(100)]],
    contribution_amount: [100000, [Validators.required, Validators.min(10_000), Validators.max(100_000_000)]],
    period_type: ['monthly'],
    slot_count: [5, [Validators.required, Validators.min(2), Validators.max(50)]],
    start_date: [tomorrow(), [Validators.required]],
    late_fee: [5000, [Validators.required, Validators.min(0)]],
    draw_method: ['random'],
    min_level_rank: [1],
  });

  /** Total dana yang terkumpul per putaran = iuran × jumlah anggota. */
  protected pot(): number {
    const v = this.form.getRawValue();
    return Math.max(0, Math.trunc(Number(v.contribution_amount) || 0)) * Math.max(0, Math.trunc(Number(v.slot_count) || 0));
  }

  protected invalid(name: keyof typeof this.form.controls): boolean {
    const c = this.form.controls[name];
    return c.touched && c.invalid;
  }

  protected lateFeeTooHigh(): boolean {
    const v = this.form.getRawValue();
    return Number(v.late_fee) > Number(v.contribution_amount);
  }

  protected submit(): void {
    if (this.saving()) return;
    if (this.form.invalid || this.lateFeeTooHigh() || this.form.controls.start_date.value < this.minDate) {
      this.form.markAllAsTouched();
      return;
    }
    this.saving.set(true);
    this.error.set(null);
    const v = this.form.getRawValue();
    this.groups
      .create({
        name: v.name.trim(),
        contribution_amount: Math.trunc(Number(v.contribution_amount)),
        period_type: v.period_type,
        slot_count: Math.trunc(Number(v.slot_count)),
        start_date: v.start_date,
        late_fee: Math.trunc(Number(v.late_fee)),
        draw_method: v.draw_method,
        min_level_rank: Number(v.min_level_rank),
      })
      .subscribe({
        next: (g) => void this.router.navigate(['/grup', g.id]),
        error: (e: unknown) => {
          this.error.set(errorMessage(e));
          this.saving.set(false);
        },
      });
  }
}
