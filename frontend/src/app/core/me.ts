import { HttpClient } from '@angular/common/http';
import { Injectable, inject } from '@angular/core';
import { Observable, map } from 'rxjs';

import { MyContribution, ReputationProfile, Wallet } from './models';

/** Data ringkas milik pengguna yang login (saldo, reputasi, iuran). */
@Injectable({ providedIn: 'root' })
export class MeService {
  private readonly http = inject(HttpClient);

  wallet(): Observable<Wallet> {
    return this.http.get<{ wallet: Wallet }>('/api/wallet/').pipe(map((r) => r.wallet));
  }

  reputation(): Observable<ReputationProfile> {
    return this.http.get<ReputationProfile>('/api/me/reputation');
  }

  contributions(status?: 'unpaid' | 'paid'): Observable<MyContribution[]> {
    const url = status ? `/api/me/contributions?status=${status}` : '/api/me/contributions';
    return this.http.get<{ contributions: MyContribution[] }>(url).pipe(map((r) => r.contributions));
  }
}
