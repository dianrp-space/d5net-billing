-- +goose Up
-- Batch ID untuk progres pengiriman broadcast.
ALTER TABLE notification_queue ADD COLUMN IF NOT EXISTS batch_id UUID;
CREATE INDEX IF NOT EXISTS idx_notification_queue_batch
    ON notification_queue(tenant_id, batch_id) WHERE batch_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_notification_queue_batch;
ALTER TABLE notification_queue DROP COLUMN IF EXISTS batch_id;
