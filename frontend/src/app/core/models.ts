export interface User {
  id: string;
  name: string;
  phone: string;
  email: string;
  role: 'member' | 'admin';
  kyc_status: 'none' | 'pending' | 'verified' | 'rejected';
  referral_code: string;
  referred_by: string | null;
  created_at: string;
}

export interface AuthResponse {
  token: string;
  user: User;
}

export interface Wallet {
  id: string;
  balance: number;
  created_at: string;
}

export interface ReputationLevel {
  rank: number;
  code: string;
  name: string;
  require_verified: boolean;
  min_completed_cycles: number;
  max_late_count: number | null;
  draw_weight: number;
  can_create_group: boolean;
  max_group_value: number | null;
}

export interface Requirement {
  key: string;
  label: string;
  met: boolean;
  current: number;
  required: number;
}

export interface ReputationProfile {
  level: ReputationLevel;
  stats: {
    kyc_status: string;
    verified: boolean;
    completed_cycles: number;
    on_time_payments: number;
    late_payments: number;
    overdue_unpaid: number;
    late_total: number;
  };
  next_level: { level: ReputationLevel; requirements: Requirement[] } | null;
}

export interface MyContribution {
  id: string;
  round_id: string;
  round_no: number;
  due_date: string;
  amount: number;
  platform_fee: number;
  late_fee: number;
  status: 'unpaid' | 'paid';
  paid_on_time: boolean | null;
  paid_at: string | null;
  overdue: boolean;
  group_id: string;
  group_name: string;
}

export interface AppNotification {
  id: number;
  type: string;
  title: string;
  body: string;
  data: Record<string, unknown>;
  read: boolean;
  created_at: string;
}

export interface NotificationPage {
  notifications: AppNotification[];
  next_before: number | null;
  unread_count: number;
}

export interface LedgerEntry {
  id: number;
  amount: number;
  balance_after: number;
  ref_type: string;
  ref_id: string;
  description: string;
  created_at: string;
}

export interface LedgerPage {
  entries: LedgerEntry[];
  next_before: number | null;
}

export interface BankAccount {
  id: string;
  bank_name: string;
  account_number: string;
  account_holder: string;
  notes: string;
  is_active: boolean;
}

export interface PaymentInfo {
  bank_accounts: BankAccount[];
  topup_min_amount: number;
  topup_max_amount: number;
  withdraw_min_amount: number;
  platform_fee_bps: number;
  topup_expiry_hours: number;
}

export type TopupStatus = 'pending' | 'submitted' | 'approved' | 'rejected' | 'cancelled' | 'expired';

export interface Topup {
  id: string;
  amount: number;
  unique_code: number;
  transfer_amount: number;
  status: TopupStatus;
  sender_name: string;
  sender_bank: string;
  note: string;
  submitted_at: string | null;
  expires_at: string;
  reviewed_at: string | null;
  review_note: string;
  received_amount: number | null;
  created_at: string;
}

export type WithdrawalStatus = 'pending' | 'completed' | 'rejected' | 'cancelled';

export interface Withdrawal {
  id: string;
  amount: number;
  bank_name: string;
  account_number: string;
  account_holder: string;
  note: string;
  status: WithdrawalStatus;
  transfer_reference: string;
  review_note: string;
  reviewed_at: string | null;
  created_at: string;
}

export interface WithdrawalInput {
  amount: number;
  bank_name: string;
  account_number: string;
  account_holder: string;
  note: string;
}

export interface AdminSettings {
  platform_fee_bps: number;
  topup_min_amount: number;
  topup_max_amount: number;
  withdraw_min_amount: number;
  topup_expiry_hours: number;
}

export interface AdminUserInfo {
  user_name: string;
  user_email: string;
  reviewed_by: string | null;
}

export type AdminTopup = Topup & AdminUserInfo & { user_id: string };
export type AdminWithdrawal = Withdrawal & AdminUserInfo & { user_id: string };

export interface KycRequest {
  id: string;
  user_id: string;
  status: 'pending' | 'verified' | 'rejected';
  note: string;
  review_note: string;
  submitted_at: string;
  reviewed_at: string | null;
}
export type AdminKycRequest = KycRequest & { user_name: string; user_email: string; reviewed_by: string | null };

export interface CommissionRule {
  level: number;
  share_bps: number;
}

export interface ReconcileReport {
  ok: boolean;
  problems: string[] | null;
  wallets_checked: number;
  entries_checked: number;
}
