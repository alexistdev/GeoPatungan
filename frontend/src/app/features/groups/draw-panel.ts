import { DatePipe } from '@angular/common';
import { Component, OnInit, computed, inject, input, output, signal } from '@angular/core';

import { errorMessage } from '../../core/errors';
import { GroupService } from '../../core/groups';
import { Draw, Group, Member } from '../../core/models';
import { DRAW_TEXT } from '../../shared/labels';

@Component({
  selector: 'app-draw-panel',
  imports: [DatePipe],
  templateUrl: './draw-panel.html',
})
export class DrawPanel implements OnInit {
  private readonly groups = inject(GroupService);

  readonly group = input.required<Group>();
  readonly members = input.required<Member[]>();
  readonly isOwner = input(false);
  readonly changed = output<void>();

  protected readonly draw = signal<Draw | null>(null);
  protected readonly loading = signal(true);
  protected readonly busy = signal(false);
  protected readonly error = signal<string | null>(null);
  protected readonly methodText = DRAW_TEXT;

  /** Urutan manual yang sedang disusun pemilik (member id). */
  protected readonly order = signal<string[]>([]);

  protected readonly activeMembers = computed(() => this.members().filter((m) => m.status === 'active'));
  protected readonly full = computed(() => this.activeMembers().length === this.group().slot_count);
  protected readonly executed = computed(() => !!this.draw()?.executed_at);
  protected readonly results = computed(() =>
    [...(this.draw()?.entries ?? [])].sort((a, b) => (a.turn_order ?? 999) - (b.turn_order ?? 999)),
  );

  ngOnInit(): void {
    this.load();
  }

  private load(): void {
    this.loading.set(true);
    this.groups.draw(this.group().id).subscribe({
      next: (d) => {
        this.draw.set(d);
        this.loading.set(false);
      },
      error: (e: unknown) => {
        this.error.set(errorMessage(e));
        this.loading.set(false);
      },
    });
  }

  protected nameOf(memberId: string): string {
    return this.members().find((m) => m.id === memberId)?.user_name ?? '-';
  }

  /** Daftar urutan manual: bila belum disusun, mulai dari urutan bergabung. */
  protected manualOrder(): string[] {
    const ids = this.activeMembers().map((m) => m.id);
    const cur = this.order().filter((id) => ids.includes(id));
    return cur.length === ids.length ? cur : ids;
  }

  protected move(index: number, delta: -1 | 1): void {
    const list = [...this.manualOrder()];
    const j = index + delta;
    if (j < 0 || j >= list.length) return;
    [list[index], list[j]] = [list[j], list[index]];
    this.order.set(list);
  }

  private run(call: ReturnType<GroupService['commitDraw']>): void {
    if (this.busy()) return;
    this.busy.set(true);
    this.error.set(null);
    call.subscribe({
      next: (d) => {
        this.draw.set(d);
        this.busy.set(false);
        this.changed.emit();
      },
      error: (e: unknown) => {
        this.busy.set(false);
        this.error.set(errorMessage(e));
      },
    });
  }

  protected commit(): void {
    this.run(this.groups.commitDraw(this.group().id));
  }
  protected execute(): void {
    this.run(this.groups.executeDraw(this.group().id));
  }
  protected saveManual(): void {
    this.run(this.groups.manualDraw(this.group().id, this.manualOrder()));
  }
}
