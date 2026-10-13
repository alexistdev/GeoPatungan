import { DatePipe } from '@angular/common';
import { Component, OnInit, inject, signal } from '@angular/core';

import { errorMessage } from '../../core/errors';
import { AppNotification } from '../../core/models';
import { NotificationService } from '../../core/notifications';
import { PageTitle } from '../../shared/page-title';

const PAGE_SIZE = 20;

@Component({
  selector: 'app-notifications',
  imports: [DatePipe, PageTitle],
  templateUrl: './notifications.html',
})
export class Notifications implements OnInit {
  protected readonly service = inject(NotificationService);

  protected readonly items = signal<AppNotification[]>([]);
  protected readonly next = signal<number | null>(null);
  protected readonly loading = signal(true);
  protected readonly loadingMore = signal(false);
  protected readonly error = signal<string | null>(null);
  protected readonly unreadOnly = signal(false);

  ngOnInit(): void {
    this.load();
  }

  protected setFilter(unreadOnly: boolean): void {
    this.unreadOnly.set(unreadOnly);
    this.load();
  }

  protected load(): void {
    this.loading.set(true);
    this.error.set(null);
    this.service.list({ limit: PAGE_SIZE, unreadOnly: this.unreadOnly() }).subscribe({
      next: (p) => {
        this.items.set(p.notifications);
        this.next.set(p.next_before);
        this.loading.set(false);
      },
      error: (e: unknown) => {
        this.error.set(errorMessage(e));
        this.loading.set(false);
      },
    });
  }

  protected more(): void {
    const before = this.next();
    if (before === null || this.loadingMore()) {
      return;
    }
    this.loadingMore.set(true);
    this.service.list({ limit: PAGE_SIZE, before, unreadOnly: this.unreadOnly() }).subscribe({
      next: (p) => {
        this.items.update((cur) => [...cur, ...p.notifications]);
        this.next.set(p.next_before);
        this.loadingMore.set(false);
      },
      error: (e: unknown) => {
        this.error.set(errorMessage(e));
        this.loadingMore.set(false);
      },
    });
  }

  protected open(n: AppNotification): void {
    if (n.read) {
      return;
    }
    this.service.markRead(n.id).subscribe({
      next: () => {
        this.items.update((cur) => cur.map((x) => (x.id === n.id ? { ...x, read: true } : x)));
        this.service.refreshCount();
      },
      error: (e: unknown) => this.error.set(errorMessage(e)),
    });
  }

  protected markAll(): void {
    this.service.markAllRead().subscribe({
      next: () => this.items.update((cur) => cur.map((x) => ({ ...x, read: true }))),
      error: (e: unknown) => this.error.set(errorMessage(e)),
    });
  }
}
