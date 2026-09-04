-- +goose Up
-- +goose StatementBegin
-- Per-cluster plan pricing / availability.

CREATE TABLE IF NOT EXISTS plan_cluster_offers (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    plan_id     UUID NOT NULL REFERENCES plans(id) ON DELETE CASCADE,
    cluster_id  UUID NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    price       BIGINT NOT NULL CHECK (price >= 0),
    is_active   BOOLEAN NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (plan_id, cluster_id)
);

CREATE INDEX IF NOT EXISTS idx_plan_cluster_offers_tenant ON plan_cluster_offers(tenant_id);
CREATE INDEX IF NOT EXISTS idx_plan_cluster_offers_cluster ON plan_cluster_offers(cluster_id);

ALTER TABLE plan_cluster_offers ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_plan_cluster_offers ON plan_cluster_offers;
CREATE POLICY tenant_isolation_plan_cluster_offers ON plan_cluster_offers
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP POLICY IF EXISTS tenant_isolation_plan_cluster_offers ON plan_cluster_offers;
DROP TABLE IF EXISTS plan_cluster_offers;
-- +goose StatementEnd
