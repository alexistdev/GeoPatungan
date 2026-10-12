import { HttpErrorResponse, HttpInterceptorFn } from '@angular/common/http';
import { inject } from '@angular/core';
import { catchError, throwError } from 'rxjs';

import { AuthService } from './auth';

const PUBLIC_PATHS = ['/api/auth/login', '/api/auth/register'];

/** Menyisipkan token ke permintaan API; sesi berakhir (401) mengeluarkan pengguna. */
export const authInterceptor: HttpInterceptorFn = (req, next) => {
  if (!req.url.startsWith('/api/')) {
    return next(req);
  }
  const auth = inject(AuthService);
  const isPublic = PUBLIC_PATHS.includes(req.url);
  const token = auth.token();
  const authed = token && !isPublic ? req.clone({ setHeaders: { Authorization: `Bearer ${token}` } }) : req;

  return next(authed).pipe(
    catchError((err: unknown) => {
      if (err instanceof HttpErrorResponse && err.status === 401 && !isPublic && auth.isLoggedIn()) {
        auth.logout();
      }
      return throwError(() => err);
    }),
  );
};
