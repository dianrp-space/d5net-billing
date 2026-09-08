-- +goose Up
-- +goose StatementBegin

ALTER TABLE customers
  ADD COLUMN IF NOT EXISTS dismantled_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS customers_tenant_dismantled_idx ON customers (tenant_id, dismantled_at);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS customers_tenant_dismantled_idx;
ALTER TABLE customers DROP COLUMN IF EXISTS dismantled_at;
-- +goose StatementEnd
