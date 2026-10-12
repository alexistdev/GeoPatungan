-- Permintaan penarikan saldo ke rekening bank pengguna, diproses admin secara manual.
--   pending   : saldo sudah dipindahkan ke dompet "holding"; menunggu admin mentransfer
--   completed : admin sudah mentransfer (dana holding -> external); referensi bank wajib
--   rejected  : admin menolak (dana holding -> kembali ke pengguna)
--   cancelled : dibatalkan pengguna selagi pending (dana holding -> kembali ke pengguna)
CREATE TABLE withdrawals (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id            UUID        NOT NULL REFERENCES users(id),
    amount             BIGINT      NOT NULL CHECK (amount > 0),
    bank_name          TEXT        NOT NULL,
    account_number     TEXT        NOT NULL,
    account_holder     TEXT        NOT NULL,
    note               TEXT        NOT NULL DEFAULT '',
    status             TEXT        NOT NULL DEFAULT 'pending'
                       CHECK (status IN ('pending', 'completed', 'rejected', 'cancelled')),
    -- Nomor referensi/bukti transfer dari bank, diisi admin saat menyelesaikan.
    transfer_reference TEXT        NOT NULL DEFAULT '',
    review_note        TEXT        NOT NULL DEFAULT '',
    reviewed_by        UUID        REFERENCES users(id) ON DELETE SET NULL,
    reviewed_at        TIMESTAMPTZ,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK ((status IN ('completed', 'rejected')) = (reviewed_at IS NOT NULL)),
    CHECK (status <> 'completed' OR transfer_reference <> '')
);

CREATE INDEX idx_withdrawals_user ON withdrawals (user_id, created_at DESC);
CREATE INDEX idx_withdrawals_status ON withdrawals (status, created_at);
