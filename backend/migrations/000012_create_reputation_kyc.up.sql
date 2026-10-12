-- Definisi level reputasi. Level seorang pengguna TIDAK disimpan: ia dihitung dari data nyata
-- (KYC, arisan tuntas, keterlambatan) setiap kali dibutuhkan, sehingga tidak bisa melenceng.
-- Ambang di tabel ini bisa diubah admin tanpa deploy ulang.
CREATE TABLE reputation_levels (
    rank                 SMALLINT         PRIMARY KEY CHECK (rank BETWEEN 1 AND 3),
    code                 TEXT             NOT NULL UNIQUE CHECK (code ~ '^[a-z_]{2,20}$'),
    name                 TEXT             NOT NULL,
    -- syarat mencapai level ini
    require_verified     BOOLEAN          NOT NULL DEFAULT false,
    min_completed_cycles INT              NOT NULL DEFAULT 0 CHECK (min_completed_cycles >= 0),
    max_late_count       INT              CHECK (max_late_count IS NULL OR max_late_count >= 0), -- NULL = tanpa batas
    -- hak di level ini
    draw_weight          DOUBLE PRECISION NOT NULL CHECK (draw_weight >= 1 AND draw_weight <= 3), -- rasio maks 3:1
    can_create_group     BOOLEAN          NOT NULL DEFAULT true,
    max_group_value      BIGINT           CHECK (max_group_value IS NULL OR max_group_value > 0), -- NULL = tanpa batas
    updated_at           TIMESTAMPTZ      NOT NULL DEFAULT now(),
    -- level 1 adalah dasar: semua orang memenuhinya
    CHECK (rank <> 1 OR (NOT require_verified AND min_completed_cycles = 0 AND max_late_count IS NULL))
);

INSERT INTO reputation_levels
    (rank, code, name, require_verified, min_completed_cycles, max_late_count, draw_weight, can_create_group, max_group_value)
VALUES
    (1, 'baru',       'Baru',       false, 0, NULL, 1, true, 500000),
    (2, 'terpercaya', 'Terpercaya', true,  1, 0,    2, true, 5000000),
    (3, 'senior',     'Senior',     true,  3, 0,    3, true, NULL);

-- Syarat level minimal sebuah grup harus level yang ada.
ALTER TABLE groups
    ADD CONSTRAINT groups_min_level_rank_fkey FOREIGN KEY (min_level_rank) REFERENCES reputation_levels(rank);

-- Permintaan verifikasi identitas (KYC), diputuskan admin secara manual. Sengaja TIDAK menyimpan
-- nomor identitas atau dokumen; hanya keputusan dan catatannya.
CREATE TABLE kyc_requests (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      UUID        NOT NULL REFERENCES users(id),
    status       TEXT        NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'verified', 'rejected')),
    note         TEXT        NOT NULL DEFAULT '',
    review_note  TEXT        NOT NULL DEFAULT '',
    submitted_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    reviewed_by  UUID        REFERENCES users(id) ON DELETE SET NULL,
    reviewed_at  TIMESTAMPTZ,
    CHECK ((status <> 'pending') = (reviewed_at IS NOT NULL))
);

-- Satu permintaan pending per pengguna.
CREATE UNIQUE INDEX uq_kyc_pending ON kyc_requests (user_id) WHERE status = 'pending';
CREATE INDEX idx_kyc_status ON kyc_requests (status, submitted_at);
