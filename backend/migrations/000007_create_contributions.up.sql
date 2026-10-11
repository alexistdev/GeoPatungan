-- Tagihan iuran: satu baris per anggota per putaran, dibuat sekaligus saat grup dimulai.
-- "Terlambat" tidak disimpan sebagai status; ia diturunkan dari due_date dan paid_at.
CREATE TABLE contributions (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    round_id     UUID        NOT NULL REFERENCES rounds(id) ON DELETE CASCADE,
    member_id    UUID        NOT NULL REFERENCES group_members(id) ON DELETE CASCADE,
    amount       BIGINT      NOT NULL CHECK (amount > 0),
    platform_fee BIGINT      NOT NULL DEFAULT 0 CHECK (platform_fee >= 0),
    late_fee     BIGINT      NOT NULL DEFAULT 0 CHECK (late_fee >= 0),
    status       TEXT        NOT NULL DEFAULT 'unpaid' CHECK (status IN ('unpaid', 'paid')),
    paid_on_time BOOLEAN,
    paid_at      TIMESTAMPTZ,
    UNIQUE (round_id, member_id),
    CHECK ((status = 'paid') = (paid_at IS NOT NULL AND paid_on_time IS NOT NULL))
);

CREATE INDEX idx_contributions_member ON contributions (member_id);
