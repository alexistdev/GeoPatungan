import { HttpClient } from '@angular/common/http';
import { Injectable, computed, inject, signal } from '@angular/core';
import { Router } from '@angular/router';
import { Observable, catchError, map, of, tap } from 'rxjs';

import { AuthResponse, User } from './models';
import { safeStorage } from './storage';

const TOKEN_KEY = 'gp.token';

export interface RegisterInput {
  name: string;
  phone: string;
  email: string;
  password: string;
  referral_code?: string;
}

@Injectable({ providedIn: 'root' })
export class AuthService {
  private readonly http = inject(HttpClient);
  private readonly router = inject(Router);

  private readonly tokenSig = signal<string | null>(safeStorage.get(TOKEN_KEY));
  readonly token = this.tokenSig.asReadonly();
  readonly user = signal<User | null>(null);
  readonly isLoggedIn = computed(() => this.tokenSig() !== null);
  readonly isAdmin = computed(() => this.user()?.role === 'admin');

  login(email: string, password: string): Observable<User> {
    return this.http
      .post<AuthResponse>('/api/auth/login', { email, password })
      .pipe(tap((r) => this.accept(r)), map((r) => r.user));
  }

  register(input: RegisterInput): Observable<User> {
    const body = { ...input };
    if (!body.referral_code?.trim()) {
      delete body.referral_code;
    }
    return this.http
      .post<AuthResponse>('/api/auth/register', body)
      .pipe(tap((r) => this.accept(r)), map((r) => r.user));
  }

  /** Memuat profil bila ada token. Mengembalikan null jika tidak login atau token ditolak. */
  loadUser(): Observable<User | null> {
    if (!this.tokenSig()) {
      return of(null);
    }
    return this.http.get<User>('/api/me').pipe(
      tap((u) => this.user.set(u)),
      catchError(() => of(null)),
    );
  }

  /** Menghapus sesi. Dipanggil juga oleh interseptor saat server menolak token (401). */
  logout(redirect = true): void {
    safeStorage.remove(TOKEN_KEY);
    this.tokenSig.set(null);
    this.user.set(null);
    if (redirect) {
      void this.router.navigateByUrl('/masuk');
    }
  }

  private accept(r: AuthResponse): void {
    safeStorage.set(TOKEN_KEY, r.token);
    this.tokenSig.set(r.token);
    this.user.set(r.user);
  }
}
