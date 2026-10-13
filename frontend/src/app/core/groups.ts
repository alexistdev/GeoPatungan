import { HttpClient, HttpErrorResponse } from '@angular/common/http';
import { Injectable, inject } from '@angular/core';
import { Observable, catchError, map, of, throwError } from 'rxjs';

import { Draw, Group, GroupInput, Member, PayResult, Quote, Round, RoundContribution } from './models';

@Injectable({ providedIn: 'root' })
export class GroupService {
  private readonly http = inject(HttpClient);
  private readonly base = '/api/groups';

  mine(): Observable<Group[]> {
    return this.http.get<{ groups: Group[] }>(`${this.base}/`).pipe(map((r) => r.groups));
  }
  create(input: GroupInput): Observable<Group> {
    return this.http.post<Group>(`${this.base}/`, input);
  }
  join(inviteCode: string): Observable<Group> {
    return this.http.post<Group>(`${this.base}/join`, { invite_code: inviteCode });
  }
  get(id: string): Observable<Group> {
    return this.http.get<Group>(`${this.base}/${id}`);
  }

  members(id: string): Observable<Member[]> {
    return this.http.get<{ members: Member[] }>(`${this.base}/${id}/members`).pipe(map((r) => r.members));
  }
  approveMember(groupId: string, memberId: string): Observable<unknown> {
    return this.http.post(`${this.base}/${groupId}/members/${memberId}/approve`, {});
  }
  rejectMember(groupId: string, memberId: string): Observable<unknown> {
    return this.http.post(`${this.base}/${groupId}/members/${memberId}/reject`, {});
  }

  /** Kocokan grup; null bila belum ada (server membalas 404 "belum ada kocokan"). */
  draw(id: string): Observable<Draw | null> {
    return this.http.get<Draw>(`${this.base}/${id}/draw`).pipe(
      catchError((e: unknown) => (e instanceof HttpErrorResponse && e.status === 404 ? of(null) : throwError(() => e))),
    );
  }
  commitDraw(id: string): Observable<Draw> {
    return this.http.post<Draw>(`${this.base}/${id}/draw/commit`, {});
  }
  executeDraw(id: string): Observable<Draw> {
    return this.http.post<Draw>(`${this.base}/${id}/draw/execute`, {});
  }
  manualDraw(id: string, order: string[]): Observable<Draw> {
    return this.http.post<Draw>(`${this.base}/${id}/draw/manual`, { order });
  }

  rounds(id: string): Observable<Round[]> {
    return this.http.get<{ rounds: Round[] }>(`${this.base}/${id}/rounds`).pipe(map((r) => r.rounds));
  }
  roundContributions(groupId: string, roundId: string): Observable<RoundContribution[]> {
    return this.http
      .get<{ contributions: RoundContribution[] }>(`${this.base}/${groupId}/rounds/${roundId}/contributions`)
      .pipe(map((r) => r.contributions));
  }

  quote(contributionId: string): Observable<Quote> {
    // Server membalas {quote: {...}, balance, sufficient}; digabung menjadi satu objek datar.
    return this.http
      .get<{ quote: Omit<Quote, 'balance' | 'sufficient'>; balance: number; sufficient: boolean }>(`/api/contributions/${contributionId}/quote`)
      .pipe(map((r) => ({ ...r.quote, balance: r.balance, sufficient: r.sufficient })));
  }
  pay(contributionId: string): Observable<PayResult> {
    return this.http.post<PayResult>(`/api/contributions/${contributionId}/pay`, {});
  }
}
