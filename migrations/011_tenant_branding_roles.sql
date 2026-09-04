-- +goose Up
-- +goose StatementBegin

ALTER TABLE tenants ADD COLUMN IF NOT EXISTS app_name TEXT;
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS favicon_url TEXT;
-- logo_url already exists from 001

CREATE TABLE IF NOT EXISTS platform_branding (
    id          SMALLINT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    app_name    TEXT NOT NULL DEFAULT 'drp-billing',
    logo_url    TEXT,
    favicon_url TEXT,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO platform_branding (id, app_name)
VALUES (1, 'drp-billing')
ON CONFLICT (id) DO NOTHING;

-- Seed system roles for existing tenants (admin may already exist)
INSERT INTO roles (tenant_id, name, slug, permissions, is_system)
SELECT t.id, 'Administrator', 'admin', '["*"]'::jsonb, true
FROM tenants t
WHERE NOT EXISTS (
    SELECT 1 FROM roles r WHERE r.tenant_id = t.id AND r.slug = 'admin'
);

INSERT INTO roles (tenant_id, name, slug, permissions, is_system)
SELECT t.id, 'Teknisi', 'teknisi', '["dashboard","network","ops","tickets"]'::jsonb, true
FROM tenants t
WHERE NOT EXISTS (
    SELECT 1 FROM roles r WHERE r.tenant_id = t.id AND r.slug = 'teknisi'
);

INSERT INTO roles (tenant_id, name, slug, permissions, is_system)
SELECT t.id, 'Sales', 'sales', '["dashboard","customers","leads","billing"]'::jsonb, true
FROM tenants t
WHERE NOT EXISTS (
    SELECT 1 FROM roles r WHERE r.tenant_id = t.id AND r.slug = 'sales'
);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS platform_branding;
ALTER TABLE tenants DROP COLUMN IF EXISTS favicon_url;
ALTER TABLE tenants DROP COLUMN IF EXISTS app_name;
-- +goose StatementEnd
