-- Satu kocokan per grup (UNIQUE group_id): tidak bisa diulang sampai hasilnya "bagus".
CREATE TABLE group_draws (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    group_id     UUID        NOT NULL UNIQUE REFERENCES groups(id) ON DELETE CASCADE,
    algorithm    TEXT        NOT NULL CHECK (algorithm IN ('weighted_es_v1', 'manual')),
    seed         TEXT,        -- hex 32 byte; rahasia sampai kocokan dijalankan
    seed_commit  TEXT,        -- sha256(seed) hex; diumumkan sebelum kocokan
    committed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    executed_at  TIMESTAMPTZ,
    CHECK (
        (algorithm = 'weighted_es_v1' AND seed IS NOT NULL AND seed_commit IS NOT NULL)
        OR (algorithm = 'manual' AND seed IS NULL AND seed_commit IS NULL)
    )
);

CREATE TABLE group_draw_entries (
    draw_id         UUID             NOT NULL REFERENCES group_draws(id) ON DELETE CASCADE,
    member_id       UUID             NOT NULL REFERENCES group_members(id) ON DELETE CASCADE,
    weight          DOUBLE PRECISION NOT NULL CHECK (weight > 0),
    score           DOUBLE PRECISION,
    resulting_order INT              CHECK (resulting_order >= 1),
    PRIMARY KEY (draw_id, member_id)
);

CREATE UNIQUE INDEX uq_draw_entries_order
    ON group_draw_entries (draw_id, resulting_order)
    WHERE resulting_order IS NOT NULL;
