import { Component } from '@angular/core';
import { RouterLink } from '@angular/router';

/** Bingkai halaman masuk/daftar bergaya Velzon (auth-page-wrapper); isi kartu diproyeksikan. */
@Component({
  selector: 'app-auth-shell',
  imports: [RouterLink],
  template: `
    <div class="auth-page-wrapper pt-5">
      <div class="auth-one-bg-position auth-one-bg" id="auth-particles">
        <div class="bg-overlay"></div>
        <div class="shape">
          <svg xmlns="http://www.w3.org/2000/svg" version="1.1" viewBox="0 0 1440 120">
            <path d="M 0,36 C 144,53.6 432,123.2 720,124 C 1008,124.8 1296,56.8 1440,40L1440 140L0 140z"></path>
          </svg>
        </div>
      </div>

      <div class="auth-page-content">
        <div class="container">
          <div class="row">
            <div class="col-lg-12">
              <div class="text-center mt-sm-5 mb-4 text-white-50">
                <a routerLink="/" class="d-inline-block auth-logo text-white fs-24 fw-semibold text-decoration-none">
                  <i class="ri-hand-coin-fill me-1"></i>GeoPatungan
                </a>
                <p class="mt-3 fs-15 fw-medium">Arisan online yang transparan</p>
              </div>
            </div>
          </div>

          <div class="row justify-content-center">
            <div class="col-md-8 col-lg-6 col-xl-5">
              <ng-content />
            </div>
          </div>
        </div>
      </div>

      <footer class="footer">
        <div class="container">
          <div class="row">
            <div class="col-lg-12">
              <div class="text-center">
                <p class="mb-0 text-muted">&copy; {{ year }} GeoPatungan · studi kasus open source</p>
              </div>
            </div>
          </div>
        </div>
      </footer>
    </div>
  `,
})
export class AuthShell {
  protected readonly year = new Date().getFullYear();
}
