-- +goose Up
-- +goose StatementBegin

ALTER TABLE plans
  ADD COLUMN IF NOT EXISTS limit_uptime TEXT,
  ADD COLUMN IF NOT EXISTS shared_users INT;

COMMENT ON COLUMN plans.quota_gb IS 'Hotspot data quota in GB → RouterOS limit-bytes-total';
COMMENT ON COLUMN plans.limit_uptime IS 'Hotspot session uptime limit (RouterOS format e.g. 1d, 12h, 30m)';
COMMENT ON COLUMN plans.shared_users IS 'Hotspot concurrent login limit (shared-users)';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

ALTER TABLE plans DROP COLUMN IF EXISTS shared_users;
ALTER TABLE plans DROP COLUMN IF EXISTS limit_uptime;

-- +goose StatementEnd
