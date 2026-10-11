/*
 * Copyright (c) 2026.
 * Github : https://github.com/alexistdev
 * Email   : AlexistDev@gmail.com
 */
CREATE TABLE referrals (
                           descendant_id UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
                           ancestor_id   UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
                           level         SMALLINT    NOT NULL CHECK (level BETWEEN 1 AND 4),
                           created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
                           PRIMARY KEY (descendant_id, ancestor_id),
                           UNIQUE (descendant_id, level),
                           CHECK (descendant_id <> ancestor_id)
);

CREATE INDEX idx_referrals_ancestor ON referrals (ancestor_id);

