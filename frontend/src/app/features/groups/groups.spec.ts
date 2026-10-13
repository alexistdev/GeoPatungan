import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { TestBed } from '@angular/core/testing';
import { Router, provideRouter } from '@angular/router';

import { AuthService } from '../../core/auth';
import { GroupService } from '../../core/groups';
import { Draw, Group, Member, MyContribution, Round, User } from '../../core/models';
import { DrawPanel } from './draw-panel';
import { GroupCreate, tomorrow } from './group-create';
import { GroupDetail } from './group-detail';
import { GroupJoin } from './group-join';
import { GroupList } from './group-list';
import { MembersPanel } from './members-panel';
import { RoundsPanel } from './rounds-panel';

const setup = () => TestBed.configureTestingModule({ providers: [provideRouter([]), provideHttpClient(), provideHttpClientTesting()] });
const ctl = () => TestBed.inject(HttpTestingController);

const group = (o: Partial<Group> = {}): Group => ({
  id: 'g1', owner_id: 'owner', name: 'Arisan Uji', contribution_amount: 50_000, period_type: 'monthly', slot_count: 3, start_date: '2026-10-14',
  late_fee: 5000, draw_method: 'random', min_level_rank: 1, status: 'open', invite_code: 'KODE1234', created_at: '', active_members: 3, my_status: 'active', ...o,
});
const member = (id: string, name: string, o: Partial<Member> = {}): Member => ({
  id, group_id: 'g1', user_id: 'u-' + id, user_name: name, turn_order: null, status: 'active', joined_at: '2026-10-12T10:00:00+07:00', ...o,
});
const members3 = [member('m1', 'Andi', { user_id: 'owner' }), member('m2', 'Budi'), member('m3', 'Cici')];

const type = (el: HTMLElement, name: string, v: string) => {
  const i = el.querySelector<HTMLInputElement>(`input[formControlName=${name}]`)!;
  i.value = v;
  i.dispatchEvent(new Event('input'));
};
const submit = (el: HTMLElement) => el.querySelector('form')!.dispatchEvent(new Event('submit'));
const button = (el: HTMLElement, text: string) =>
  Array.from(el.querySelectorAll('button')).find((b) => b.textContent?.includes(text)) as HTMLButtonElement;

describe('GroupService', () => {
  afterEach(() => ctl().verify());
  it('quote: respons bersarang {quote, balance, sufficient} digabung menjadi satu objek', () => {
    setup();
    let got: unknown;
    TestBed.inject(GroupService).quote('c1').subscribe((q) => (got = q));
    ctl().expectOne('/api/contributions/c1/quote').flush({
      quote: { contribution_id: 'c1', total: 50_000, payable: true, status: 'unpaid' }, balance: 500_971, sufficient: true,
    });
    expect(got).toMatchObject({ contribution_id: 'c1', total: 50_000, payable: true, balance: 500_971, sufficient: true });
  });

  it('draw: 404 berarti belum ada kocokan (null), galat lain tetap dilempar', () => {
    setup();
    const svc = TestBed.inject(GroupService);
    let got: unknown = 'x';
    svc.draw('g1').subscribe((d) => (got = d));
    ctl().expectOne('/api/groups/g1/draw').flush({ error: 'belum ada kocokan' }, { status: 404, statusText: 'x' });
    expect(got).toBeNull();
    let failed = false;
    svc.draw('g1').subscribe({ error: () => (failed = true) });
    ctl().expectOne('/api/groups/g1/draw').flush({}, { status: 500, statusText: 'x' });
    expect(failed).toBe(true);
  });
});

describe('tomorrow', () => {
  it('besok dalam format YYYY-MM-DD, melewati batas bulan dan tahun', () => {
    expect(tomorrow(new Date(2026, 9, 13))).toBe('2026-10-14');
    expect(tomorrow(new Date(2026, 9, 31))).toBe('2026-11-01');
    expect(tomorrow(new Date(2026, 11, 31))).toBe('2027-01-01');
  });
});

