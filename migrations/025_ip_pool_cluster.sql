-- +goose Up
-- +goose StatementBegin

ALTER TABLE ip_pools
  ADD COLUMN IF NOT EXISTS cluster_id UUID REFERENCES sites(id) ON DELETE SET NULL;

UPDATE ip_pools p
SET cluster_id = r.site_id
FROM routers r
WHERE p.router_id = r.id
  AND p.cluster_id IS NULL
  AND r.site_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS ip_pools_tenant_cluster_idx ON ip_pools (tenant_id, cluster_id);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS ip_pools_tenant_cluster_idx;
ALTER TABLE ip_pools DROP COLUMN IF EXISTS cluster_id;
-- +goose StatementEnd
