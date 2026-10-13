/** Kelas lencana Velzon (badge) untuk tiap nada status. */
export const TONE = {
  neutral: 'bg-secondary-subtle text-secondary',
  ok: 'bg-success-subtle text-success',
  warn: 'bg-warning-subtle text-warning',
  danger: 'bg-danger-subtle text-danger',
  primary: 'bg-primary-subtle text-primary',
} as const;

export interface StatusLabel {
  text: string;
  /** Kelas CSS lencana, mis. "bg-success-subtle text-success". */
  tone: string;
}

const TOPUP: Record<string, StatusLabel> = {
  pending: { text: 'Menunggu transfer', tone: TONE.warn },
  submitted: { text: 'Menunggu verifikasi', tone: TONE.primary },
  approved: { text: 'Disetujui', tone: TONE.ok },
  rejected: { text: 'Ditolak', tone: TONE.danger },
  cancelled: { text: 'Dibatalkan', tone: TONE.neutral },
  expired: { text: 'Kedaluwarsa', tone: TONE.neutral },
};

const WITHDRAWAL: Record<string, StatusLabel> = {
  pending: { text: 'Diproses', tone: TONE.warn },
  completed: { text: 'Selesai', tone: TONE.ok },
  rejected: { text: 'Ditolak', tone: TONE.danger },
  cancelled: { text: 'Dibatalkan', tone: TONE.neutral },
};

const unknown = (s: string): StatusLabel => ({ text: s, tone: TONE.neutral });

export const topupLabel = (status: string): StatusLabel => TOPUP[status] ?? unknown(status);
export const withdrawalLabel = (status: string): StatusLabel => WITHDRAWAL[status] ?? unknown(status);

const GROUP: Record<string, StatusLabel> = {
  open: { text: 'Terbuka', tone: TONE.primary },
  running: { text: 'Berjalan', tone: TONE.ok },
  done: { text: 'Selesai', tone: TONE.neutral },
};
const ROUND: Record<string, StatusLabel> = {
  upcoming: { text: 'Akan datang', tone: TONE.neutral },
  collecting: { text: 'Pengumpulan iuran', tone: TONE.warn },
  paid_out: { text: 'Sudah dicairkan', tone: TONE.ok },
};
export const groupLabel = (s: string): StatusLabel => GROUP[s] ?? unknown(s);
export const roundLabel = (s: string): StatusLabel => ROUND[s] ?? unknown(s);

export const PERIOD_TEXT: Record<string, string> = { weekly: 'Mingguan', monthly: 'Bulanan' };
export const DRAW_TEXT: Record<string, string> = {
  random: 'Acak (adil)',
  weighted: 'Berbobot reputasi',
  manual: 'Ditentukan pemilik',
};
