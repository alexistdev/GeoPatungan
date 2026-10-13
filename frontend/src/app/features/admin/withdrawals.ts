import { DatePipe } from '@angular/common';
import { Component, OnInit, inject } from '@angular/core';

import { AdminService } from '../../core/admin';
import { AdminWithdrawal } from '../../core/models';
import { ActionForm, FieldSpec } from '../../shared/action-form';
import { withdrawalLabel } from '../../shared/labels';
import { RupiahPipe } from '../../shared/rupiah-pipe';
import { PageTitle } from '../../shared/page-title';
import { ReviewState } from './review';

@Component({
  selector: 'app-admin-withdrawals',
  imports: [ActionForm, RupiahPipe, DatePipe, PageTitle],
  templateUrl: './withdrawals.html',
})
export class AdminWithdrawals implements OnInit {
  private readonly admin = inject(AdminService);
  protected readonly s = new ReviewState<AdminWithdrawal>((st) => this.admin.withdrawals(st), 'pending');
  protected readonly label = withdrawalLabel;

  protected readonly filters = [
    { value: 'pending', text: 'Perlu diproses' },
    { value: 'completed', text: 'Selesai' },
    { value: 'rejected', text: 'Ditolak' },
    { value: 'cancelled', text: 'Dibatalkan' },
    { value: 'all', text: 'Semua' },
  ];

  protected readonly completeFields: FieldSpec[] = [
    { name: 'reference', label: 'Nomor referensi / bukti transfer bank', required: true, minLength: 3, maxLength: 100 },
    { name: 'note', label: 'Catatan (opsional)', maxLength: 200 },
  ];
  protected readonly rejectFields: FieldSpec[] = [
    { name: 'note', label: 'Alasan penolakan', required: true, minLength: 3, maxLength: 200, hint: 'Dana dikembalikan ke saldo pengguna.' },
  ];

  ngOnInit(): void {
    this.s.load();
  }

  protected complete(w: AdminWithdrawal, v: Record<string, string>): void {
    this.s.run(this.admin.completeWithdrawal(w.id, { reference: v['reference'], note: v['note'] }), `Penarikan ${w.user_name} ditandai selesai.`);
  }

  protected reject(w: AdminWithdrawal, v: Record<string, string>): void {
    this.s.run(this.admin.rejectWithdrawal(w.id, v['note']), `Penarikan ${w.user_name} ditolak dan dana dikembalikan.`);
  }
}
