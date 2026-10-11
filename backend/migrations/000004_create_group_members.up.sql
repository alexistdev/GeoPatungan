CREATE TABLE group_members (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    group_id   UUID        NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    user_id    UUID        NOT NULL REFERENCES users(id),
    turn_order INT         CHECK (turn_order >= 1),
    status     TEXT        NOT NULL DEFAULT 'pending'
               CHECK (status IN ('pending', 'active', 'removed')),
    joined_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (group_id, user_id)
);

-- Nomor giliran unik per grup, tetapi boleh kosong sebelum kocokan.
CREATE UNIQUE INDEX uq_group_members_turn
    ON group_members (group_id, turn_order)
    WHERE turn_order IS NOT NULL;

CREATE INDEX idx_group_members_user ON group_members (user_id);
