import { formatRupiah, RupiahPipe } from './rupiah-pipe';

describe('formatRupiah', () => {
  it.each([
    [0, 'Rp0'],
    [5, 'Rp5'],
    [999, 'Rp999'],
    [1000, 'Rp1.000'],
    [1234567, 'Rp1.234.567'],
    [-1500, '-Rp1.500'],
    [100000000, 'Rp100.000.000'],
  ])('%d -> %s', (n, want) => {
    expect(formatRupiah(n)).toBe(want);
  });

  it('nilai kosong atau bukan angka menjadi "-"', () => {
    expect(formatRupiah(null)).toBe('-');
    expect(formatRupiah(undefined)).toBe('-');
    expect(formatRupiah(NaN)).toBe('-');
  });

  it('pipe memakai fungsi yang sama', () => {
    expect(new RupiahPipe().transform(2500)).toBe('Rp2.500');
  });
});
