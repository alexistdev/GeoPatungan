import { Component, OnInit, inject, input, signal } from '@angular/core';
import { FormBuilder, ReactiveFormsModule, Validators } from '@angular/forms';
import { Router, RouterLink } from '@angular/router';

import { errorMessage } from '../../core/errors';
import { GroupService } from '../../core/groups';

@Component({
  selector: 'app-group-join',
  imports: [ReactiveFormsModule, RouterLink],
  templateUrl: './group-join.html',
})
export class GroupJoin implements OnInit {
  private readonly groups = inject(GroupService);
  private readonly router = inject(Router);

  /** Terisi dari ?kode= (tautan undangan). */
  readonly kode = input<string | undefined>();

  protected readonly form = inject(FormBuilder).nonNullable.group({
    code: ['', [Validators.required, Validators.minLength(4)]],
  });
  protected readonly saving = signal(false);
  protected readonly error = signal<string | null>(null);
  protected readonly joined = signal<string | null>(null);

  ngOnInit(): void {
    const k = this.kode();
    if (k) this.form.controls.code.setValue(k);
  }

  protected submit(): void {
    if (this.saving()) return;
    if (this.form.invalid) {
      this.form.markAllAsTouched();
      return;
    }
    this.saving.set(true);
    this.error.set(null);
    this.groups.join(this.form.controls.code.value.trim()).subscribe({
      next: (g) => {
        this.joined.set(g.name);
        this.saving.set(false);
      },
      error: (e: unknown) => {
        this.error.set(errorMessage(e));
        this.saving.set(false);
      },
    });
  }

  protected toList(): void {
    void this.router.navigateByUrl('/grup');
  }
}
