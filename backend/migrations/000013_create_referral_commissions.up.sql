-- Aturan komisi referral: bagian dari BIAYA PLATFORM (bukan denda, bukan dana arisan) yang diberikan ke
-- upline pada tiap level, dalam basis poin dari biaya itu (4000 = 40%). share_bps = 0 menonaktifkan level.
CREATE TABLE commission_rules (
    level      SMALLINT    PRIMARY KEY CHECK (level BETWEEN 1 AND 4),
    share_bps  INT         NOT NULL CHECK (share_bps BETWEEN 0 AND 10000),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO commission_rules (level, share_bps) VALUES (1, 4000), (2, 2500), (3, 1500), (4, 1000);

-- Total bagian semua level tidak boleh melebihi 100% dari biaya: platform tidak boleh membayar komisi
-- lebih banyak daripada biaya yang diterimanya. Dicek saat COMMIT (DEFERRED) agar PUT yang mengubah
-- keempat baris boleh melewati keadaan antara.
CREATE FUNCTION commission_rules_total_ok() RETURNS trigger AS $$
BEGIN
    IF (SELECT COALESCE(SUM(share_bps), 0) FROM commission_rules) > 10000 THEN
        RAISE EXCEPTION 'total bagian komisi melebihi 100%% dari biaya platform';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER trg_commission_rules_total
    AFTER INSERT OR UPDATE ON commission_rules
    DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW EXECUTE FUNCTION commission_rules_total_ok();

-- Komisi yang sudah dibayarkan: satu baris per iuran per level. Uangnya sendiri tercatat di ledger
-- (ref_type 'referral_commission'); tabel ini menyimpan konteksnya untuk riwayat pengguna.
CREATE TABLE referral_commissions (
    id              BIGSERIAL PRIMARY KEY,
    contribution_id UUID        NOT NULL REFERENCES contributions(id) ON DELETE CASCADE,
    beneficiary_id  UUID        NOT NULL REFERENCES users(id),
    source_user_id  UUID        NOT NULL REFERENCES users(id),
    level           SMALLINT    NOT NULL CHECK (level BETWEEN 1 AND 4),
    share_bps       INT         NOT NULL CHECK (share_bps BETWEEN 1 AND 10000),
    amount          BIGINT      NOT NULL CHECK (amount > 0),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (contribution_id, level),
    CHECK (beneficiary_id <> source_user_id)
);

CREATE INDEX idx_referral_commissions_beneficiary ON referral_commissions (beneficiary_id, id DESC);
