import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';

import { LedgerEntry, PaymentInfo, Topup, Withdrawal } from '../../core/models';
import { TopupItem } from './topup-item';
import { TopupPage } from './topup-page';
import { WalletPage } from './wallet-page';
import { WithdrawPage } from './withdraw-page';

const setup = () =>
  TestBed.configureTestingModule({ providers: [provideRouter([]), provideHttpClient(), provideHttpClientTesting()] });
const ctl = () => TestBed.inject(HttpTestingController);

const info: PaymentInfo = {
  bank_accounts: [
    { id: 'b1', bank_name: 'BCA', account_number: '1234567890', account_holder: 'PT Geo', notes: '', is_active: true },
    { id: 'b2', bank_name: 'Mandiri', account_number: '999', account_holder: 'PT Geo', notes: '', is_active: false },
  ],
  topup_min_amount: 10_000, topup_max_amount: 1_000_000, withdraw_min_amount: 50_000, platform_fee_bps: 0, topup_expiry_hours: 24,
};
const topup = (o: Partial<Topup> = {}): Topup => ({
  id: 't1', amount: 100_000, unique_code: 347, transfer_amount: 100_347, status: 'pending', sender_name: '', sender_bank: '', note: '',
  submitted_at: null, expires_at: '2026-10-13T10:00:00+07:00', reviewed_at: null, review_note: '', received_amount: null,
  created_at: '2026-10-12T10:00:00+07:00', ...o,
});
const wd = (o: Partial<Withdrawal> = {}): Withdrawal => ({
  id: 'w1', amount: 100_000, bank_name: 'BCA', account_number: '1234567890', account_holder: 'Budi', note: '', status: 'pending',
  transfer_reference: '', review_note: '', reviewed_at: null, created_at: '2026-10-12T10:00:00+07:00', ...o,
});
const entry = (id: number, amount: number): LedgerEntry => ({
  id, amount, balance_after: 1000, ref_type: 'topup', ref_id: 'x', description: `Entri ${id}`, created_at: '2026-10-12T10:00:00+07:00',
});

function type(el: HTMLElement, name: string, v: string) {
  const i = el.querySelector<HTMLInputElement>(`input[formControlName=${name}]`)!;
  i.value = v;
  i.dispatchEvent(new Event('input'));
}
const submit = (el: HTMLElement) => el.querySelector('form')!.dispatchEvent(new Event('submit'));
const button = (el: HTMLElement, text: string) =>
  Array.from(el.querySelectorAll('button')).find((b) => b.textContent?.includes(text)) as HTMLButtonElement;

afterEach(() => ctl().verify());

describe('WalletPage', () => {
  it('menampilkan saldo dan riwayat, lalu memuat halaman berikutnya dengan kursor', () => {
    setup();
    const f = TestBed.createComponent(WalletPage);
    f.detectChanges();
    ctl().expectOne('/api/wallet/').flush({ wallet: { id: 'w', balance: 250_000, created_at: '' } });
    ctl().expectOne((r) => r.url === '/api/wallet/ledger' && !r.params.has('before')).flush({ entries: [entry(3, 5000), entry(2, -2000)], next_before: 2 });
    f.detectChanges();
    const el = f.nativeElement as HTMLElement;
    expect(el.textContent).toContain('Rp250.000');
    expect(el.textContent).toContain('+Rp5.000');
    expect(el.textContent).toContain('-Rp2.000');
    button(el, 'Muat lebih banyak').click();
    ctl().expectOne((r) => r.url === '/api/wallet/ledger' && r.params.get('before') === '2').flush({ entries: [entry(1, 100)], next_before: null });
    f.detectChanges();
    expect(el.querySelectorAll('li').length).toBe(3);
    expect(el.textContent).not.toContain('Muat lebih banyak');
  });

  it('keadaan kosong dan galat', () => {
    setup();
    const f = TestBed.createComponent(WalletPage);
    f.detectChanges();
    ctl().expectOne('/api/wallet/').flush({}, { status: 500, statusText: 'x' });
    ctl().expectOne((r) => r.url === '/api/wallet/ledger').flush({ entries: [], next_before: null });
    f.detectChanges();
    const el = f.nativeElement as HTMLElement;
    expect(el.textContent).toContain('Belum ada transaksi');
    expect(el.querySelector('[role=alert]')?.textContent).toContain('server');
  });
});

