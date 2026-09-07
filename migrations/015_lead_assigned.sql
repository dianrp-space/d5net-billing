-- +goose Up
-- +goose StatementBegin

ALTER TABLE leads
  ADD COLUMN IF NOT EXISTS assigned_to UUID REFERENCES users(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_leads_assigned_to
  ON leads (tenant_id, assigned_to)
  WHERE assigned_to IS NOT NULL;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP INDEX IF EXISTS idx_leads_assigned_to;
ALTER TABLE leads DROP COLUMN IF EXISTS assigned_to;

-- +goose StatementEnd
