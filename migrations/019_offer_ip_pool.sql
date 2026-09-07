-- +goose Up
-- +goose StatementBegin

ALTER TABLE plan_cluster_offers
  ADD COLUMN IF NOT EXISTS ip_pool_id UUID REFERENCES ip_pools(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_plan_cluster_offers_ip_pool
  ON plan_cluster_offers(ip_pool_id)
  WHERE ip_pool_id IS NOT NULL;

COMMENT ON COLUMN plan_cluster_offers.ip_pool_id IS
  'Preferred IP pool for profile address-pool / remote-address when syncing to routers in this cluster';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP INDEX IF EXISTS idx_plan_cluster_offers_ip_pool;
ALTER TABLE plan_cluster_offers DROP COLUMN IF EXISTS ip_pool_id;

-- +goose StatementEnd