describe('GroupList', () => {
  afterEach(() => ctl().verify());
  it('daftar grup, keadaan kosong, dan status menunggu', () => {
    setup();
    const f = TestBed.createComponent(GroupList);
    f.detectChanges();
    ctl().expectOne('/api/groups/').flush({ groups: [group(), group({ id: 'g2', name: 'Lain', my_status: 'pending', status: 'open' })] });
    f.detectChanges();
    const t = (f.nativeElement as HTMLElement).textContent!;
    expect(t).toContain('Arisan Uji');
    expect(t).toContain('Rp50.000');
    expect(t).toContain('Menunggu persetujuan pemilik');
  });

  it('tombol Buat grup hanya untuk admin', () => {
    const render = (role: 'admin' | 'member') => {
      TestBed.resetTestingModule();
      setup();
      TestBed.inject(AuthService).user.set({ id: 'u', role } as User);
      const f = TestBed.createComponent(GroupList);
      f.detectChanges();
      ctl().expectOne('/api/groups/').flush({ groups: [] });
      f.detectChanges();
      return f.nativeElement as HTMLElement;
    };
    expect(render('admin').querySelector('a[href="/grup/baru"]')).not.toBeNull();
    const member = render('member');
    expect(member.querySelector('a[href="/grup/baru"]')).toBeNull();
    expect(member.querySelector('a[href="/grup/gabung"]')).not.toBeNull();
  });

  it('kosong dan galat', () => {
    setup();
    const f = TestBed.createComponent(GroupList);
    f.detectChanges();
    ctl().expectOne('/api/groups/').flush({}, { status: 500, statusText: 'x' });
    f.detectChanges();
    expect((f.nativeElement as HTMLElement).querySelector('[role=alert]')?.textContent).toContain('server');
  });

  it('belum punya grup', () => {
    setup();
    const f = TestBed.createComponent(GroupList);
    f.detectChanges();
    ctl().expectOne('/api/groups/').flush({ groups: [] });
    f.detectChanges();
    expect((f.nativeElement as HTMLElement).textContent).toContain('belum bergabung');
  });
});

describe('GroupCreate', () => {
  afterEach(() => ctl().verify());
  function mount() {
    setup();
    const f = TestBed.createComponent(GroupCreate);
    f.detectChanges();
    const nav = vi.spyOn(TestBed.inject(Router), 'navigate').mockResolvedValue(true);
    return { f, el: f.nativeElement as HTMLElement, nav };
  }

  it('menampilkan dana per putaran dari iuran × slot', () => {
    const { f, el } = mount();
    type(el, 'contribution_amount', '50000');
    type(el, 'slot_count', '4');
    f.detectChanges();
    expect(el.textContent).toContain('Rp200.000');
  });

  it('validasi klien: nama pendek, iuran di luar batas, slot, denda melebihi iuran', () => {
    const { f, el } = mount();
    type(el, 'name', 'ab');
    type(el, 'contribution_amount', '5000');
    type(el, 'slot_count', '1');
    type(el, 'late_fee', '99999999');
    submit(el);
    f.detectChanges();
    ctl().expectNone((r) => r.method === 'POST');
    expect(el.textContent).toContain('3 sampai 100 karakter');
    expect(el.textContent).toContain('Rp10.000 sampai Rp100.000.000');
    expect(el.textContent).toContain('2 sampai 50');
  });

  it('denda lebih besar dari iuran ditolak walau kolom lain benar', () => {
    const { f, el } = mount();
    type(el, 'name', 'Arisan Uji');
    type(el, 'contribution_amount', '50000');
    type(el, 'late_fee', '60000');
    submit(el);
    f.detectChanges();
    ctl().expectNone((r) => r.method === 'POST');
    expect(el.textContent).toContain('Denda tidak boleh melebihi iuran');
  });

  it('tanggal mulai di masa lalu ditolak', () => {
    const { f, el } = mount();
    type(el, 'name', 'Arisan Uji');
    type(el, 'start_date', '2020-01-01');
    submit(el);
    f.detectChanges();
    ctl().expectNone((r) => r.method === 'POST');
    expect(el.textContent).toContain('Paling cepat besok');
  });

  it('mengirim data bersih lalu membuka detail grup', () => {
    const { el, nav } = mount();
    type(el, 'name', '  Arisan Uji ');
    type(el, 'contribution_amount', '50000');
    type(el, 'slot_count', '3');
    submit(el);
    const req = ctl().expectOne((r) => r.method === 'POST' && r.url === '/api/groups/');
    expect(req.request.body).toEqual({
      name: 'Arisan Uji', contribution_amount: 50000, period_type: 'monthly', slot_count: 3, start_date: tomorrow(),
      late_fee: 5000, draw_method: 'random', min_level_rank: 1,
    });
    expect(typeof req.request.body.min_level_rank).toBe('number');
    req.flush(group({ id: 'baru' }), { status: 201, statusText: 'Created' });
    expect(nav).toHaveBeenCalledWith(['/grup', 'baru']);
  });

  it('pesan server (mis. melebihi batas level) tampil dan tidak berpindah halaman', () => {
    const { f, el, nav } = mount();
    type(el, 'name', 'Arisan Uji');
    submit(el);
    ctl().expectOne((r) => r.method === 'POST').flush({ error: 'nilai grup melebihi batas level Anda' }, { status: 403, statusText: 'x' });
    f.detectChanges();
    expect(el.querySelector('[role=alert]')?.textContent).toContain('melebihi batas level');
    expect(nav).not.toHaveBeenCalled();
    expect(el.querySelector<HTMLButtonElement>('button[type=submit]')!.disabled).toBe(false);
  });

  it('klik ganda tidak membuat dua grup', () => {
    const { el } = mount();
    type(el, 'name', 'Arisan Uji');
    submit(el);
    submit(el);
    ctl().expectOne((r) => r.method === 'POST');
  });
});

