import { inject } from '@angular/core';
import { CanActivateFn, Router } from '@angular/router';
import { map } from 'rxjs';

import { AuthService } from './auth';

/** Hanya untuk yang sudah login. Profil dimuat sekali agar peran (admin) diketahui. */
export const authGuard: CanActivateFn = () => {
  const auth = inject(AuthService);
  const router = inject(Router);
  if (!auth.isLoggedIn()) {
    return router.parseUrl('/masuk');
  }
  if (auth.user()) {
    return true;
  }
  return auth.loadUser().pipe(
    map((u) => {
      if (u) {
        return true;
      }
      auth.logout(false);
      return router.parseUrl('/masuk');
    }),
  );
};

/** Hanya untuk admin; peran dibaca dari server, bukan dari token. */
export const adminGuard: CanActivateFn = () => {
  const auth = inject(AuthService);
  const router = inject(Router);
  return auth.isAdmin() ? true : router.parseUrl('/');
};

/** Halaman masuk/daftar: yang sudah login diarahkan ke beranda. */
export const guestGuard: CanActivateFn = () => {
  const auth = inject(AuthService);
  return auth.isLoggedIn() ? inject(Router).parseUrl('/') : true;
};
