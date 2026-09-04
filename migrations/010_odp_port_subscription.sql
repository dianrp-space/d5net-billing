-- +goose Up
CREATE UNIQUE INDEX IF NOT EXISTS idx_odp_ports_subscription_id
  ON odp_ports (subscription_id)
  WHERE subscription_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_odp_ports_odp_status
  ON odp_ports (odp_id, status);

-- +goose Down
DROP INDEX IF EXISTS idx_odp_ports_odp_status;
DROP INDEX IF EXISTS idx_odp_ports_subscription_id;
