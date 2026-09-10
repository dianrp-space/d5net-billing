-- +goose Up
-- +goose StatementBegin

ALTER TABLE invoices
  ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;

ALTER TABLE payments
  ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS invoices_tenant_deleted_idx ON invoices (tenant_id, deleted_at);
CREATE INDEX IF NOT EXISTS payments_tenant_deleted_idx ON payments (tenant_id, deleted_at);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS payments_tenant_deleted_idx;
DROP INDEX IF EXISTS invoices_tenant_deleted_idx;
ALTER TABLE payments DROP COLUMN IF EXISTS deleted_at;
ALTER TABLE invoices DROP COLUMN IF EXISTS deleted_at;
-- +goose StatementEnd
