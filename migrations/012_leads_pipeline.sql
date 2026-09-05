-- +goose Up
-- +goose StatementBegin

ALTER TABLE leads
  ADD COLUMN IF NOT EXISTS customer_id UUID REFERENCES customers(id) ON DELETE SET NULL,
  ADD COLUMN IF NOT EXISTS converted_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_leads_tenant_status ON leads (tenant_id, status);
CREATE INDEX IF NOT EXISTS idx_leads_tenant_created ON leads (tenant_id, created_at DESC);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP INDEX IF EXISTS idx_leads_tenant_created;
DROP INDEX IF EXISTS idx_leads_tenant_status;
ALTER TABLE leads DROP COLUMN IF EXISTS converted_at;
ALTER TABLE leads DROP COLUMN IF EXISTS customer_id;

-- +goose StatementEnd
