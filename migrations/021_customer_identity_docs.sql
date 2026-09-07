-- +goose Up
-- +goose StatementBegin
ALTER TABLE customers
    ADD COLUMN IF NOT EXISTS identity_type TEXT,
    ADD COLUMN IF NOT EXISTS identity_number TEXT;

-- Migrate legacy unused id_number if present
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'customers' AND column_name = 'id_number'
    ) THEN
        UPDATE customers
        SET identity_number = COALESCE(NULLIF(TRIM(identity_number), ''), NULLIF(TRIM(id_number), ''))
        WHERE identity_number IS NULL OR TRIM(identity_number) = '';
    END IF;
END $$;

CREATE TABLE IF NOT EXISTS lead_documents (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    lead_id      UUID NOT NULL REFERENCES leads(id) ON DELETE CASCADE,
    kind         TEXT NOT NULL DEFAULT 'other',
    url          TEXT NOT NULL,
    caption      TEXT NOT NULL DEFAULT '',
    uploaded_by  UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT lead_documents_kind_check CHECK (kind IN ('ktp', 'rumah', 'odp', 'psb', 'other'))
);

CREATE INDEX IF NOT EXISTS idx_lead_documents_lead
    ON lead_documents (tenant_id, lead_id, created_at);

CREATE TABLE IF NOT EXISTS customer_documents (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    customer_id  UUID NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
    kind         TEXT NOT NULL DEFAULT 'other',
    url          TEXT NOT NULL,
    caption      TEXT NOT NULL DEFAULT '',
    source       TEXT NOT NULL DEFAULT 'upload',
    source_ref   UUID,
    uploaded_by  UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT customer_documents_kind_check CHECK (kind IN ('ktp', 'rumah', 'odp', 'psb', 'other')),
    CONSTRAINT customer_documents_source_check CHECK (source IN ('upload', 'lead_convert'))
);

CREATE INDEX IF NOT EXISTS idx_customer_documents_customer
    ON customer_documents (tenant_id, customer_id, created_at);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS customer_documents;
DROP TABLE IF EXISTS lead_documents;
ALTER TABLE customers DROP COLUMN IF EXISTS identity_number;
ALTER TABLE customers DROP COLUMN IF EXISTS identity_type;
-- +goose StatementEnd
