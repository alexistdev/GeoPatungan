CREATE TABLE notifications (
    id         BIGSERIAL PRIMARY KEY,
    user_id    UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type       VARCHAR(50) NOT NULL,
    title      VARCHAR(120) NOT NULL CHECK (length(btrim(title)) > 0),
    body       VARCHAR(500) NOT NULL CHECK (length(btrim(body)) > 0),
    data       JSONB       NOT NULL DEFAULT '{}'::jsonb,
    read_at    TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- daftar terbaru dulu per pengguna (paging kursor) dan hitung yang belum dibaca
CREATE INDEX idx_notifications_user ON notifications (user_id, id DESC);
CREATE INDEX idx_notifications_unread ON notifications (user_id) WHERE read_at IS NULL;
