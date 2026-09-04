-- +goose Up
-- +goose StatementBegin
-- Enhance sites as POP/clusters; link customers; atomic customer-code sequences.

ALTER TABLE sites
    ADD COLUMN IF NOT EXISTS customer_code_prefix TEXT,
    ADD COLUMN IF NOT EXISTS customer_code_pattern TEXT NOT NULL DEFAULT '{prefix}-{yyyymm}-{seq}',
    ADD COLUMN IF NOT EXISTS seq_width INT NOT NULL DEFAULT 4,
    ADD COLUMN IF NOT EXISTS is_active BOOLEAN NOT NULL DEFAULT TRUE,
    ADD COLUMN IF NOT EXISTS notes TEXT;

UPDATE sites
SET customer_code_prefix = UPPER(REGEXP_REPLACE(code, '[^A-Za-z0-9]', '', 'g'))
WHERE customer_code_prefix IS NULL OR customer_code_prefix = '';

ALTER TABLE sites
    ALTER COLUMN customer_code_prefix SET NOT NULL;

ALTER TABLE customers
    ADD COLUMN IF NOT EXISTS cluster_id UUID REFERENCES sites(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_customers_cluster_id ON customers(cluster_id);
CREATE INDEX IF NOT EXISTS idx_routers_site_id ON routers(site_id);

CREATE TABLE IF NOT EXISTS customer_code_sequences (
    cluster_id UUID NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    period     TEXT NOT NULL DEFAULT '',
    last_seq   INT NOT NULL DEFAULT 0,
    PRIMARY KEY (cluster_id, period)
);

ALTER TABLE sites ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_sites ON sites;
CREATE POLICY tenant_isolation_sites ON sites
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP POLICY IF EXISTS tenant_isolation_sites ON sites;
ALTER TABLE sites DISABLE ROW LEVEL SECURITY;

DROP TABLE IF EXISTS customer_code_sequences;
DROP INDEX IF EXISTS idx_customers_cluster_id;
DROP INDEX IF EXISTS idx_routers_site_id;

ALTER TABLE customers DROP COLUMN IF EXISTS cluster_id;

ALTER TABLE sites
    DROP COLUMN IF EXISTS notes,
    DROP COLUMN IF EXISTS is_active,
    DROP COLUMN IF EXISTS seq_width,
    DROP COLUMN IF EXISTS customer_code_pattern,
    DROP COLUMN IF EXISTS customer_code_prefix;
-- +goose StatementEnd
