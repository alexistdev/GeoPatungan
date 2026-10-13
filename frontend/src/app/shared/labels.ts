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

const GROUP: Record<string, StatusLabel> = {
  open: { text: 'Terbuka', tone: 'primary' },
  running: { text: 'Berjalan', tone: 'ok' },
  done: { text: 'Selesai', tone: '' },
};
const ROUND: Record<string, StatusLabel> = {
  upcoming: { text: 'Akan datang', tone: '' },
  collecting: { text: 'Pengumpulan iuran', tone: 'warn' },
  paid_out: { text: 'Sudah dicairkan', tone: 'ok' },
};
export const groupLabel = (s: string): StatusLabel => GROUP[s] ?? unknown(s);
export const roundLabel = (s: string): StatusLabel => ROUND[s] ?? unknown(s);

export const PERIOD_TEXT: Record<string, string> = { weekly: 'Mingguan', monthly: 'Bulanan' };
export const DRAW_TEXT: Record<string, string> = {
  random: 'Acak (adil)',
  weighted: 'Berbobot reputasi',
  manual: 'Ditentukan pemilik',
};