describe('GroupJoin', () => {
  afterEach(() => ctl().verify());
  it('kode dari ?kode= terisi; berhasil menampilkan konfirmasi menunggu persetujuan', () => {
    setup();
    const f = TestBed.createComponent(GroupJoin);
    f.componentRef.setInput('kode', 'KODE1234');
    f.detectChanges();
    const el = f.nativeElement as HTMLElement;
    expect(el.querySelector<HTMLInputElement>('input')!.value).toBe('KODE1234');
    submit(el);
    const req = ctl().expectOne('/api/groups/join');
    expect(req.request.body).toEqual({ invite_code: 'KODE1234' });
    req.flush(group({ name: 'Arisan Uji', my_status: 'pending' }), { status: 201, statusText: 'Created' });
    f.detectChanges();
    expect(el.textContent).toContain('Arisan Uji');
    expect(el.textContent).toContain('menyetujui permintaan');
  });

  it('kode kosong tidak dikirim; kode salah menampilkan pesan server', () => {
    setup();
    const f = TestBed.createComponent(GroupJoin);
    f.detectChanges();
    const el = f.nativeElement as HTMLElement;
    submit(el);
    f.detectChanges();
    ctl().expectNone('/api/groups/join');
    expect(el.textContent).toContain('Masukkan kode undangan');
    type(el, 'code', ' SALAH123 ');
    submit(el);
    const req = ctl().expectOne('/api/groups/join');
    expect(req.request.body).toEqual({ invite_code: 'SALAH123' });
    req.flush({ error: 'kode undangan tidak ditemukan' }, { status: 404, statusText: 'x' });
    f.detectChanges();
    expect(el.querySelector('[role=alert]')?.textContent).toContain('kode undangan tidak ditemukan');
  });
});

