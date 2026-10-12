import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { TestBed } from '@angular/core/testing';

import { AppNotification, NotificationPage } from '../../core/models';
import { NotificationService } from '../../core/notifications';
import { Notifications } from './notifications';

const note = (id: number, read = false): AppNotification => ({
  id, type: 'x', title: `Judul ${id}`, body: `Isi ${id}`, data: {}, read, created_at: '2026-10-12T10:00:00+07:00',
});
const page = (items: AppNotification[], next: number | null, unread: number): NotificationPage => ({
  notifications: items, next_before: next, unread_count: unread,
});

function mount() {
  TestBed.configureTestingModule({ imports: [Notifications], providers: [provideHttpClient(), provideHttpClientTesting()] });
  const ctl = TestBed.inject(HttpTestingController);
  const fixture = TestBed.createComponent(Notifications);
  fixture.detectChanges();
  return { fixture, ctl, el: fixture.nativeElement as HTMLElement, svc: TestBed.inject(NotificationService) };
}

describe('Notifications', () => {
  afterEach(() => TestBed.inject(HttpTestingController).verify());

  it('menampilkan daftar dan memperbarui lencana belum dibaca', () => {
    const { fixture, ctl, el, svc } = mount();
    ctl.expectOne((r) => r.url === '/api/me/notifications/' && r.params.get('limit') === '20').flush(page([note(2), note(1, true)], null, 1));
    fixture.detectChanges();
    expect(el.textContent).toContain('Judul 2');
    expect(el.textContent).toContain('Judul 1');
    expect(svc.unread()).toBe(1);
    expect(el.querySelector('.dot')).toBeTruthy();
    expect(el.textContent).not.toContain('Muat lebih banyak');
  });

  it('keadaan kosong', () => {
    const { fixture, ctl, el } = mount();
    ctl.expectOne((r) => r.url === '/api/me/notifications/').flush(page([], null, 0));
    fixture.detectChanges();
    expect(el.textContent).toContain('Belum ada notifikasi');
  });

  it('muat lebih banyak memakai kursor dan menyambung daftar tanpa ganda', () => {
    const { fixture, ctl, el } = mount();
    ctl.expectOne((r) => r.url === '/api/me/notifications/').flush(page([note(3), note(2)], 2, 2));
    fixture.detectChanges();
    const more = Array.from(el.querySelectorAll('button')).find((b) => b.textContent?.includes('Muat lebih banyak'))!;
    more.click();
    const req = ctl.expectOne((r) => r.url === '/api/me/notifications/' && r.params.get('before') === '2');
    req.flush(page([note(1)], null, 2));
    fixture.detectChanges();
    expect(el.querySelectorAll('li').length).toBe(3);
    expect(el.textContent).not.toContain('Muat lebih banyak');
  });

  it('klik pemberitahuan menandai dibaca lalu menyegarkan hitungan', () => {
    const { fixture, ctl, el } = mount();
    ctl.expectOne((r) => r.url === '/api/me/notifications/').flush(page([note(5)], null, 1));
    fixture.detectChanges();
    el.querySelector<HTMLButtonElement>('.item')!.click();
    ctl.expectOne('/api/me/notifications/5/read').flush({ read: true });
    ctl.expectOne('/api/me/notifications/unread-count').flush({ unread_count: 0 });
    fixture.detectChanges();
    expect(el.querySelector('.dot')).toBeNull();
    // klik lagi pada yang sudah dibaca tidak memanggil server
    el.querySelector<HTMLButtonElement>('.item')!.click();
    ctl.expectNone('/api/me/notifications/5/read');
  });

  it('tandai semua dibaca', () => {
    const { fixture, ctl, el, svc } = mount();
    ctl.expectOne((r) => r.url === '/api/me/notifications/').flush(page([note(2), note(1)], null, 2));
    fixture.detectChanges();
    Array.from(el.querySelectorAll('button')).find((b) => b.textContent?.includes('Tandai semua'))!.click();
    ctl.expectOne('/api/me/notifications/read-all').flush({ updated: 2 });
    fixture.detectChanges();
    expect(el.querySelector('.dot')).toBeNull();
    expect(svc.unread()).toBe(0);
  });

  it('filter belum dibaca mengirim unread=true', () => {
    const { fixture, ctl, el } = mount();
    ctl.expectOne((r) => r.url === '/api/me/notifications/').flush(page([], null, 0));
    fixture.detectChanges();
    Array.from(el.querySelectorAll('button')).find((b) => b.textContent?.includes('Belum dibaca ('))!.click();
    ctl.expectOne((r) => r.params.get('unread') === 'true').flush(page([], null, 0));
    fixture.detectChanges();
    expect(el.textContent).toContain('Tidak ada yang belum dibaca');
  });

  it('galat server tampil sebagai pesan, bukan layar kosong', () => {
    const { fixture, ctl, el } = mount();
    ctl.expectOne((r) => r.url === '/api/me/notifications/').flush({}, { status: 500, statusText: 'x' });
    fixture.detectChanges();
    expect(el.querySelector('[role=alert]')?.textContent).toContain('server');
  });
});
