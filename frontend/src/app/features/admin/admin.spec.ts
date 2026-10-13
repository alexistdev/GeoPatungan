import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';

import { AdminKycRequest, AdminTopup, AdminWithdrawal, ReputationLevel } from '../../core/models';
import { AdminKyc } from './kyc';
import { AdminOverview } from './overview';
import { AdminReferralRules } from './referral-rules';
import { AdminReputationLevels } from './reputation-levels';
import { AdminSettingsPage, percentToBps } from './settings';
import { AdminTopups } from './topups';
import { AdminWithdrawals } from './withdrawals';

const setup = () => TestBed.configureTestingModule({ providers: [provideRouter([]), provideHttpClient(), provideHttpClientTesting()] });
const ctl = () => TestBed.inject(HttpTestingController);

const type = (el: HTMLElement, name: string, v: string) => {
  const i = el.querySelector<HTMLInputElement>(`input[name=${name}], input[formControlName=${name}]`)!;
  i.value = v;
  i.dispatchEvent(new Event('input'));
};
const submit = (el: HTMLElement) => el.querySelector('form')!.dispatchEvent(new Event('submit'));
const button = (el: HTMLElement, text: string) =>
  Array.from(el.querySelectorAll('button')).find((b) => b.textContent?.includes(text)) as HTMLButtonElement;

const user = { user_id: 'u', user_name: 'Budi', user_email: 'b@x.id', reviewed_by: null };
const topup = (o: Partial<AdminTopup> = {}): AdminTopup => ({
  id: 't1', amount: 100_000, unique_code: 347, transfer_amount: 100_347, status: 'submitted', sender_name: 'Budi S', sender_bank: 'BCA',
  note: '', submitted_at: null, expires_at: '2026-10-13T10:00:00+07:00', reviewed_at: null, review_note: '', received_amount: null,
  created_at: '2026-10-12T10:00:00+07:00', ...user, ...o,
});
const wd = (o: Partial<AdminWithdrawal> = {}): AdminWithdrawal => ({
  id: 'w1', amount: 200_000, bank_name: 'BCA', account_number: '1234567890', account_holder: 'Budi', note: '', status: 'pending',
  transfer_reference: '', review_note: '', reviewed_at: null, created_at: '2026-10-12T10:00:00+07:00', ...user, ...o,
});
const kyc = (o: Partial<AdminKycRequest> = {}): AdminKycRequest => ({
  id: 'k1', user_id: 'u', status: 'pending', note: 'ktp saya', review_note: '', submitted_at: '2026-10-12T10:00:00+07:00', reviewed_at: null,
  user_name: 'Budi', user_email: 'b@x.id', reviewed_by: null, ...o,
});