describe('MembersPanel', () => {
  afterEach(() => ctl().verify());
  function mount(ms: Member[], o: { owner?: boolean; g?: Group } = {}) {
    setup();
    const f = TestBed.createComponent(MembersPanel);
    f.componentRef.setInput('group', o.g ?? group({ active_members: 1 }));
    f.componentRef.setInput('members', ms);
    f.componentRef.setInput('isOwner', o.owner ?? true);
    const changed: number[] = [];
    f.componentInstance.changed.subscribe(() => changed.push(1));
    f.detectChanges();
    return { f, el: f.nativeElement as HTMLElement, changed };
  }
  const pend = [member('p1', 'Budi', { status: 'pending' }), member('p2', 'Cici', { status: 'pending' })];

  it('pemilik melihat permintaan dan menerima; non-pemilik tidak melihat tombol', () => {
    const a = mount([member('m1', 'Andi', { user_id: 'owner' }), ...pend]);
    expect(a.el.textContent).toContain('2 permintaan bergabung');
    button(a.el, 'Terima').click();
    ctl().expectOne('/api/groups/g1/members/p1/approve').flush({ status: 'active' });
    expect(a.changed.length).toBe(1);
    TestBed.resetTestingModule();
    const b = mount([member('m1', 'Andi', { user_id: 'owner' }), ...pend], { owner: false });
    expect(b.el.textContent).not.toContain('permintaan bergabung');
    expect(button(b.el, 'Terima')).toBeUndefined();
  });

  it('menolak memanggil endpoint reject', () => {
    const { el, changed } = mount([member('m1', 'Andi', { user_id: 'owner' }), ...pend]);
    button(el, 'Tolak').click();
    ctl().expectOne('/api/groups/g1/members/p1/reject').flush({ status: 'removed' });
    expect(changed.length).toBe(1);
  });

  it('slot penuh: tombol Terima nonaktif dengan penjelasan', () => {
    const { el } = mount([member('m1', 'A', { user_id: 'owner' }), member('m2', 'B'), member('m3', 'C'), member('p1', 'D', { status: 'pending' })], { g: group({ active_members: 3 }) });
    expect(button(el, 'Terima').disabled).toBe(true);
    expect(el.textContent).toContain('Slot sudah penuh');
  });

  it('galat server tampil (mis. grup penuh)', () => {
    const { f, el } = mount([member('m1', 'Andi', { user_id: 'owner' }), ...pend]);
    button(el, 'Terima').click();
    ctl().expectOne('/api/groups/g1/members/p1/approve').flush({ error: 'slot grup sudah penuh' }, { status: 409, statusText: 'x' });
    f.detectChanges();
    expect(el.querySelector('[role=alert]')?.textContent).toContain('sudah penuh');
  });

  it('hanya anggota aktif dihitung; pemilik diberi lencana', () => {
    const { el } = mount([member('m1', 'Andi', { user_id: 'owner' }), member('x', 'Dewi', { status: 'removed' }), ...pend]);
    expect(el.textContent).toContain('Anggota (1/3)');
    expect(el.textContent).toContain('Pemilik');
    const lists = el.querySelectorAll('ul.list-group-flush');
    expect(lists[lists.length - 1]?.textContent).not.toContain('Dewi');
  });
});

