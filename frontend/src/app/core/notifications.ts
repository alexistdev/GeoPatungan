import { HttpClient, HttpParams } from '@angular/common/http';
import { Injectable, inject, signal } from '@angular/core';
import { Observable, tap } from 'rxjs';

import { NotificationPage } from './models';

const POLL_MS = 60_000;

@Injectable({ providedIn: 'root' })
export class NotificationService {
  private readonly http = inject(HttpClient);
  readonly unread = signal(0);
  private timer: ReturnType<typeof setInterval> | null = null;

  list(opts: { limit?: number; before?: number | null; unreadOnly?: boolean } = {}): Observable<NotificationPage> {
    let params = new HttpParams();
    if (opts.limit) params = params.set('limit', opts.limit);
    if (opts.before) params = params.set('before', opts.before);
    if (opts.unreadOnly) params = params.set('unread', 'true');
    return this.http
      .get<NotificationPage>('/api/me/notifications/', { params })
      .pipe(tap((p) => this.unread.set(p.unread_count)));
  }

  markRead(id: number): Observable<unknown> {
    return this.http.post(`/api/me/notifications/${id}/read`, {});
  }

  markAllRead(): Observable<{ updated: number }> {
    return this.http
      .post<{ updated: number }>('/api/me/notifications/read-all', {})
      .pipe(tap(() => this.unread.set(0)));
  }

  refreshCount(): void {
    this.http.get<{ unread_count: number }>('/api/me/notifications/unread-count').subscribe({
      next: (r) => this.unread.set(r.unread_count),
      error: () => {
        /* lencana tidak kritis; gagal diabaikan */
      },
    });
  }

  startPolling(): void {
    this.stopPolling();
    this.refreshCount();
    this.timer = setInterval(() => this.refreshCount(), POLL_MS);
  }

  stopPolling(): void {
    if (this.timer !== null) {
      clearInterval(this.timer);
      this.timer = null;
    }
  }
}
