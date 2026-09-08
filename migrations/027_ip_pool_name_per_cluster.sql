-- +goose Up
-- +goose StatementBegin

ALTER TABLE ip_pools DROP CONSTRAINT IF EXISTS ip_pools_tenant_id_name_key;

CREATE UNIQUE INDEX IF NOT EXISTS ip_pools_tenant_cluster_name_uidx
  ON ip_pools (tenant_id, cluster_id, name)
  WHERE cluster_id IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS ip_pools_tenant_name_unassigned_uidx
  ON ip_pools (tenant_id, name)
  WHERE cluster_id IS NULL;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS ip_pools_tenant_cluster_name_uidx;
DROP INDEX IF EXISTS ip_pools_tenant_name_unassigned_uidx;
ALTER TABLE ip_pools ADD CONSTRAINT ip_pools_tenant_id_name_key UNIQUE (tenant_id, name);
-- +goose StatementEnd
