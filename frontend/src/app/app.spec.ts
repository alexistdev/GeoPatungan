import { TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';

import { App } from './app';

describe('App', () => {
  it('membuat aplikasi dengan outlet rute', async () => {
    await TestBed.configureTestingModule({ imports: [App], providers: [provideRouter([])] }).compileComponents();
    const fixture = TestBed.createComponent(App);
    await fixture.whenStable();
    expect((fixture.nativeElement as HTMLElement).querySelector('router-outlet')).toBeTruthy();
  });
});
