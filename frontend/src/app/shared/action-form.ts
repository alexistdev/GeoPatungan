import { Component, OnInit, input, output } from '@angular/core';
import { AbstractControl, FormControl, FormGroup, ReactiveFormsModule, ValidatorFn, Validators } from '@angular/forms';

export interface FieldSpec {
  name: string;
  label: string;
  type?: 'text' | 'number';
  required?: boolean;
  minLength?: number;
  maxLength?: number;
  min?: number;
  max?: number;
  pattern?: RegExp;
  patternMessage?: string;
  hint?: string;
  value?: string | number;
}

/** Form kecil serbaguna untuk tindakan admin (catatan, referensi transfer, rekening, dst.). */
@Component({
  selector: 'app-action-form',
  imports: [ReactiveFormsModule],
  templateUrl: './action-form.html',
})
export class ActionForm implements OnInit {
  readonly fields = input.required<FieldSpec[]>();
  readonly submitLabel = input('Simpan');
  readonly busy = input(false);
  readonly error = input<string | null>(null);
  readonly submitted = output<Record<string, string>>();
  readonly cancelled = output<void>();

  protected form = new FormGroup<Record<string, FormControl<string>>>({});

  ngOnInit(): void {
    const controls: Record<string, FormControl<string>> = {};
    for (const f of this.fields()) {
      const v: ValidatorFn[] = [];
      if (f.required) v.push(Validators.required, notBlank);
      if (f.minLength) v.push(Validators.minLength(f.minLength));
      if (f.maxLength) v.push(Validators.maxLength(f.maxLength));
      if (f.min !== undefined) v.push(Validators.min(f.min));
      if (f.max !== undefined) v.push(Validators.max(f.max));
      if (f.pattern) v.push(Validators.pattern(f.pattern));
      controls[f.name] = new FormControl(String(f.value ?? ''), { nonNullable: true, validators: v });
    }
    this.form = new FormGroup(controls);
  }

  protected message(f: FieldSpec): string | null {
    const c = this.form.controls[f.name];
    if (!c || !c.touched || !c.errors) return null;
    const e = c.errors;
    if (e['required'] || e['blank']) return 'Wajib diisi.';
    if (e['minlength']) return `Minimal ${f.minLength} karakter.`;
    if (e['maxlength']) return `Maksimal ${f.maxLength} karakter.`;
    if (e['min']) return `Minimal ${f.min}.`;
    if (e['max']) return `Maksimal ${f.max}.`;
    if (e['pattern']) return f.patternMessage ?? 'Format tidak valid.';
    return 'Isian tidak valid.';
  }

  protected submit(): void {
    if (this.busy()) return;
    if (this.form.invalid) {
      this.form.markAllAsTouched();
      return;
    }
    const out: Record<string, string> = {};
    for (const [k, c] of Object.entries(this.form.controls)) out[k] = c.value.trim();
    this.submitted.emit(out);
  }
}

function notBlank(c: AbstractControl): { blank: true } | null {
  return typeof c.value === 'string' && c.value.length > 0 && c.value.trim() === '' ? { blank: true } : null;
}
