import { Component, OnInit, inject, signal } from '@angular/core';
import { FormArray, FormBuilder, ReactiveFormsModule, Validators } from '@angular/forms';

import { AdminService } from '../../core/admin';
import { errorMessage } from '../../core/errors';
import { ReputationLevel } from '../../core/models';

/** Teks kosong = tanpa batas (null); selain itu bilangan bulat. */
function optionalInt(text: string): number | null {
  const t = text.trim();
  return t === '' ? null : Math.trunc(Number(t));
}

@Component({
  selector: 'app-admin-reputation-levels',
  imports: [ReactiveFormsModule],
  templateUrl: './reputation-levels.html',
})
export class AdminReputationLevels implements OnInit {
  private readonly admin = inject(AdminService);
  private readonly fb = inject(FormBuilder).nonNullable;

  protected readonly levels = this.fb.array<ReturnType<AdminReputationLevels['levelGroup']>>([]);
  protected readonly loading = signal(true);
  protected readonly saving = signal(false);
  protected readonly error = signal<string | null>(null);
  protected readonly flash = signal<string | null>(null);

  private levelGroup(l: ReputationLevel) {
    return this.fb.group({
      rank: [l.rank],
      name: [l.name, [Validators.required, Validators.minLength(2), Validators.maxLength(30)]],
      code: [l.code, [Validators.required, Validators.pattern(/^[a-z_]{2,20}$/)]],
      require_verified: [l.require_verified],
      min_completed_cycles: [String(l.min_completed_cycles), [Validators.required, Validators.min(0), Validators.max(1000)]],
      max_late_count: [l.max_late_count === null ? '' : String(l.max_late_count), [Validators.min(0), Validators.max(1000)]],
      draw_weight: [String(l.draw_weight), [Validators.required, Validators.min(1), Validators.max(3)]],
      can_create_group: [l.can_create_group],
      max_group_value: [l.max_group_value === null ? '' : String(l.max_group_value), [Validators.min(10_000), Validators.max(100_000_000)]],
    });
  }

  ngOnInit(): void {
    this.admin.levels().subscribe({
      next: (ls) => {
        this.fill(ls);
        this.loading.set(false);
      },
      error: (e: unknown) => {
        this.error.set(errorMessage(e));
        this.loading.set(false);
      },
    });
  }

  private fill(ls: ReputationLevel[]): void {
    (this.levels as FormArray).clear();
    for (const l of ls) this.levels.push(this.levelGroup(l));
  }

  protected invalid(i: number, name: string): boolean {
    const c = this.levels.at(i).get(name);
    return !!c && c.touched && c.invalid;
  }

  protected save(): void {
    if (this.saving()) return;
    if (this.levels.invalid) {
      this.levels.markAllAsTouched();
      return;
    }
    this.saving.set(true);
    this.error.set(null);
    this.flash.set(null);
    const body: ReputationLevel[] = this.levels.getRawValue().map((v) => ({
      rank: v.rank,
      name: v.name.trim(),
      code: v.code.trim(),
      require_verified: v.require_verified,
      min_completed_cycles: Math.trunc(Number(v.min_completed_cycles)),
      max_late_count: optionalInt(v.max_late_count),
      draw_weight: Number(v.draw_weight),
      can_create_group: v.can_create_group,
      max_group_value: optionalInt(v.max_group_value),
    }));
    this.admin.saveLevels(body).subscribe({
      next: (saved) => {
        this.fill(saved);
        this.saving.set(false);
        this.flash.set('Level disimpan. Berlaku seketika untuk semua pengguna.');
      },
      error: (e: unknown) => {
        this.error.set(errorMessage(e));
        this.saving.set(false);
      },
    });
  }
}
