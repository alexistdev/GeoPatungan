import { DatePipe } from '@angular/common';
import { Component, inject, input, output, signal } from '@angular/core';

import { errorMessage } from '../../core/errors';
import { GroupService } from '../../core/groups';
import { Group, Member } from '../../core/models';

@Component({
  selector: 'app-members-panel',
  imports: [DatePipe],
  templateUrl: './members-panel.html',
})
export class MembersPanel {
  private readonly groups = inject(GroupService);

  readonly group = input.required<Group>();
  readonly members = input.required<Member[]>();
  readonly isOwner = input(false);
  readonly changed = output<void>();

  protected readonly busy = signal<string | null>(null);
  protected readonly error = signal<string | null>(null);

  protected active(): Member[] {
    return this.members().filter((m) => m.status === 'active');
  }
  protected pending(): Member[] {
    return this.members().filter((m) => m.status === 'pending');
  }

  protected act(m: Member, kind: 'approve' | 'reject'): void {
    if (this.busy()) return;
    this.busy.set(m.id);
    this.error.set(null);
    const call = kind === 'approve' ? this.groups.approveMember(this.group().id, m.id) : this.groups.rejectMember(this.group().id, m.id);
    call.subscribe({
      next: () => {
        this.busy.set(null);
        this.changed.emit();
      },
      error: (e: unknown) => {
        this.busy.set(null);
        this.error.set(errorMessage(e));
      },
    });
  }
}
