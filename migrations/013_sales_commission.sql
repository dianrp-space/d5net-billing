-- +goose Up
-- +goose StatementBegin

ALTER TABLE leads
  ADD COLUMN IF NOT EXISTS reseller_id UUID REFERENCES resellers(id) ON DELETE SET NULL,
  ADD COLUMN IF NOT EXISTS sales_user_id UUID REFERENCES users(id) ON DELETE SET NULL;

ALTER TABLE customers
  ADD COLUMN IF NOT EXISTS reseller_id UUID REFERENCES resellers(id) ON DELETE SET NULL,
  ADD COLUMN IF NOT EXISTS sales_user_id UUID REFERENCES users(id) ON DELETE SET NULL;

CREATE TABLE IF NOT EXISTS commission_entries (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    customer_id   UUID NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
    lead_id       UUID REFERENCES leads(id) ON DELETE SET NULL,
    reseller_id   UUID REFERENCES resellers(id) ON DELETE SET NULL,
    sales_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    amount        BIGINT NOT NULL CHECK (amount >= 0),
    basis         TEXT NOT NULL DEFAULT 'new_customer_flat',
    status        TEXT NOT NULL DEFAULT 'pending',
    note          TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    paid_at       TIMESTAMPTZ,
    CONSTRAINT commission_entries_status_chk CHECK (status IN ('pending', 'paid', 'void')),
    CONSTRAINT commission_entries_payee_chk CHECK (
        (reseller_id IS NOT NULL AND sales_user_id IS NULL)
        OR (reseller_id IS NULL AND sales_user_id IS NOT NULL)
    )
);

CREATE INDEX IF NOT EXISTS idx_commission_entries_tenant_status ON commission_entries (tenant_id, status);
CREATE INDEX IF NOT EXISTS idx_commission_entries_tenant_created ON commission_entries (tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_leads_reseller ON leads (tenant_id, reseller_id);
CREATE INDEX IF NOT EXISTS idx_leads_sales_user ON leads (tenant_id, sales_user_id);
CREATE INDEX IF NOT EXISTS idx_customers_reseller ON customers (tenant_id, reseller_id);
CREATE INDEX IF NOT EXISTS idx_customers_sales_user ON customers (tenant_id, sales_user_id);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP INDEX IF EXISTS idx_customers_sales_user;
DROP INDEX IF EXISTS idx_customers_reseller;
DROP INDEX IF EXISTS idx_leads_sales_user;
DROP INDEX IF EXISTS idx_leads_reseller;
DROP INDEX IF EXISTS idx_commission_entries_tenant_created;
DROP INDEX IF EXISTS idx_commission_entries_tenant_status;
DROP TABLE IF EXISTS commission_entries;
ALTER TABLE customers DROP COLUMN IF EXISTS sales_user_id;
ALTER TABLE customers DROP COLUMN IF EXISTS reseller_id;
ALTER TABLE leads DROP COLUMN IF EXISTS sales_user_id;
ALTER TABLE leads DROP COLUMN IF EXISTS reseller_id;

-- +goose StatementEnd