describe('DrawPanel', () => {
  afterEach(() => ctl().verify());
  const committed: Draw = { id: 'd1', group_id: 'g1', algorithm: 'weighted_es_v1', seed_commit: 'abc123hash', committed_at: '2026-10-12T10:00:00+07:00', executed_at: null, entries: [] };
  const done: Draw = {
    ...committed, seed_reveal: 'seedrevealed', executed_at: '2026-10-12T11:00:00+07:00',
    entries: [
      { member_id: 'm3', user_name: 'Cici', weight: 1, turn_order: 2 },
      { member_id: 'm1', user_name: 'Andi', weight: 2, turn_order: 1 },
      { member_id: 'm2', user_name: 'Budi', weight: 1, turn_order: 3 },
    ],
  };

  function mount(draw: Draw | null, o: { owner?: boolean; g?: Group; ms?: Member[] } = {}) {
    setup();
    const f = TestBed.createComponent(DrawPanel);
    f.componentRef.setInput('group', o.g ?? group());
    f.componentRef.setInput('members', o.ms ?? members3);
    f.componentRef.setInput('isOwner', o.owner ?? true);
    const changed: number[] = [];
    f.componentInstance.changed.subscribe(() => changed.push(1));
    f.detectChanges();
    if (draw) ctl().expectOne('/api/groups/g1/draw').flush(draw);
    else ctl().expectOne('/api/groups/g1/draw').flush({ error: 'belum ada kocokan' }, { status: 404, statusText: 'x' });
    f.detectChanges();
    return { f, el: f.nativeElement as HTMLElement, changed };
  }

  it('belum ada kocokan: pemilik mengunci (commit) lalu melihat hash dan tombol jalankan', () => {
    const { f, el, changed } = mount(null);
    button(el, 'Kunci kocokan').click();
    ctl().expectOne('/api/groups/g1/draw/commit').flush(committed, { status: 201, statusText: 'Created' });
    f.detectChanges();
    expect(el.textContent).toContain('abc123hash');
    expect(el.textContent).toContain('Kocokan sudah dikunci');
    expect(changed.length).toBe(1);
    button(el, 'Jalankan kocokan').click();
    ctl().expectOne('/api/groups/g1/draw/execute').flush(done);
    f.detectChanges();
    expect(el.textContent).toContain('seedrevealed');
  });

  it('grup belum penuh: commit tidak ditawarkan dan ada penjelasan', () => {
    const { el } = mount(null, { g: group({ active_members: 2 }), ms: [members3[0], members3[1]] });
    expect(button(el, 'Kunci kocokan')).toBeUndefined();
    expect(el.textContent).toContain('harus penuh (2/3)');
  });

  it('non-pemilik tidak punya tombol; hanya melihat hash bila sudah dikunci', () => {
    const a = mount(null, { owner: false });
    expect(a.el.querySelector('button')).toBeNull();
    expect(a.el.textContent).toContain('pemilik grup');
    TestBed.resetTestingModule();
    const b = mount(committed, { owner: false });
    expect(b.el.textContent).toContain('abc123hash');
    expect(b.el.querySelector('button')).toBeNull();
  });

  it('hasil: diurutkan menurut giliran, menampilkan bobot dan bukti verifikasi', () => {
    const { el } = mount(done, { g: group({ status: 'running' }) });
    const items = Array.from(el.querySelectorAll('ol li')).map((li) => li.textContent!.replace(/\s+/g, ' ').trim());
    expect(items[0]).toContain('1. Andi');
    expect(items[1]).toContain('2. Cici');
    expect(items[2]).toContain('3. Budi');
    expect(items[0]).toContain('bobot 2');
    expect(el.textContent).toContain('Verifikasi kocokan');
    expect(el.textContent).toContain('weighted_es_v1');
  });

  it('manual dan grup belum penuh: peringatan, tanpa editor urutan dan tanpa tombol tetapkan', () => {
    const { el } = mount(null, { g: group({ draw_method: 'manual', active_members: 2 }), ms: [members3[0], members3[1]] });
    expect(el.textContent).toContain('harus penuh (2/3)');
    expect(el.textContent).toContain('sebelum urutan ditetapkan');
    expect(button(el, 'Tetapkan urutan')).toBeUndefined();
    expect(el.querySelector('button[aria-label^="Naikkan"]')).toBeNull();
  });

  it('manual: urutan awal sesuai bergabung, naik/turun, lalu ditetapkan', () => {
    const { f, el, changed } = mount(null, { g: group({ draw_method: 'manual' }) });
    const names = () => Array.from(el.querySelectorAll('ol li')).map((li) => li.textContent!.replace(/\s+/g, ' ').trim().slice(0, 8));
    expect(names()[0]).toContain('Andi');
    // turunkan Andi satu langkah: Budi, Andi, Cici
    el.querySelectorAll<HTMLButtonElement>('button[aria-label^="Turunkan"]')[0].click();
    f.detectChanges();
    expect(names()[0]).toContain('Budi');
    expect(names()[1]).toContain('Andi');
    // tombol batas: yang pertama tidak bisa naik, yang terakhir tidak bisa turun
    expect(el.querySelectorAll<HTMLButtonElement>('button[aria-label^="Naikkan"]')[0].disabled).toBe(true);
    const downs = el.querySelectorAll<HTMLButtonElement>('button[aria-label^="Turunkan"]');
    expect(downs[downs.length - 1].disabled).toBe(true);
    button(el, 'Tetapkan urutan').click();
    const req = ctl().expectOne('/api/groups/g1/draw/manual');
    expect(req.request.body).toEqual({ order: ['m2', 'm1', 'm3'] });
    req.flush({
      ...done, algorithm: 'manual', seed_commit: undefined,
      entries: [
        { member_id: 'm2', user_name: 'Budi', weight: 1, turn_order: 1 },
        { member_id: 'm1', user_name: 'Andi', weight: 1, turn_order: 2 },
        { member_id: 'm3', user_name: 'Cici', weight: 1, turn_order: 3 },
      ],
    }, { status: 201, statusText: 'Created' });
    f.detectChanges();
    expect(changed.length).toBe(1);
    expect(el.textContent).toContain('1. Budi');
    expect(el.textContent).not.toContain('Verifikasi kocokan');
  });

  it('galat server tampil dan tombol aktif kembali', () => {
    const { f, el } = mount(null);
    button(el, 'Kunci kocokan').click();
    ctl().expectOne('/api/groups/g1/draw/commit').flush({ error: 'grup belum penuh' }, { status: 409, statusText: 'x' });
    f.detectChanges();
    expect(el.querySelector('[role=alert]')?.textContent).toContain('belum penuh');
    expect(button(el, 'Kunci kocokan').disabled).toBe(false);
  });
});

