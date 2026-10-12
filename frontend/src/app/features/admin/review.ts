import { signal } from '@angular/core';
import { Observable } from 'rxjs';

import { errorMessage } from '../../core/errors';

/** Kondisi bersama halaman peninjauan admin: daftar berfilter + satu tindakan aktif pada satu baris. */
export class ReviewState<T> {
  readonly items = signal<T[]>([]);
  readonly loading = signal(true);
  readonly error = signal<string | null>(null);
  readonly status = signal('');
  /** `id:jenis`, mis. "abc:approve"; hanya satu form terbuka pada satu waktu. */
  readonly acting = signal<string | null>(null);
  readonly busy = signal(false);
  readonly actionError = signal<string | null>(null);
  readonly flash = signal<string | null>(null);

  constructor(private readonly fetch: (status: string) => Observable<T[]>, initial: string) {
    this.status.set(initial);
  }

  load(): void {
    this.loading.set(true);
    this.error.set(null);
    this.fetch(this.status()).subscribe({
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

  setStatus(s: string): void {
    this.status.set(s);
    this.acting.set(null);
    this.load();
  }

  open(id: string, kind: string): void {
    this.actionError.set(null);
    this.acting.set(`${id}:${kind}`);
  }

  close(): void {
    this.acting.set(null);
    this.actionError.set(null);
  }

  is(id: string, kind: string): boolean {
    return this.acting() === `${id}:${kind}`;
  }

  /** Menjalankan tindakan; bila sukses form ditutup, daftar dimuat ulang, dan pesan singkat tampil. */
  run(call: Observable<unknown>, successMessage: string): void {
    if (this.busy()) return;
    this.busy.set(true);
    this.actionError.set(null);
    call.subscribe({
      next: () => {
        this.busy.set(false);
        this.acting.set(null);
        this.flash.set(successMessage);
        this.load();
      },
      error: (e: unknown) => {
        this.busy.set(false);
        this.actionError.set(errorMessage(e));
      },
    });
  }
}
