CREATE TABLE groups (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id            UUID        NOT NULL REFERENCES users(id),
    name                TEXT        NOT NULL,
    contribution_amount BIGINT      NOT NULL CHECK (contribution_amount > 0),
    period_type         TEXT        NOT NULL CHECK (period_type IN ('weekly', 'monthly')),
    slot_count          INT         NOT NULL CHECK (slot_count BETWEEN 2 AND 50),
    start_date          DATE        NOT NULL,
    late_fee            BIGINT      NOT NULL DEFAULT 0 CHECK (late_fee >= 0),
    draw_method         TEXT        NOT NULL DEFAULT 'random'
                        CHECK (draw_method IN ('random', 'weighted', 'manual')),
    min_level_rank      SMALLINT    NOT NULL DEFAULT 1 CHECK (min_level_rank BETWEEN 1 AND 3),
    status              TEXT        NOT NULL DEFAULT 'open'
                        CHECK (status IN ('open', 'running', 'done', 'cancelled')),
    invite_code         TEXT        NOT NULL UNIQUE,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_groups_owner ON groups (owner_id);