describe('AdminTopups', () => {
  afterEach(() => ctl().verify());
  function mount(list: AdminTopup[]) {
    setup();
    const f = TestBed.createComponent(AdminTopups);
    f.detectChanges();
    ctl().expectOne((r) => r.url === '/api/admin/topups' && r.params.get('status') === 'submitted').flush({ topups: list });
    f.detectChanges();
    return { f, el: f.nativeElement as HTMLElement };
  }

  it('menampilkan nominal transfer tepat, kode unik, dan data pengirim', () => {
    const { el } = mount([topup()]);
    expect(el.textContent).toContain('Rp100.347');
    expect(el.textContent).toContain('kode unik 347');
    expect(el.textContent).toContain('Budi S (BCA)');
  });

  it('menyetujui tanpa jumlah khusus: received_amount tidak dikirim, daftar dimuat ulang', () => {
    const { f, el } = mount([topup()]);
    button(el, 'Setujui').click();
    f.detectChanges();
    submit(el);
    const req = ctl().expectOne('/api/admin/topups/t1/approve');
    expect(req.request.body).toEqual({ note: '' });
    req.flush({ topup: {} });
    ctl().expectOne((r) => r.url === '/api/admin/topups').flush({ topups: [] });
    f.detectChanges();
    expect(el.textContent).toContain('Top-up Budi disetujui');
    expect(el.textContent).toContain('Tidak ada top-up');
  });

  it('menyetujui dengan jumlah diterima berbeda', () => {
    const { f, el } = mount([topup()]);
    button(el, 'Setujui').click();
    f.detectChanges();
    type(el, 'received_amount', '99000');
    type(el, 'note', 'kurang bayar');
    submit(el);
    const req = ctl().expectOne('/api/admin/topups/t1/approve');
    expect(req.request.body).toEqual({ received_amount: 99000, note: 'kurang bayar' });
    req.flush({});
    ctl().expectOne((r) => r.url === '/api/admin/topups').flush({ topups: [] });
  });

  it('menolak wajib beralasan; tanpa alasan tidak dikirim', () => {
    const { f, el } = mount([topup()]);
    button(el, 'Tolak').click();
    f.detectChanges();
    submit(el);
    f.detectChanges();
    ctl().expectNone('/api/admin/topups/t1/reject');
    expect(el.textContent).toContain('Wajib diisi');
    type(el, 'note', 'transfer tidak ditemukan');
    submit(el);
    const req = ctl().expectOne('/api/admin/topups/t1/reject');
    expect(req.request.body).toEqual({ note: 'transfer tidak ditemukan' });
    req.flush({});
    ctl().expectOne((r) => r.url === '/api/admin/topups').flush({ topups: [] });
  });

  it('galat server (mis. memproses milik sendiri) tampil dan form tetap terbuka', () => {
    const { f, el } = mount([topup()]);
    button(el, 'Setujui').click();
    f.detectChanges();
    submit(el);
    ctl().expectOne('/api/admin/topups/t1/approve').flush({ error: 'admin tidak boleh memproses permintaannya sendiri' }, { status: 403, statusText: 'x' });
    f.detectChanges();
    expect(el.querySelector('[role=alert]')?.textContent).toContain('permintaannya sendiri');
    expect(el.querySelector('app-action-form')).toBeTruthy();
  });

  it('hanya yang belum diputuskan punya tombol; kedaluwarsa tetap bisa disetujui', () => {
    const { el } = mount([topup({ id: 'a', status: 'approved' }), topup({ id: 'b', status: 'expired' }), topup({ id: 'c', status: 'cancelled' })]);
    const cards = el.querySelectorAll('[data-testid=item]');
    expect(cards[0].querySelector('button')).toBeNull();
    expect(cards[1].textContent).toContain('Setujui');
    expect(cards[2].querySelector('button')).toBeNull();
  });

  it('ganti filter memuat ulang dengan status itu', () => {
    const { f, el } = mount([]);
    const sel = el.querySelector('select')!;
    sel.value = 'all';
    sel.dispatchEvent(new Event('change'));
    ctl().expectOne((r) => r.params.get('status') === 'all').flush({ topups: [topup({ status: 'approved' })] });
    f.detectChanges();
    expect(el.querySelectorAll('[data-testid=item]').length).toBe(1);
  });
});

