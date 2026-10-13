import { Component, OnInit, computed, inject, input, signal } from '@angular/core';

import { AuthService } from '../../core/auth';
import { errorMessage } from '../../core/errors';
import { GroupService } from '../../core/groups';
import { Group, Member } from '../../core/models';
import { DRAW_TEXT, groupLabel, PERIOD_TEXT } from '../../shared/labels';
import { PageTitle } from '../../shared/page-title';
import { RupiahPipe } from '../../shared/rupiah-pipe';
import { DrawPanel } from './draw-panel';
import { MembersPanel } from './members-panel';
import { RoundsPanel } from './rounds-panel';

@Component({
  selector: 'app-group-detail',
  imports: [RupiahPipe, MembersPanel, DrawPanel, RoundsPanel, PageTitle],
  templateUrl: './group-detail.html',
})
export class GroupDetail implements OnInit {
  private readonly groups = inject(GroupService);
  private readonly auth = inject(AuthService);

  /** Dari parameter rute :id (withComponentInputBinding). */
  readonly id = input.required<string>();

  protected readonly group = signal<Group | null>(null);
  protected readonly members = signal<Member[]>([]);
  protected readonly loading = signal(true);
  protected readonly error = signal<string | null>(null);
  protected readonly copied = signal(false);

  protected readonly isOwner = computed(() => !!this.group() && this.group()!.owner_id === this.auth.user()?.id);
  protected readonly isActiveMember = computed(() => this.group()?.my_status === 'active' || this.isOwner());
  protected readonly label = groupLabel;
  protected readonly period = PERIOD_TEXT;
  protected readonly method = DRAW_TEXT;

  ngOnInit(): void {
    this.load();
  }

  protected load(): void {
    this.error.set(null);
    this.groups.get(this.id()).subscribe({
      next: (g) => {
        this.group.set(g);
        this.loading.set(false);
      },
      error: (e: unknown) => {
        this.error.set(errorMessage(e));
        this.loading.set(false);
      },
    });
    this.groups.members(this.id()).subscribe({
      next: (m) => this.members.set(m),
      error: () => this.members.set([]),
    });
  }

  protected inviteLink(g: Group): string {
    return `${location.origin}/grup/gabung?kode=${encodeURIComponent(g.invite_code ?? '')}`;
  }

  protected async copy(text: string): Promise<void> {
    try {
      await navigator.clipboard.writeText(text);
      this.copied.set(true);
      setTimeout(() => this.copied.set(false), 2000);
    } catch {
      /* clipboard bisa diblokir; pengguna masih bisa menyalin manual */
    }
  }
}
