-- +goose Up
-- Replace the "N days after issue" grace period with a fixed calendar due day
-- (1-28). NULL falls back to the parent scope: offer -> plan -> tenant default.
ALTER TABLE plans ADD COLUMN IF NOT EXISTS due_day INT;
ALTER TABLE plans DROP COLUMN IF EXISTS grace_days;

ALTER TABLE plan_cluster_offers ADD COLUMN IF NOT EXISTS due_day INT;
ALTER TABLE plan_cluster_offers DROP COLUMN IF EXISTS grace_days;

-- +goose Down
ALTER TABLE plans DROP COLUMN IF EXISTS due_day;
ALTER TABLE plans ADD COLUMN IF NOT EXISTS grace_days INT NOT NULL DEFAULT 3;

ALTER TABLE plan_cluster_offers DROP COLUMN IF EXISTS due_day;
ALTER TABLE plan_cluster_offers ADD COLUMN IF NOT EXISTS grace_days INT;
