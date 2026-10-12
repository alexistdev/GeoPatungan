import { DatePipe } from '@angular/common';
import { Component, OnInit, inject } from '@angular/core';

import { AdminService } from '../../core/admin';
import { AdminTopup } from '../../core/models';
import { ActionForm, FieldSpec } from '../../shared/action-form';
import { topupLabel } from '../../shared/labels';
import { RupiahPipe } from '../../shared/rupiah-pipe';
import { ReviewState } from './review';

@Component({
  selector: 'app-admin-topups',
  imports: [ActionForm, RupiahPipe, DatePipe],
  templateUrl: './topups.html',
})
export class AdminTopups implements OnInit {
  private readonly admin = inject(AdminService);
  protected readonly s = new ReviewState<AdminTopup>((st) => this.admin.topups(st), 'submitted');
  protected readonly label = topupLabel;

  protected readonly filters = [
    { value: 'submitted', text: 'Menunggu verifikasi' },
    { value: 'pending', text: 'Belum transfer' },
    { value: 'approved', text: 'Disetujui' },
    { value: 'rejected', text: 'Ditolak' },
    { value: 'expired', text: 'Kedaluwarsa' },
    { value: 'cancelled', text: 'Dibatalkan' },
    { value: 'all', text: 'Semua' },
  ];

  protected readonly approveFields: FieldSpec[] = [
    { name: 'received_amount', label: 'Jumlah diterima (Rp)', type: 'number', min: 1, hint: 'Kosongkan bila sesuai nominal transfer.' },
    { name: 'note', label: 'Catatan (opsional)', maxLength: 200 },
  ];
  protected readonly rejectFields: FieldSpec[] = [
    { name: 'note', label: 'Alasan penolakan', required: true, minLength: 3, maxLength: 200 },
  ];

  ngOnInit(): void {
    this.s.load();
  }

  /** Top-up yang masih bisa diputuskan admin (kedaluwarsa pun boleh: uang bisa datang terlambat). */
  protected decidable(t: AdminTopup): boolean {
    return t.status === 'pending' || t.status === 'submitted' || t.status === 'expired';
  }

  protected approve(t: AdminTopup, v: Record<string, string>): void {
    const body: { received_amount?: number; note: string } = { note: v['note'] };
    if (v['received_amount']) body.received_amount = Math.trunc(Number(v['received_amount']));
    this.s.run(this.admin.approveTopup(t.id, body), `Top-up ${t.user_name} disetujui.`);
  }

  protected reject(t: AdminTopup, v: Record<string, string>): void {
    this.s.run(this.admin.rejectTopup(t.id, v['note']), `Top-up ${t.user_name} ditolak.`);
  }
}
