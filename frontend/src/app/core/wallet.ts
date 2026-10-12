import { HttpClient, HttpParams } from '@angular/common/http';
import { Injectable, inject } from '@angular/core';
import { Observable, map } from 'rxjs';

import { LedgerPage, PaymentInfo, Topup, Withdrawal, WithdrawalInput } from './models';

@Injectable({ providedIn: 'root' })
export class WalletService {
  private readonly http = inject(HttpClient);

  paymentInfo(): Observable<PaymentInfo> {
    return this.http.get<PaymentInfo>('/api/payment-info');
  }

  ledger(opts: { limit?: number; before?: number | null } = {}): Observable<LedgerPage> {
    let params = new HttpParams();
    if (opts.limit) params = params.set('limit', opts.limit);
    if (opts.before) params = params.set('before', opts.before);
    return this.http.get<LedgerPage>('/api/wallet/ledger', { params });
  }

  topups(): Observable<Topup[]> {
    return this.http.get<{ topups: Topup[] }>('/api/wallet/topups').pipe(map((r) => r.topups));
  }

  createTopup(amount: number): Observable<Topup> {
    return this.http.post<{ topup: Topup }>('/api/wallet/topups', { amount }).pipe(map((r) => r.topup));
  }

  confirmTopup(id: string, body: { sender_name: string; sender_bank: string; note: string }): Observable<Topup> {
    return this.http.post<{ topup: Topup }>(`/api/wallet/topups/${id}/confirm`, body).pipe(map((r) => r.topup));
  }

  cancelTopup(id: string): Observable<Topup> {
    return this.http.post<{ topup: Topup }>(`/api/wallet/topups/${id}/cancel`, {}).pipe(map((r) => r.topup));
  }

  withdrawals(): Observable<Withdrawal[]> {
    return this.http.get<{ withdrawals: Withdrawal[] }>('/api/wallet/withdrawals').pipe(map((r) => r.withdrawals));
  }

  createWithdrawal(input: WithdrawalInput): Observable<Withdrawal> {
    return this.http.post<{ withdrawal: Withdrawal }>('/api/wallet/withdrawals', input).pipe(map((r) => r.withdrawal));
  }

  cancelWithdrawal(id: string): Observable<Withdrawal> {
    return this.http.post<{ withdrawal: Withdrawal }>(`/api/wallet/withdrawals/${id}/cancel`, {}).pipe(map((r) => r.withdrawal));
  }
}