describe('AdminWithdrawals', () => {
  afterEach(() => ctl().verify());
  function mount(list: AdminWithdrawal[]) {
    setup();
    const f = TestBed.createComponent(AdminWithdrawals);
    f.detectChanges();
    ctl().expectOne((r) => r.url === '/api/admin/withdrawals' && r.params.get('status') === 'pending').flush({ withdrawals: list });
    f.detectChanges();
    return { f, el: f.nativeElement as HTMLElement };
  }

  it('menampilkan rekening tujuan lengkap', () => {
    const { el } = mount([wd()]);
    expect(el.textContent).toContain('Rp200.000');
    expect(el.textContent).toContain('1234567890');
    expect(el.textContent).toContain('a.n. Budi');
  });

  it('menandai selesai wajib memakai referensi transfer', () => {
    const { f, el } = mount([wd()]);
    button(el, 'Sudah ditransfer').click();
    f.detectChanges();
    submit(el);
    f.detectChanges();
    ctl().expectNone('/api/admin/withdrawals/w1/complete');
    type(el, 'reference', 'REF-998877');
    submit(el);
    const req = ctl().expectOne('/api/admin/withdrawals/w1/complete');
    expect(req.request.body).toEqual({ reference: 'REF-998877', note: '' });
    req.flush({});
    ctl().expectOne((r) => r.url === '/api/admin/withdrawals').flush({ withdrawals: [] });
  });

  it('menolak wajib beralasan dan menjelaskan dana kembali', () => {
    const { f, el } = mount([wd()]);
    button(el, 'Tolak').click();
    f.detectChanges();
    expect(el.textContent).toContain('Dana dikembalikan');
    type(el, 'note', 'rekening tidak cocok');
    submit(el);
    const req = ctl().expectOne('/api/admin/withdrawals/w1/reject');
    expect(req.request.body).toEqual({ note: 'rekening tidak cocok' });
    req.flush({});
    ctl().expectOne((r) => r.url === '/api/admin/withdrawals').flush({ withdrawals: [] });
  });

  it('yang sudah selesai tidak punya tombol', () => {
    const { el } = mount([wd({ status: 'completed', transfer_reference: 'REF-1' })]);
    expect(el.querySelector('[data-testid=item] button')).toBeNull();
    expect(el.textContent).toContain('REF-1');
  });
});

describe('AdminKyc', () => {
  afterEach(() => ctl().verify());
  it('verifikasi dan penolakan (alasan wajib)', () => {
    setup();
    const f = TestBed.createComponent(AdminKyc);
    f.detectChanges();
    ctl().expectOne((r) => r.url === '/api/admin/kyc' && r.params.get('status') === 'pending').flush({ requests: [kyc()] });
    f.detectChanges();
    const el = f.nativeElement as HTMLElement;
    expect(el.textContent).toContain('ktp saya');

    button(el, 'Tolak').click();
    f.detectChanges();
    submit(el);
    f.detectChanges();
    ctl().expectNone('/api/admin/kyc/k1/reject');
    type(el, 'note', 'foto buram');
    submit(el);
    const rej = ctl().expectOne('/api/admin/kyc/k1/reject');
    expect(rej.request.body).toEqual({ note: 'foto buram' });
    rej.flush({});
    ctl().expectOne((r) => r.url === '/api/admin/kyc').flush({ requests: [kyc()] });
    f.detectChanges();

    button(el, 'Verifikasi').click();
    f.detectChanges();
    submit(el);
    const app = ctl().expectOne('/api/admin/kyc/k1/approve');
    expect(app.request.body).toEqual({ note: '' });
    app.flush({});
    ctl().expectOne((r) => r.url === '/api/admin/kyc').flush({ requests: [] });
  });
});

describe('AdminKyc status akhir', () => {
  afterEach(() => ctl().verify());
  it('yang sudah diverifikasi atau ditolak tidak punya tombol aksi', () => {
    setup();
    const f = TestBed.createComponent(AdminKyc);
    f.detectChanges();
    ctl().expectOne((r) => r.url === '/api/admin/kyc').flush({ requests: [kyc({ id: 'a', status: 'verified' }), kyc({ id: 'b', status: 'rejected', review_note: 'buram' })] });
    f.detectChanges();
    const el = f.nativeElement as HTMLElement;
    const cards = el.querySelectorAll('[data-testid=item]');
    expect(cards.length).toBe(2);
    cards.forEach((c) => expect(c.querySelector('button')).toBeNull());
    expect(el.textContent).toContain('Terverifikasi');
    expect(el.textContent).toContain('buram');
  });
});

describe('percentToBps', () => {
  it.each([['2.5', 250], ['2,5', 250], ['0', 0], ['10', 1000], ['12.34', 1234], ['0.07', 7], ['1.15', 115]])('%s%% -> %d bps', (t, want) => {
    expect(percentToBps(t as string)).toBe(want);
  });
});

