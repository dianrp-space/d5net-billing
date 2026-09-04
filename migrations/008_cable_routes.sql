-- +goose Up
-- +goose StatementBegin
-- Manual FTTH cable paths (POP → ODP → splitter → customer), GeoJSON LineString coords.

CREATE TABLE IF NOT EXISTS cable_routes (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    cluster_id  UUID REFERENCES sites(id) ON DELETE SET NULL,
    name        TEXT NOT NULL,
    from_kind   TEXT NOT NULL DEFAULT 'manual',
    from_id     UUID,
    to_kind     TEXT NOT NULL DEFAULT 'manual',
    to_id       UUID,
    -- GeoJSON coordinates as [[lng,lat], ...] (LineString)
    path        JSONB NOT NULL DEFAULT '[]',
    color       TEXT NOT NULL DEFAULT '#5A5A40',
    notes       TEXT,
    is_active   BOOLEAN NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_cable_routes_tenant ON cable_routes(tenant_id);
CREATE INDEX IF NOT EXISTS idx_cable_routes_cluster ON cable_routes(cluster_id);

ALTER TABLE cable_routes ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_cable_routes ON cable_routes;
CREATE POLICY tenant_isolation_cable_routes ON cable_routes
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP POLICY IF EXISTS tenant_isolation_cable_routes ON cable_routes;
DROP TABLE IF EXISTS cable_routes;
-- +goose StatementEnd