describe('TopupPage', () => {
  function mount(list: Topup[] = []) {
    setup();
    const f = TestBed.createComponent(TopupPage);
    f.detectChanges();
    ctl().expectOne('/api/payment-info').flush(info);
    ctl().expectOne('/api/wallet/topups').flush({ topups: list });
    f.detectChanges();
    return { f, el: f.nativeElement as HTMLElement };
  }

  it('hanya menampilkan rekening aktif pada instruksi', () => {
    const { el } = mount([topup()]);
    expect(el.textContent).toContain('BCA');
    expect(el.textContent).toContain('1234567890');
    expect(el.textContent).not.toContain('Mandiri');
    expect(el.textContent).toContain('Rp100.347');
    expect(el.textContent).toContain('kode unik 347');
  });

  it('nominal di luar batas ditolak di klien', () => {
    const { f, el } = mount();
    for (const bad of ['5000', '2000000', '']) {
      type(el, 'amount', bad);
      submit(el);
      f.detectChanges();
    }
    ctl().expectNone('/api/wallet/topups');
    expect(el.textContent).toContain('di luar batas');
  });

  it('membuat permintaan dan menampilkannya paling atas', () => {
    const { f, el } = mount([topup({ id: 'lama' })]);
    type(el, 'amount', '100000');
    submit(el);
    const req = ctl().expectOne((r) => r.method === 'POST' && r.url === '/api/wallet/topups');
    expect(req.request.body).toEqual({ amount: 100000 });
    req.flush({ topup: topup({ id: 'baru', unique_code: 12, transfer_amount: 100_012 }) }, { status: 201, statusText: 'Created' });
    f.detectChanges();
    expect(el.querySelectorAll('app-topup-item').length).toBe(2);
    expect(el.querySelector('app-topup-item')!.textContent).toContain('Rp100.012');
  });

  it('batas 3 permintaan terbuka dari server tampil sebagai pesan', () => {
    const { f, el } = mount();
    type(el, 'amount', '100000');
    submit(el);
    ctl().expectOne((r) => r.method === 'POST').flush({ error: 'terlalu banyak permintaan top-up yang belum selesai' }, { status: 409, statusText: 'x' });
    f.detectChanges();
    expect(el.querySelector('[role=alert]')?.textContent).toContain('terlalu banyak');
  });
});

describe('TopupItem', () => {
  function mount(t: Topup) {
    setup();
    const f = TestBed.createComponent(TopupItem);
    f.componentRef.setInput('topup', t);
    f.componentRef.setInput('banks', [info.bank_accounts[0]]);
    const updates: Topup[] = [];
    f.componentInstance.updated.subscribe((u) => updates.push(u));
    f.detectChanges();
    return { f, el: f.nativeElement as HTMLElement, updates };
  }

  it('konfirmasi transfer mengirim data bersih dan melaporkan hasil', () => {
    const { f, el, updates } = mount(topup());
    button(el, 'Saya sudah transfer').click();
    f.detectChanges();
    type(el, 'sender_name', '  Budi Santoso ');
    type(el, 'sender_bank', 'BCA');
    submit(el);
    const req = ctl().expectOne('/api/wallet/topups/t1/confirm');
    expect(req.request.body).toEqual({ sender_name: 'Budi Santoso', sender_bank: 'BCA', note: '' });
    req.flush({ topup: topup({ status: 'submitted', sender_name: 'Budi Santoso', sender_bank: 'BCA' }) });
    expect(updates[0].status).toBe('submitted');
  });

  it('form konfirmasi kosong tidak dikirim', () => {
    const { f, el } = mount(topup());
    button(el, 'Saya sudah transfer').click();
    f.detectChanges();
    submit(el);
    f.detectChanges();
    ctl().expectNone('/api/wallet/topups/t1/confirm');
    expect(el.textContent).toContain('2 sampai 100 karakter');
  });

  it('batalkan hanya untuk yang masih menunggu transfer', () => {
    const a = mount(topup({ status: 'pending' }));
    expect(button(a.el, 'Batalkan')).toBeTruthy();
    button(a.el, 'Batalkan').click();
    ctl().expectOne('/api/wallet/topups/t1/cancel').flush({ topup: topup({ status: 'cancelled' }) });
    expect(a.updates[0].status).toBe('cancelled');
    TestBed.resetTestingModule();
    const b = mount(topup({ status: 'submitted', sender_name: 'Budi', sender_bank: 'BCA' }));
    expect(button(b.el, 'Batalkan')).toBeUndefined();
    expect(b.el.textContent).toContain('Admin akan memverifikasi');
  });

  it('status akhir: disetujui menampilkan jumlah, ditolak menampilkan alasan, tanpa instruksi transfer', () => {
    const ok = mount(topup({ status: 'approved', received_amount: 100_347 }));
    expect(ok.el.textContent).toContain('Rp100.347');
    expect(ok.el.textContent).not.toContain('Transfer tepat');
    TestBed.resetTestingModule();
    const no = mount(topup({ status: 'rejected', review_note: 'transfer tidak ditemukan' }));
    expect(no.el.textContent).toContain('transfer tidak ditemukan');
    expect(no.el.textContent).not.toContain('Transfer tepat');
  });

  it('tanpa rekening aktif: peringatan, bukan daftar kosong', () => {
    setup();
    const f = TestBed.createComponent(TopupItem);
    f.componentRef.setInput('topup', topup());
    f.detectChanges();
    expect((f.nativeElement as HTMLElement).textContent).toContain('belum diatur admin');
  });
});

