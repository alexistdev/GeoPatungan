-- Satu putaran per giliran: N slot = N putaran. Tiap anggota menerima tepat satu kali.
CREATE TABLE rounds (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    group_id            UUID NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    round_no            INT  NOT NULL CHECK (round_no >= 1),
    due_date            DATE NOT NULL,
    recipient_member_id UUID NOT NULL REFERENCES group_members(id),
    status              TEXT NOT NULL DEFAULT 'upcoming'
                        CHECK (status IN ('upcoming', 'collecting', 'paid_out')),
    UNIQUE (group_id, round_no),
    UNIQUE (group_id, recipient_member_id)
);
