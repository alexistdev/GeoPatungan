import { Component } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';

import { Crumb, PageTitle } from './page-title';

@Component({
  imports: [PageTitle],
  template: `<app-page-title [title]="title" [crumbs]="crumbs" />`,
})
class Host {
  title = 'Isi saldo';
  crumbs: Crumb[] = [{ label: 'Dompet', link: '/dompet' }, { label: 'Tanpa tautan' }];
}

describe('PageTitle', () => {
  function mount(init?: (h: Host) => void) {
    TestBed.configureTestingModule({ providers: [provideRouter([])] });
    const f = TestBed.createComponent(Host);
    init?.(f.componentInstance);
    f.detectChanges();
    return f.nativeElement as HTMLElement;
  }

  it('menampilkan judul dan breadcrumb: GeoPatungan > tingkat bertautan > judul aktif', () => {
    const el = mount();
    expect(el.querySelector('h4')!.textContent).toBe('Isi saldo');
    const items = Array.from(el.querySelectorAll('.breadcrumb-item')).map((li) => li.textContent!.trim());
    expect(items).toEqual(['GeoPatungan', 'Dompet', 'Isi saldo']);
    expect(el.querySelector('.breadcrumb-item.active')!.textContent).toContain('Isi saldo');
  });

  it('tingkat tanpa tautan tidak ditampilkan', () => {
    expect(mount().textContent).not.toContain('Tanpa tautan');
  });

  it('tautan menuju rute yang benar', () => {
    const el = mount();
    const hrefs = Array.from(el.querySelectorAll('a')).map((a) => a.getAttribute('href'));
    expect(hrefs).toEqual(['/', '/dompet']);
  });

  it('tanpa crumbs hanya beranda dan judul', () => {
    const el = mount((h) => (h.crumbs = []));
    expect(Array.from(el.querySelectorAll('.breadcrumb-item')).length).toBe(2);
  });
});