describe('RoundsPanel', () => {
  afterEach(() => ctl().verify());
  const round = (no: number, o: Partial<Round> = {}): Round => ({
    id: 'r' + no, group_id: 'g1', round_no: no, due_date: '2026-10-14', recipient_member_id: 'm1', recipient_name: 'Andi', status: 'collecting',
    paid_count: 1, member_count: 3, pot_amount: 150_000, ...o,
  });
  const mine = (no: number, o: Partial<MyContribution> = {}): MyContribution => ({
    id: 'c' + no, round_id: 'r' + no, round_no: no, due_date: '2026-10-14', amount: 50_000, platform_fee: 0, late_fee: 0, status: 'unpaid',
    paid_on_time: null, paid_at: null, overdue: false, group_id: 'g1', group_name: 'Arisan Uji', ...o,
  });
  const quote = (o = {}) => ({ quote: { contribution_id: 'c1', group_id: 'g1', round_id: 'r1', due_date: '2026-10-14', status: 'unpaid', round_status: 'collecting', amount: 50_000, platform_fee: 1000, late_fee: 0, total: 51_000, late: false, payable: true, ...o }, balance: 500_000, sufficient: true });

  function mount(rounds: Round[], contribs: MyContribution[]) {
    setup();
    const f = TestBed.createComponent(RoundsPanel);
    f.componentRef.setInput('group', group({ status: 'running' }));
    const changed: number[] = [];
    f.componentInstance.changed.subscribe(() => changed.push(1));
    f.detectChanges();
    ctl().expectOne('/api/groups/g1/rounds').flush({ rounds });
    ctl().expectOne('/api/me/contributions').flush({ contributions: contribs });
    f.detectChanges();
    return { f, el: f.nativeElement as HTMLElement, changed };
  }
  const reload = (rounds: Round[], contribs: MyContribution[]) => {
    ctl().expectOne('/api/groups/g1/rounds').flush({ rounds });
    ctl().expectOne('/api/me/contributions').flush({ contributions: contribs });
  };

  it('hanya iuran milik grup ini yang dipakai; tombol bayar hanya pada putaran pengumpulan', () => {
    const { el } = mount(
      [round(1), round(2, { status: 'upcoming', paid_count: 0 }), round(3, { status: 'paid_out', paid_count: 3 })],
      [mine(1), mine(2), mine(3, { status: 'paid' }), { ...mine(1), id: 'lain', round_id: 'rx', group_id: 'gX' }],
    );
    const items = el.querySelectorAll('li');
    expect(items[0].textContent).toContain('Bayar iuran');
    expect(items[1].textContent).not.toContain('Bayar iuran');
    expect(items[1].textContent).toContain('Belum dibuka');
    expect(items[2].textContent).toContain('Anda sudah bayar');
    expect(items[0].textContent).toContain('1 dari 3 sudah membayar');
  });

  it('bayar: lihat rincian, konfirmasi, tampilkan struk dan muat ulang', () => {
    const { f, el, changed } = mount([round(1)], [mine(1, { id: 'c1' })]);
    button(el, 'Bayar iuran').click();
    ctl().expectOne('/api/contributions/c1/quote').flush(quote());
    f.detectChanges();
    expect(el.textContent).toContain('Rp51.000');
    expect(el.textContent).toContain('Rp500.000');
    button(el, 'Bayar Rp51.000').click();
    ctl().expectOne((r) => r.method === 'POST' && r.url === '/api/contributions/c1/pay').flush({
      receipt: { contribution_id: 'c1', total: 51_000, paid_on_time: true }, balance: 449_000,
      round: { completed: true, group_done: false, payout: { recipient_name: 'Andi', amount: 150_000 } },
    });
    reload([round(1, { status: 'paid_out', paid_count: 3 })], [mine(1, { id: 'c1', status: 'paid' })]);
    f.detectChanges();
    expect(el.textContent).toContain('Pembayaran Rp51.000 berhasil');
    expect(el.textContent).toContain('Rp449.000');
    expect(el.textContent).toContain('Rp150.000');
    expect(el.textContent).toContain('dicairkan ke Andi');
    expect(el.textContent).toContain('Anda sudah bayar');
    expect(changed.length).toBe(1);
  });

  it('pembayaran terakhir grup menampilkan grup ditutup', () => {
    const { f, el } = mount([round(1)], [mine(1, { id: 'c1' })]);
    button(el, 'Bayar iuran').click();
    ctl().expectOne('/api/contributions/c1/quote').flush(quote());
    f.detectChanges();
    button(el, 'Bayar Rp51.000').click();
    ctl().expectOne((r) => r.method === 'POST').flush({
      receipt: { contribution_id: 'c1', total: 51_000, paid_on_time: false }, balance: 1,
      round: { completed: true, group_done: true, payout: { recipient_name: 'Andi', amount: 150_000 } },
    });
    reload([round(1, { status: 'paid_out' })], [mine(1, { id: 'c1', status: 'paid' })]);
    f.detectChanges();
    expect(el.textContent).toContain('terlambat');
    expect(el.textContent).toContain('grup ditutup');
  });

  it('saldo kurang menurut rincian: tombol bayar nonaktif dan ada tautan isi saldo', () => {
    const { f, el } = mount([round(1)], [mine(1, { id: 'c1' })]);
    button(el, 'Bayar iuran').click();
    ctl().expectOne('/api/contributions/c1/quote').flush({ ...quote(), balance: 10_000, sufficient: false });
    f.detectChanges();
    expect(el.textContent).toContain('Saldo kurang Rp41.000');
    expect(el.querySelector('a[href="/dompet/isi-saldo"]')).toBeTruthy();
    expect(button(el, 'Bayar Rp51.000').disabled).toBe(true);
  });

  it('tidak dapat dibayar: alasan dari server tampil dan tombol nonaktif', () => {
    const { f, el } = mount([round(1)], [mine(1, { id: 'c1' })]);
    button(el, 'Bayar iuran').click();
    ctl().expectOne('/api/contributions/c1/quote').flush(quote({ payable: false, reason: 'putaran belum dibuka' }));
    f.detectChanges();
    expect(el.textContent).toContain('putaran belum dibuka');
    expect(button(el, 'Bayar Rp51.000').disabled).toBe(true);
  });

  it('402 saat membayar menampilkan angka kebutuhan dan saldo', () => {
    const { f, el } = mount([round(1)], [mine(1, { id: 'c1' })]);
    button(el, 'Bayar iuran').click();
    ctl().expectOne('/api/contributions/c1/quote').flush(quote());
    f.detectChanges();
    button(el, 'Bayar Rp51.000').click();
    ctl().expectOne((r) => r.method === 'POST').flush({ error: 'saldo tidak cukup', required: 51_000, balance: 20_000 }, { status: 402, statusText: 'x' });
    f.detectChanges();
    expect(el.querySelector('[role=alert]')?.textContent).toContain('dibutuhkan Rp51.000, saldo Anda Rp20.000');
  });

  it('galat saat memuat rincian terlihat (tidak diam)', () => {
    const { f, el } = mount([round(1)], [mine(1, { id: 'c1' })]);
    button(el, 'Bayar iuran').click();
    ctl().expectOne('/api/contributions/c1/quote').flush({ error: 'iuran tidak ditemukan' }, { status: 404, statusText: 'x' });
    f.detectChanges();
    expect(el.querySelector('[role=alert]')?.textContent).toContain('iuran tidak ditemukan');
  });

  it('batal menutup rincian; klik ganda bayar hanya mengirim satu permintaan', () => {
    const { f, el } = mount([round(1)], [mine(1, { id: 'c1' })]);
    button(el, 'Bayar iuran').click();
    ctl().expectOne('/api/contributions/c1/quote').flush(quote());
    f.detectChanges();
    button(el, 'Batal').click();
    f.detectChanges();
    expect(el.textContent).not.toContain('Rincian pembayaran');
    button(el, 'Bayar iuran').click();
    ctl().expectOne('/api/contributions/c1/quote').flush(quote());
    f.detectChanges();
    const pay = button(el, 'Bayar Rp51.000');
    pay.click();
    pay.click();
    ctl().expectOne((r) => r.method === 'POST');
  });

  it('lihat pembayar: lunas, terlambat, menunggak, belum', () => {
    const { f, el } = mount([round(1)], [mine(1)]);
    button(el, 'Lihat pembayar').click();
    const c = (id: string, name: string, o = {}) => ({ id, round_id: 'r1', round_no: 1, due_date: '', member_id: id, user_name: name, amount: 1, platform_fee: 0, late_fee: 0, status: 'unpaid', paid_on_time: null, paid_at: null, overdue: false, ...o });
    ctl().expectOne('/api/groups/g1/rounds/r1/contributions').flush({
      contributions: [c('1', 'Andi', { status: 'paid', paid_on_time: true }), c('2', 'Budi', { status: 'paid', paid_on_time: false }), c('3', 'Cici', { overdue: true }), c('4', 'Dewi')],
    });
    f.detectChanges();
    const t = el.querySelector('.payers')!.textContent!;
    expect(t).toContain('Lunas');
    expect(t).toContain('Lunas (terlambat)');
    expect(t).toContain('Menunggak');
    expect(t).toContain('Belum');
    button(el, 'Sembunyikan pembayar').click();
    f.detectChanges();
    expect(el.querySelector('.payers')).toBeNull();
  });
});

