-- Permintaan top-up saldo lewat transfer bank manual.
--   pending    : menunggu pengguna mentransfer
--   submitted  : pengguna menyatakan sudah mentransfer, menunggu verifikasi admin
--   approved   : admin memverifikasi; saldo dikreditkan
--   rejected   : admin menolak
--   cancelled  : dibatalkan pengguna (hanya saat pending)
--   expired    : pending yang melewati batas waktu (admin tetap bisa menyetujuinya)
CREATE TABLE topups (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID        NOT NULL REFERENCES users(id),
    amount          BIGINT      NOT NULL CHECK (amount > 0),
    -- Kode unik 1..999 ditambahkan ke nominal transfer agar admin mudah mencocokkan mutasi bank.
    unique_code     INT         NOT NULL CHECK (unique_code BETWEEN 1 AND 999),
    transfer_amount BIGINT      NOT NULL,
    status          TEXT        NOT NULL DEFAULT 'pending'
                    CHECK (status IN ('pending', 'submitted', 'approved', 'rejected', 'cancelled', 'expired')),
    sender_name     TEXT        NOT NULL DEFAULT '',
    sender_bank     TEXT        NOT NULL DEFAULT '',
    note            TEXT        NOT NULL DEFAULT '',
    submitted_at    TIMESTAMPTZ,
    expires_at      TIMESTAMPTZ NOT NULL,
    reviewed_by     UUID        REFERENCES users(id) ON DELETE SET NULL,
    reviewed_at     TIMESTAMPTZ,
    review_note     TEXT        NOT NULL DEFAULT '',
    -- Nominal yang benar-benar diterima (bisa berbeda dari transfer_amount jika admin menyesuaikan).
    received_amount BIGINT      CHECK (received_amount > 0),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (transfer_amount = amount + unique_code),
    CHECK ((status = 'approved') = (received_amount IS NOT NULL)),
    CHECK ((status IN ('approved', 'rejected')) = (reviewed_at IS NOT NULL))
);

-- Dua permintaan terbuka tidak boleh punya nominal transfer yang sama; itulah gunanya kode unik.
CREATE UNIQUE INDEX uq_topups_open_transfer_amount
    ON topups (transfer_amount)
    WHERE status IN ('pending', 'submitted');

CREATE INDEX idx_topups_user ON topups (user_id, created_at DESC);
CREATE INDEX idx_topups_status ON topups (status, created_at);