describe('AdminSettingsPage', () => {
  afterEach(() => ctl().verify());
  const settings = { platform_fee_bps: 250, topup_min_amount: 10_000, topup_max_amount: 100_000_000, withdraw_min_amount: 50_000, topup_expiry_hours: 24 };
  const bank = (o = {}) => ({ id: 'b1', bank_name: 'BCA', account_number: '1234567890', account_holder: 'PT Geo', notes: '', is_active: true, ...o });

  function mount(banks = [bank()]) {
    setup();
    const f = TestBed.createComponent(AdminSettingsPage);
    f.detectChanges();
    ctl().expectOne('/api/admin/settings').flush(settings);
    ctl().expectOne('/api/admin/bank-accounts').flush({ bank_accounts: banks });
    f.detectChanges();
    return { f, el: f.nativeElement as HTMLElement };
  }

  it('menampilkan pengaturan dan rekening', () => {
    const { el } = mount();
    expect(el.textContent).toContain('2.5%');
    expect(el.textContent).toContain('1234567890');
  });

  it('menyimpan pengaturan: persen diubah ke basis poin', () => {
    const { f, el } = mount();
    button(el, 'Ubah').click();
    f.detectChanges();
    type(el, 'fee', '3.5');
    submit(el);
    const req = ctl().expectOne((r) => r.method === 'PUT' && r.url === '/api/admin/settings');
    expect(req.request.body).toEqual({ ...settings, platform_fee_bps: 350 });
    req.flush({ ...settings, platform_fee_bps: 350 });
    f.detectChanges();
    expect(el.textContent).toContain('3.5%');
    expect(el.textContent).toContain('Pengaturan disimpan');
  });

  it('biaya di atas 10% ditolak di klien', () => {
    const { f, el } = mount();
    button(el, 'Ubah').click();
    f.detectChanges();
    type(el, 'fee', '11');
    submit(el);
    f.detectChanges();
    ctl().expectNone((r) => r.method === 'PUT');
    expect(el.textContent).toContain('Maksimal 10.');
  });

  it('menambah rekening dan memuat ulang daftar', () => {
    const { f, el } = mount([]);
    expect(el.textContent).toContain('Belum ada rekening');
    button(el, 'Tambah rekening').click();
    f.detectChanges();
    type(el, 'bank_name', 'BCA');
    type(el, 'account_number', '1234567890');
    type(el, 'account_holder', 'PT Geo');
    submit(el);
    const req = ctl().expectOne((r) => r.method === 'POST' && r.url === '/api/admin/bank-accounts');
    expect(req.request.body).toEqual({ bank_name: 'BCA', account_number: '1234567890', account_holder: 'PT Geo', notes: '', is_active: true });
    req.flush(bank(), { status: 201, statusText: 'Created' });
    ctl().expectOne('/api/admin/bank-accounts').flush({ bank_accounts: [bank()] });
    f.detectChanges();
    expect(el.textContent).toContain('Rekening ditambahkan');
  });

  it('nomor rekening bukan angka ditolak di klien', () => {
    const { f, el } = mount([]);
    button(el, 'Tambah rekening').click();
    f.detectChanges();
    type(el, 'bank_name', 'BCA');
    type(el, 'account_number', '12-34');
    type(el, 'account_holder', 'PT Geo');
    submit(el);
    f.detectChanges();
    ctl().expectNone((r) => r.method === 'POST');
    expect(el.textContent).toContain('5 sampai 30 digit angka');
  });

  it('menonaktifkan rekening mempertahankan data lain dan membalik is_active', () => {
    const { f, el } = mount();
    button(el, 'Nonaktifkan').click();
    const req = ctl().expectOne((r) => r.method === 'PUT' && r.url === '/api/admin/bank-accounts/b1');
    expect(req.request.body).toEqual({ bank_name: 'BCA', account_number: '1234567890', account_holder: 'PT Geo', notes: '', is_active: false });
    req.flush(bank({ is_active: false }));
    ctl().expectOne('/api/admin/bank-accounts').flush({ bank_accounts: [bank({ is_active: false })] });
    f.detectChanges();
    expect(el.textContent).toContain('Nonaktif');
    expect(button(el, 'Aktifkan')).toBeTruthy();
  });

  it('rekening ganda dari server tampil pada form', () => {
    const { f, el } = mount([]);
    button(el, 'Tambah rekening').click();
    f.detectChanges();
    type(el, 'bank_name', 'BCA');
    type(el, 'account_number', '1234567890');
    type(el, 'account_holder', 'PT Geo');
    submit(el);
    ctl().expectOne((r) => r.method === 'POST').flush({ error: 'rekening dengan bank dan nomor yang sama sudah ada' }, { status: 409, statusText: 'x' });
    f.detectChanges();
    expect(el.querySelector('[role=alert]')?.textContent).toContain('sudah ada');
  });
});

