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
