import { Pipe, PipeTransform } from '@angular/core';

/** 1234567 -> "Rp1.234.567". Nilai kosong ditampilkan sebagai "-". */
export function formatRupiah(n: number | null | undefined): string {
  if (n === null || n === undefined || Number.isNaN(n)) {
    return '-';
  }
  const abs = Math.abs(Math.trunc(n)).toString().replace(/\B(?=(\d{3})+(?!\d))/g, '.');
  return (n < 0 ? '-Rp' : 'Rp') + abs;
}

@Pipe({ name: 'rupiah' })
export class RupiahPipe implements PipeTransform {
  transform(value: number | null | undefined): string {
    return formatRupiah(value);
  }
}
