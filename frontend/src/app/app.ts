import { Component, inject } from '@angular/core';
import { RouterOutlet } from '@angular/router';

import { LayoutService } from './core/layout';

@Component({
  imports: [RouterOutlet],
  selector: 'app-root',
  template: '<router-outlet />',
})
export class App {
  // Menginstansiasi layanan layout sejak awal agar tema (terang/gelap) langsung diterapkan, termasuk di halaman masuk.
  protected readonly layout = inject(LayoutService);
}
