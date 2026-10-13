import { HttpClient, HttpParams } from '@angular/common/http';
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
    const params = status ? new HttpParams().set('status', status) : new HttpParams();
    return this.http.get<{ contributions: MyContribution[] }>('/api/me/contributions', { params }).pipe(map((r) => r.contributions));
  }
}
