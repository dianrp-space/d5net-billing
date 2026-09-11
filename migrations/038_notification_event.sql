-- +goose Up
-- Kolom event untuk riwayat pengiriman notifikasi (dunning, konfirmasi bayar, broadcast, ops, laporan).
ALTER TABLE notification_queue ADD COLUMN IF NOT EXISTS event TEXT NOT NULL DEFAULT '';

-- Index untuk halaman Riwayat (urut terbaru per tenant).
CREATE INDEX IF NOT EXISTS idx_notification_queue_history
    ON notification_queue(tenant_id, created_at DESC);

-- +goose Down
DROP INDEX IF EXISTS idx_notification_queue_history;
ALTER TABLE notification_queue DROP COLUMN IF EXISTS event;
