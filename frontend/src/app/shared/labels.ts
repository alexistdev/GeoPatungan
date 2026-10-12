export interface StatusLabel {
  text: string;
  tone: '' | 'ok' | 'warn' | 'danger' | 'primary';
}

const TOPUP: Record<string, StatusLabel> = {
  pending: { text: 'Menunggu transfer', tone: 'warn' },
  submitted: { text: 'Menunggu verifikasi', tone: 'primary' },
  approved: { text: 'Disetujui', tone: 'ok' },
  rejected: { text: 'Ditolak', tone: 'danger' },
  cancelled: { text: 'Dibatalkan', tone: '' },
  expired: { text: 'Kedaluwarsa', tone: '' },
};

const WITHDRAWAL: Record<string, StatusLabel> = {
  pending: { text: 'Diproses', tone: 'warn' },
  completed: { text: 'Selesai', tone: 'ok' },
  rejected: { text: 'Ditolak', tone: 'danger' },
  cancelled: { text: 'Dibatalkan', tone: '' },
};

const unknown = (s: string): StatusLabel => ({ text: s, tone: '' });

export const topupLabel = (status: string): StatusLabel => TOPUP[status] ?? unknown(status);
export const withdrawalLabel = (status: string): StatusLabel => WITHDRAWAL[status] ?? unknown(status);
