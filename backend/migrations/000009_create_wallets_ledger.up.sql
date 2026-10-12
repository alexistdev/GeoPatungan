-- Dompet. Selain dompet pengguna ada empat dompet sistem (satu per jenis):
--   escrow   : iuran yang terkumpul dan belum dicairkan ke penerima giliran
--   platform : biaya platform dan denda
--   holding  : dana penarikan yang sudah dipotong dari pengguna, menunggu admin mentransfer
--   external : "dunia luar" (rekening bank). Saldonya boleh negatif; ia lawan catatan
--              untuk uang yang masuk/keluar sistem, sehingga SELURUH saldo selalu berjumlah 0.
CREATE TABLE wallets (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID        UNIQUE REFERENCES users(id),
    type       TEXT        NOT NULL CHECK (type IN ('user', 'escrow', 'platform', 'holding', 'external')),
    balance    BIGINT      NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK ((type = 'user') = (user_id IS NOT NULL)),
    CHECK (balance >= 0 OR type = 'external')
);

CREATE UNIQUE INDEX uq_wallets_system_type ON wallets (type) WHERE type <> 'user';

INSERT INTO wallets (type) VALUES ('escrow'), ('platform'), ('holding'), ('external');

-- Buku besar berpasangan (double-entry): tiap peristiwa uang = satu entry_group berisi
-- beberapa baris bertanda (+/-) yang jumlahnya selalu 0. Hanya bisa ditambah.
CREATE TABLE ledger_entries (
    id            BIGSERIAL PRIMARY KEY,
    entry_group   UUID        NOT NULL,
    wallet_id     UUID        NOT NULL REFERENCES wallets(id),
    amount        BIGINT      NOT NULL CHECK (amount <> 0),
    balance_after BIGINT      NOT NULL,
    ref_type      TEXT        NOT NULL,
    ref_id        UUID        NOT NULL,
    description   TEXT        NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Satu peristiwa bisnis (ref) hanya boleh dicatat sekali per dompet: mencegah pencatatan ganda.
CREATE UNIQUE INDEX uq_ledger_ref_wallet ON ledger_entries (ref_type, ref_id, wallet_id);
CREATE INDEX idx_ledger_wallet ON ledger_entries (wallet_id, id DESC);
CREATE INDEX idx_ledger_group ON ledger_entries (entry_group);

-- Append-only: tidak boleh diubah, dihapus, atau dikosongkan.
CREATE FUNCTION ledger_entries_immutable() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'ledger_entries bersifat append-only (% ditolak)', TG_OP;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_ledger_immutable
    BEFORE UPDATE OR DELETE ON ledger_entries
    FOR EACH ROW EXECUTE FUNCTION ledger_entries_immutable();

CREATE TRIGGER trg_ledger_no_truncate
    BEFORE TRUNCATE ON ledger_entries
    FOR EACH STATEMENT EXECUTE FUNCTION ledger_entries_immutable();

-- Setiap entry_group harus seimbang. Dicek saat COMMIT (DEFERRED) agar baris-baris
-- dalam satu transaksi boleh dimasukkan satu per satu.
CREATE FUNCTION ledger_group_balanced() RETURNS trigger AS $$
BEGIN
    IF (SELECT COALESCE(SUM(amount), 0) FROM ledger_entries WHERE entry_group = NEW.entry_group) <> 0 THEN
        RAISE EXCEPTION 'entry_group % tidak seimbang', NEW.entry_group;
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER trg_ledger_balanced
    AFTER INSERT ON ledger_entries
    DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW EXECUTE FUNCTION ledger_group_balanced();
