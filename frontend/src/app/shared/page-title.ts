import { Component, input } from '@angular/core';
import { RouterLink } from '@angular/router';

export interface Crumb {
  label: string;
  link?: string;
}

/** Judul halaman + breadcrumb ala Velzon (page-title-box). */
@Component({
  selector: 'app-page-title',
  imports: [RouterLink],
  template: `
    <div class="row">
      <div class="col-12">
        <div class="page-title-box d-sm-flex align-items-center justify-content-between">
          <h4 class="mb-sm-0">{{ title() }}</h4>
          <div class="page-title-right">
            <ol class="breadcrumb m-0">
              <li class="breadcrumb-item"><a routerLink="/">GeoPatungan</a></li>
              @for (c of crumbs(); track c.label) {
                @if (c.link) {
                  <li class="breadcrumb-item"><a [routerLink]="c.link">{{ c.label }}</a></li>
                }
              }
              <li class="breadcrumb-item active">{{ title() }}</li>
            </ol>
          </div>
        </div>
      </div>
    </div>
  `,
})
export class PageTitle {
  readonly title = input.required<string>();
  /** Tingkat di antara beranda dan halaman ini (hanya yang punya link yang ditampilkan). */
  readonly crumbs = input<Crumb[]>([]);
}
