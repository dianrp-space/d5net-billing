-- +goose Up
-- Per-cluster grace days (invoice due date offset) override for a plan offer.
-- NULL falls back to plans.grace_days.
ALTER TABLE plan_cluster_offers
    ADD COLUMN IF NOT EXISTS grace_days INT;

-- +goose Down
ALTER TABLE plan_cluster_offers DROP COLUMN IF EXISTS grace_days;
