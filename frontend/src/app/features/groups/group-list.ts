import { Component, OnInit, inject, signal } from '@angular/core';
import { RouterLink } from '@angular/router';

import { AuthService } from '../../core/auth';
import { errorMessage } from '../../core/errors';
import { GroupService } from '../../core/groups';
import { Group } from '../../core/models';
import { groupLabel, PERIOD_TEXT } from '../../shared/labels';
import { PageTitle } from '../../shared/page-title';
import { RupiahPipe } from '../../shared/rupiah-pipe';

@Component({
  selector: 'app-group-list',
  imports: [RouterLink, RupiahPipe, PageTitle],
  templateUrl: './group-list.html',
})
export class GroupList implements OnInit {
  private readonly groups = inject(GroupService);

  protected readonly isAdmin = inject(AuthService).isAdmin;
  protected readonly items = signal<Group[]>([]);
  protected readonly loading = signal(true);
  protected readonly error = signal<string | null>(null);
  protected readonly label = groupLabel;
  protected readonly period = PERIOD_TEXT;

  ngOnInit(): void {
    this.groups.mine().subscribe({
      next: (g) => {
        this.items.set(g);
        this.loading.set(false);
      },
      error: (e: unknown) => {
        this.error.set(errorMessage(e));
        this.loading.set(false);
      },
    });
  }

  protected myStatusText(g: Group): string {
    if (g.my_status === 'pending') return 'Menunggu persetujuan pemilik';
    if (g.my_status === 'removed') return 'Ditolak';
    return '';
  }
}
