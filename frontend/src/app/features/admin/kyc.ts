import { DatePipe } from '@angular/common';
import { Component, OnInit, inject } from '@angular/core';

import { AdminService } from '../../core/admin';
import { AdminKycRequest } from '../../core/models';
import { ActionForm, FieldSpec } from '../../shared/action-form';
import { ReviewState } from './review';

@Component({
  selector: 'app-admin-kyc',
  imports: [ActionForm, DatePipe],
  templateUrl: './kyc.html',
})
export class AdminKyc implements OnInit {
  private readonly admin = inject(AdminService);
  protected readonly s = new ReviewState<AdminKycRequest>((st) => this.admin.kyc(st), 'pending');

  protected readonly filters = [
    { value: 'pending', text: 'Menunggu' },
    { value: 'verified', text: 'Terverifikasi' },
    { value: 'rejected', text: 'Ditolak' },
    { value: 'all', text: 'Semua' },
  ];
  protected readonly approveFields: FieldSpec[] = [{ name: 'note', label: 'Catatan (opsional)', maxLength: 200 }];
  protected readonly rejectFields: FieldSpec[] = [
    { name: 'note', label: 'Alasan penolakan', required: true, minLength: 3, maxLength: 200, hint: 'Pengguna akan melihat alasan ini.' },
  ];

  ngOnInit(): void {
    this.s.load();
  }

  protected statusText(st: string): string {
    return st === 'verified' ? 'Terverifikasi' : st === 'rejected' ? 'Ditolak' : 'Menunggu';
  }

  protected approve(r: AdminKycRequest, v: Record<string, string>): void {
    this.s.run(this.admin.approveKyc(r.id, v['note']), `${r.user_name} diverifikasi.`);
  }

  protected reject(r: AdminKycRequest, v: Record<string, string>): void {
    this.s.run(this.admin.rejectKyc(r.id, v['note']), `Pengajuan ${r.user_name} ditolak.`);
  }
}
