import { Component, signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';

import { ActionForm, FieldSpec } from './action-form';

@Component({
  imports: [ActionForm],
  template: `<app-action-form [fields]="fields" submitLabel="Kirim" [busy]="busy()" [error]="err()" (submitted)="got.push($event)" (cancelled)="cancelled = true" />`,
})
class Host {
  fields: FieldSpec[] = [
    { name: 'note', label: 'Alasan', required: true, minLength: 3, maxLength: 10 },
    { name: 'amount', label: 'Jumlah', type: 'number', min: 5, max: 100 },
    { name: 'no', label: 'Nomor', pattern: /^\d+$/, patternMessage: 'Hanya angka.', value: '123' },
  ];
  busy = signal(false);
  err = signal<string | null>(null);
  got: Record<string, string>[] = [];
  cancelled = false;
}

function mount() {
  const f = TestBed.createComponent(Host);
  f.detectChanges();
  const el = f.nativeElement as HTMLElement;
  const type = (name: string, v: string) => {
    const i = el.querySelector<HTMLInputElement>(`input[name=${name}]`)!;
    i.value = v;
    i.dispatchEvent(new Event('input'));
  };
  const submit = () => {
    el.querySelector('form')!.dispatchEvent(new Event('submit'));
    f.detectChanges();
  };
  return { f, el, type, submit, host: f.componentInstance };
}

describe('ActionForm', () => {
  it('nilai awal terisi dan isian wajib kosong tidak terkirim', () => {
    const { el, submit, host } = mount();
    expect(el.querySelector<HTMLInputElement>('input[name=no]')!.value).toBe('123');
    submit();
    expect(host.got.length).toBe(0);
    expect(el.textContent).toContain('Wajib diisi');
  });

  it('isian hanya spasi dianggap kosong', () => {
    const { type, submit, host, el } = mount();
    type('note', '     ');
    submit();
    expect(host.got.length).toBe(0);
    expect(el.textContent).toContain('Wajib diisi');
  });

  it('validasi panjang, rentang angka, dan pola', () => {
    const { type, submit, host, el } = mount();
    type('note', 'ab');
    type('amount', '1');
    type('no', '12a');
    submit();
    expect(host.got.length).toBe(0);
    expect(el.textContent).toContain('Minimal 3 karakter');
    expect(el.textContent).toContain('Minimal 5.');
    expect(el.textContent).toContain('Hanya angka.');
    type('note', 'a'.repeat(11));
    type('amount', '101');
    submit();
    expect(el.textContent).toContain('Maksimal 10 karakter');
    expect(el.textContent).toContain('Maksimal 100.');
  });

  it('mengirim nilai yang sudah di-trim; kolom opsional kosong tetap ada sebagai string kosong', () => {
    const { type, submit, host } = mount();
    type('note', '  alasan ');
    submit();
    expect(host.got).toEqual([{ note: 'alasan', amount: '', no: '123' }]);
  });

  it('sibuk: tombol nonaktif dan tidak mengirim ganda', () => {
    const { f, type, submit, host, el } = mount();
    type('note', 'alasan');
    host.busy.set(true);
    f.detectChanges();
    expect(el.querySelector<HTMLButtonElement>('button[type=submit]')!.disabled).toBe(true);
    submit();
    expect(host.got.length).toBe(0);
  });

  it('menampilkan galat dari luar dan memancarkan batal', () => {
    const { f, host, el } = mount();
    host.err.set('admin tidak boleh memproses miliknya sendiri');
    f.detectChanges();
    expect(el.querySelector('[role=alert]')?.textContent).toContain('miliknya sendiri');
    Array.from(el.querySelectorAll('button')).find((b) => b.textContent?.includes('Batal'))!.click();
    expect(host.cancelled).toBe(true);
  });
});
