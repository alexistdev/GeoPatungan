-- Pengaturan platform (kunci/nilai) yang diubah admin.
CREATE TABLE platform_settings (
    key        TEXT PRIMARY KEY CHECK (key ~ '^[a-z][a-z0-9_]{0,63}$'),
    value      TEXT        NOT NULL,
    updated_by UUID        REFERENCES users(id) ON DELETE SET NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Rekening bank tujuan transfer top-up, diisi admin. Tidak dihapus, hanya dinonaktifkan,
-- supaya riwayat tetap utuh.
CREATE TABLE bank_accounts (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    bank_name      TEXT        NOT NULL,
    account_number TEXT        NOT NULL,
    account_holder TEXT        NOT NULL,
    notes          TEXT        NOT NULL DEFAULT '',
    is_active      BOOLEAN     NOT NULL DEFAULT true,
    created_by     UUID        REFERENCES users(id) ON DELETE SET NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (bank_name, account_number)
);

-- Jejak tindakan admin dan kejadian penting lainnya.
CREATE TABLE audit_logs (
    id         BIGSERIAL PRIMARY KEY,
    actor_id   UUID        REFERENCES users(id) ON DELETE SET NULL,
    action     TEXT        NOT NULL,
    entity     TEXT        NOT NULL,
    entity_id  UUID,
    meta       JSONB       NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_audit_logs_entity ON audit_logs (entity, entity_id);
CREATE INDEX idx_audit_logs_created ON audit_logs (created_at);
