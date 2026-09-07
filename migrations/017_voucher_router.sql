-- +goose Up
-- +goose StatementBegin

ALTER TABLE voucher_batches
  ADD COLUMN IF NOT EXISTS router_id UUID REFERENCES routers(id) ON DELETE SET NULL,
  ADD COLUMN IF NOT EXISTS sync_status TEXT NOT NULL DEFAULT 'none',
  ADD COLUMN IF NOT EXISTS sync_error TEXT,
  ADD COLUMN IF NOT EXISTS synced_count INT NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS synced_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_voucher_batches_router ON voucher_batches(router_id)
  WHERE router_id IS NOT NULL;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP INDEX IF EXISTS idx_voucher_batches_router;
ALTER TABLE voucher_batches DROP COLUMN IF EXISTS synced_at;
ALTER TABLE voucher_batches DROP COLUMN IF EXISTS synced_count;
ALTER TABLE voucher_batches DROP COLUMN IF EXISTS sync_error;
ALTER TABLE voucher_batches DROP COLUMN IF EXISTS sync_status;
ALTER TABLE voucher_batches DROP COLUMN IF EXISTS router_id;

-- +goose StatementEnd