describe('AdminReferralRules', () => {
  afterEach(() => ctl().verify());
  function mount() {
    setup();
    const f = TestBed.createComponent(AdminReferralRules);
    f.detectChanges();
    ctl().expectOne('/api/admin/referral/rules').flush({ rules: [{ level: 1, share_bps: 4000 }, { level: 2, share_bps: 2500 }, { level: 3, share_bps: 1500 }, { level: 4, share_bps: 1000 }] });
    f.detectChanges();
    const el = f.nativeElement as HTMLElement;
    const set = (i: number, v: string) => {
      const inp = el.querySelectorAll<HTMLInputElement>('input[type=number]')[i];
      inp.value = v;
      inp.dispatchEvent(new Event('input'));
      f.detectChanges();
    };
    return { f, el, set };
  }

  it('menampilkan nilai dalam persen, total 90%, platform menyimpan 10%', () => {
    const { el } = mount();
    expect(Array.from(el.querySelectorAll<HTMLInputElement>('input')).map((i) => i.value)).toEqual(['40', '25', '15', '10']);
    expect(el.textContent).toContain('Total: 90%');
    expect(el.textContent).toContain('platform menyimpan 10%');
  });

  it('total melebihi 100% menonaktifkan simpan dan tidak mengirim', () => {
    const { f, el, set } = mount();
    set(0, '60');
    expect(el.textContent).toContain('melebihi 100%');
    const btn = button(el, 'Simpan aturan');
    expect(btn.disabled).toBe(true);
    el.querySelector('form')!.dispatchEvent(new Event('submit'));
    ctl().expectNone((r) => r.method === 'PUT');
    f.detectChanges();
  });

  it('tepat 100% boleh; menyimpan memakai basis poin termasuk desimal', () => {
    const { f, el, set } = mount();
    set(0, '50');
    set(1, '30');
    set(2, '15.5');
    set(3, '4.5');
    expect(el.textContent).toContain('Total: 100%');
    el.querySelector('form')!.dispatchEvent(new Event('submit'));
    const req = ctl().expectOne((r) => r.method === 'PUT');
    expect(req.request.body).toEqual({ rules: [{ level: 1, share_bps: 5000 }, { level: 2, share_bps: 3000 }, { level: 3, share_bps: 1550 }, { level: 4, share_bps: 450 }] });
    req.flush({ rules: req.request.body.rules });
    f.detectChanges();
    expect(el.textContent).toContain('Aturan komisi disimpan');
  });

  it('isian kosong atau negatif tidak valid', () => {
    const { el, set } = mount();
    set(1, '');
    expect(button(el, 'Simpan aturan').disabled).toBe(true);
    set(1, '-5');
    expect(button(el, 'Simpan aturan').disabled).toBe(true);
    expect(el.textContent).toContain('Isi 0 sampai 100');
  });

  it('penolakan server tampil', () => {
    const { f, el } = mount();
    el.querySelector('form')!.dispatchEvent(new Event('submit'));
    ctl().expectOne((r) => r.method === 'PUT').flush({ error: 'total bagian komisi melebihi 100%' }, { status: 400, statusText: 'x' });
    f.detectChanges();
    expect(el.querySelector('[role=alert]')?.textContent).toContain('melebihi 100%');
  });
});

