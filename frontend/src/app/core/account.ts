import { HttpClient, HttpParams } from '@angular/common/http';
import { Injectable, inject } from '@angular/core';
import { Observable, map } from 'rxjs';

import { CommissionPage, KycRequest, KycState, ReferralSummary } from './models';

/** KYC dan referral milik pengguna yang login. */
@Injectable({ providedIn: 'root' })
export class AccountService {
  private readonly http = inject(HttpClient);

  kyc(): Observable<KycState> {
    return this.http.get<KycState>('/api/me/kyc/');
  }

  submitKyc(note: string): Observable<KycRequest> {
    return this.http.post<{ request: KycRequest }>('/api/me/kyc/', { note }).pipe(map((r) => r.request));
  }

  referral(): Observable<ReferralSummary> {
    return this.http.get<ReferralSummary>('/api/me/referral/');
  }

  commissions(opts: { limit?: number; before?: number | null } = {}): Observable<CommissionPage> {
    let params = new HttpParams();
    if (opts.limit) params = params.set('limit', opts.limit);
    if (opts.before) params = params.set('before', opts.before);
    return this.http.get<CommissionPage>('/api/me/referral/commissions', { params });
  }
}
