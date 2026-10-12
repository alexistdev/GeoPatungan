DROP TABLE IF EXISTS kyc_requests;
ALTER TABLE groups DROP CONSTRAINT IF EXISTS groups_min_level_rank_fkey;
DROP TABLE IF EXISTS reputation_levels;
