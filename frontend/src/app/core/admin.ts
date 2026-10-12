import { HttpClient } from '@angular/common/http';
import { Injectable, inject } from '@angular/core';
import { Observable, map } from 'rxjs';

import {
  AdminKycRequest, AdminSettings, AdminTopup, AdminWithdrawal, BankAccount, CommissionRule,
  ReconcileReport, ReputationLevel,
} from './models';

export interface BankInput {
  bank_name: string;
  account_number: string;
  account_holder: string;
  notes: string;
  is_active: boolean;
}

/** Semua panggilan API khusus admin (server tetap memeriksa peran di setiap permintaan). */
@Injectable({ providedIn: 'root' })
export class AdminService {
  private readonly http = inject(HttpClient);
  private readonly base = '/api/admin';

  settings(): Observable<AdminSettings> {
    return this.http.get<AdminSettings>(`${this.base}/settings`);
  }
  updateSettings(s: AdminSettings): Observable<AdminSettings> {
    return this.http.put<AdminSettings>(`${this.base}/settings`, s);
  }

  banks(): Observable<BankAccount[]> {
    return this.http.get<{ bank_accounts: BankAccount[] }>(`${this.base}/bank-accounts`).pipe(map((r) => r.bank_accounts));
  }
  createBank(b: BankInput): Observable<BankAccount> {
    return this.http.post<BankAccount>(`${this.base}/bank-accounts`, b);
  }
  updateBank(id: string, b: BankInput): Observable<BankAccount> {
    return this.http.put<BankAccount>(`${this.base}/bank-accounts/${id}`, b);
  }

  topups(status: string): Observable<AdminTopup[]> {
    return this.http.get<{ topups: AdminTopup[] }>(`${this.base}/topups`, { params: { status } }).pipe(map((r) => r.topups));
  }
  approveTopup(id: string, body: { received_amount?: number; note: string }): Observable<unknown> {
    return this.http.post(`${this.base}/topups/${id}/approve`, body);
  }
  rejectTopup(id: string, note: string): Observable<unknown> {
    return this.http.post(`${this.base}/topups/${id}/reject`, { note });
  }

  withdrawals(status: string): Observable<AdminWithdrawal[]> {
    return this.http
      .get<{ withdrawals: AdminWithdrawal[] }>(`${this.base}/withdrawals`, { params: { status } })
      .pipe(map((r) => r.withdrawals));
  }
  completeWithdrawal(id: string, body: { reference: string; note: string }): Observable<unknown> {
    return this.http.post(`${this.base}/withdrawals/${id}/complete`, body);
  }
  rejectWithdrawal(id: string, note: string): Observable<unknown> {
    return this.http.post(`${this.base}/withdrawals/${id}/reject`, { note });
  }

  kyc(status: string): Observable<AdminKycRequest[]> {
    return this.http.get<{ requests: AdminKycRequest[] }>(`${this.base}/kyc`, { params: { status } }).pipe(map((r) => r.requests));
  }
  approveKyc(id: string, note: string): Observable<unknown> {
    return this.http.post(`${this.base}/kyc/${id}/approve`, { note });
  }
  rejectKyc(id: string, note: string): Observable<unknown> {
    return this.http.post(`${this.base}/kyc/${id}/reject`, { note });
  }

  referralRules(): Observable<CommissionRule[]> {
    return this.http.get<{ rules: CommissionRule[] }>(`${this.base}/referral/rules`).pipe(map((r) => r.rules));
  }
  saveReferralRules(rules: CommissionRule[]): Observable<CommissionRule[]> {
    return this.http.put<{ rules: CommissionRule[] }>(`${this.base}/referral/rules`, { rules }).pipe(map((r) => r.rules));
  }

  levels(): Observable<ReputationLevel[]> {
    return this.http.get<{ levels: ReputationLevel[] }>(`${this.base}/reputation/levels`).pipe(map((r) => r.levels));
  }
  saveLevels(levels: ReputationLevel[]): Observable<ReputationLevel[]> {
    return this.http.put<{ levels: ReputationLevel[] }>(`${this.base}/reputation/levels`, { levels }).pipe(map((r) => r.levels));
  }

  reconcile(): Observable<ReconcileReport> {
    return this.http.get<ReconcileReport>(`${this.base}/wallet/reconcile`);
  }
}