describe('GroupDetail', () => {
  afterEach(() => ctl().verify());
  function mount(g: Group, userId: string, ms: Member[] = members3) {
    setup();
    TestBed.inject(AuthService).user.set({ id: userId } as User);
    const f = TestBed.createComponent(GroupDetail);
    f.componentRef.setInput('id', 'g1');
    f.detectChanges();
    ctl().expectOne('/api/groups/g1').flush(g);
    ctl().expectOne('/api/groups/g1/members').flush({ members: ms });
    f.detectChanges();
    return { f, el: f.nativeElement as HTMLElement };
  }

  it('pemilik melihat kode undangan dan panel; grup terbuka belum menampilkan putaran', () => {
    const { el } = mount(group(), 'owner');
    expect(el.textContent).toContain('KODE1234');
    expect(el.textContent).toContain('Salin tautan');
    expect(el.querySelector('app-members-panel')).toBeTruthy();
    expect(el.querySelector('app-draw-panel')).toBeTruthy();
    expect(el.querySelector('app-rounds-panel')).toBeNull();
    ctl().expectOne('/api/groups/g1/draw').flush({}, { status: 404, statusText: 'x' });
  });

  it('anggota biasa: tanpa kode undangan; grup berjalan menampilkan putaran', () => {
    const { f, el } = mount(group({ status: 'running', invite_code: undefined }), 'u-m2');
    expect(el.textContent).not.toContain('Kode undangan');
    expect(el.querySelector('app-rounds-panel')).toBeTruthy();
    ctl().expectOne('/api/groups/g1/draw').flush({}, { status: 404, statusText: 'x' });
    ctl().expectOne('/api/groups/g1/rounds').flush({ rounds: [] });
    ctl().expectOne('/api/me/contributions').flush({ contributions: [] });
    f.detectChanges();
  });

  it('kode undangan hanya untuk pemilik, walau data grup memuatnya', () => {
    const { el } = mount(group({ invite_code: 'RAHASIA99' }), 'u-m2');
    expect(el.textContent).not.toContain('RAHASIA99');
    expect(el.textContent).not.toContain('Salin tautan');
    ctl().expectOne('/api/groups/g1/draw').flush({}, { status: 404, statusText: 'x' });
  });

  it('calon anggota yang masih menunggu: hanya info dan pemberitahuan, tanpa panel', () => {
    const { el } = mount(group({ my_status: 'pending', invite_code: undefined }), 'orang-lain');
    expect(el.textContent).toContain('menunggu persetujuan pemilik');
    expect(el.querySelector('app-members-panel')).toBeNull();
    expect(el.querySelector('app-draw-panel')).toBeNull();
  });

  it('grup tidak ditemukan (bukan anggota) menampilkan pesan, bukan layar kosong', () => {
    setup();
    TestBed.inject(AuthService).user.set({ id: 'x' } as User);
    const f = TestBed.createComponent(GroupDetail);
    f.componentRef.setInput('id', 'g1');
    f.detectChanges();
    ctl().expectOne('/api/groups/g1').flush({ error: 'grup tidak ditemukan' }, { status: 404, statusText: 'x' });
    ctl().expectOne('/api/groups/g1/members').flush({}, { status: 404, statusText: 'x' });
    f.detectChanges();
    expect((f.nativeElement as HTMLElement).querySelector('[role=alert]')?.textContent).toContain('grup tidak ditemukan');
  });
});