describe('AdminReputationLevels', () => {
  afterEach(() => ctl().verify());
  const levels: ReputationLevel[] = [
    { rank: 1, code: 'baru', name: 'Baru', require_verified: false, min_completed_cycles: 0, max_late_count: null, draw_weight: 1, can_create_group: true, max_group_value: 500_000 },
    { rank: 2, code: 'terpercaya', name: 'Terpercaya', require_verified: true, min_completed_cycles: 1, max_late_count: 0, draw_weight: 2, can_create_group: true, max_group_value: 5_000_000 },
    { rank: 3, code: 'senior', name: 'Senior', require_verified: true, min_completed_cycles: 3, max_late_count: null, draw_weight: 3, can_create_group: true, max_group_value: null },
  ];
  function mount() {
    setup();
    const f = TestBed.createComponent(AdminReputationLevels);
    f.detectChanges();
    ctl().expectOne('/api/admin/reputation/levels').flush({ levels });
    f.detectChanges();
    return { f, el: f.nativeElement as HTMLElement };
  }

  it('menyimpan tanpa perubahan mengirim ulang data yang sama (kosong = null)', () => {
    const { f, el } = mount();
    submit(el);
    const req = ctl().expectOne((r) => r.method === 'PUT');
    expect(req.request.body).toEqual({ levels });
    expect(req.request.body.levels[2].max_group_value).toBeNull();
    expect(req.request.body.levels[0].max_late_count).toBeNull();
    req.flush({ levels });
    f.detectChanges();
    expect(el.textContent).toContain('Level disimpan');
  });

  it('level dasar tidak menampilkan syarat; level lain menampilkannya', () => {
    const { el } = mount();
    const sets = el.querySelectorAll('fieldset');
    expect(sets.length).toBe(3);
    expect(sets[0].textContent).toContain('tanpa syarat');
    expect(sets[1].textContent).toContain('Arisan tuntas minimal');
  });

  it('bobot di luar 1–3 ditolak di klien', () => {
    const { f, el } = mount();
    const w = el.querySelectorAll<HTMLInputElement>('input[formControlName=draw_weight]')[1];
    w.value = '5';
    w.dispatchEvent(new Event('input'));
    submit(el);
    f.detectChanges();
    ctl().expectNone((r) => r.method === 'PUT');
    expect(el.textContent).toContain('Isi 1 sampai 3');
  });

  it('galat konsistensi dari server tampil', () => {
    const { f, el } = mount();
    submit(el);
    ctl().expectOne((r) => r.method === 'PUT').flush({ error: 'level 3 tidak boleh berbobot lebih kecil dari level 2' }, { status: 400, statusText: 'x' });
    f.detectChanges();
    expect(el.querySelector('[role=alert]')?.textContent).toContain('tidak boleh berbobot lebih kecil');
  });
});

describe('AdminOverview', () => {
  afterEach(() => ctl().verify());
  it('ledger konsisten', () => {
    setup();
    const f = TestBed.createComponent(AdminOverview);
    f.detectChanges();
    ctl().expectOne('/api/admin/wallet/reconcile').flush({ ok: true, problems: null, wallets_checked: 12, entries_checked: 80 });
    f.detectChanges();
    expect((f.nativeElement as HTMLElement).textContent).toContain('Konsisten');
    expect((f.nativeElement as HTMLElement).textContent).toContain('12 dompet');
  });

  it('ledger bermasalah menampilkan daftar masalah', () => {
    setup();
    const f = TestBed.createComponent(AdminOverview);
    f.detectChanges();
    ctl().expectOne('/api/admin/wallet/reconcile').flush({ ok: false, problems: ['saldo dompet X tidak cocok'], wallets_checked: 1, entries_checked: 1 });
    f.detectChanges();
    const t = (f.nativeElement as HTMLElement).textContent;
    expect(t).toContain('Ada masalah');
    expect(t).toContain('saldo dompet X tidak cocok');
  });
});