describe('WithdrawPage', () => {
  function mount(list: Withdrawal[] = []) {
    setup();
    const f = TestBed.createComponent(WithdrawPage);
    f.detectChanges();
    ctl().expectOne('/api/payment-info').flush(info);
    ctl().expectOne('/api/wallet/').flush({ wallet: { id: 'w', balance: 500_000, created_at: '' } });
    ctl().expectOne('/api/wallet/withdrawals').flush({ withdrawals: list });
    f.detectChanges();
    return { f, el: f.nativeElement as HTMLElement };
  }
  const fill = (el: HTMLElement, amount = '100000') => {
    type(el, 'amount', amount);
    type(el, 'bank_name', 'BCA');
    type(el, 'account_number', '1234567890');
    type(el, 'account_holder', 'Budi Santoso');
  };

  it('validasi klien: nominal di bawah minimal, rekening non-angka', () => {
    const { f, el } = mount();
    fill(el, '1000');
    type(el, 'account_number', '12ab');
    submit(el);
    f.detectChanges();
    ctl().expectNone((r) => r.method === 'POST');
    expect(el.textContent).toContain('kurang dari batas minimal');
    expect(el.textContent).toContain('5 sampai 30 digit angka');
  });

  it('mengajukan penarikan, menyegarkan saldo, dan mengosongkan form', () => {
    const { f, el } = mount();
    fill(el);
    submit(el);
    const req = ctl().expectOne((r) => r.method === 'POST' && r.url === '/api/wallet/withdrawals');
    expect(req.request.body).toEqual({ amount: 100000, bank_name: 'BCA', account_number: '1234567890', account_holder: 'Budi Santoso', note: '' });
    req.flush({ withdrawal: wd() }, { status: 201, statusText: 'Created' });
    ctl().expectOne('/api/wallet/').flush({ wallet: { id: 'w', balance: 400_000, created_at: '' } });
    f.detectChanges();
    expect(el.textContent).toContain('Rp400.000');
    expect(el.querySelectorAll('article').length).toBe(1);
    expect(el.querySelector<HTMLInputElement>('input[formControlName=bank_name]')!.value).toBe('');
  });

  it('saldo tidak cukup (402) menampilkan angka dari server', () => {
    const { f, el } = mount();
    fill(el, '900000');
    submit(el);
    ctl().expectOne((r) => r.method === 'POST').flush({ error: 'saldo tidak cukup', required: 900_000, balance: 500_000 }, { status: 402, statusText: 'x' });
    f.detectChanges();
    expect(el.querySelector('[role=alert]')?.textContent).toContain('dibutuhkan Rp900.000, saldo Anda Rp500.000');
  });

  it('pembatalan hanya untuk yang diproses, lalu saldo disegarkan', () => {
    const { f, el } = mount([wd({ id: 'a', status: 'pending' }), wd({ id: 'b', status: 'completed', transfer_reference: 'REF-1' })]);
    expect(el.querySelectorAll('button.danger').length).toBe(1);
    expect(el.textContent).toContain('REF-1');
    button(el, 'Batalkan').click();
    ctl().expectOne('/api/wallet/withdrawals/a/cancel').flush({ withdrawal: wd({ id: 'a', status: 'cancelled' }) });
    ctl().expectOne('/api/wallet/').flush({ wallet: { id: 'w', balance: 600_000, created_at: '' } });
    f.detectChanges();
    expect(el.querySelectorAll('button.danger').length).toBe(0);
    expect(el.textContent).toContain('Dibatalkan');
  });
});
