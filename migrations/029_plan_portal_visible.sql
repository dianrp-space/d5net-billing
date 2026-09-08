-- +goose Up
ALTER TABLE plans
    ADD COLUMN IF NOT EXISTS portal_visible BOOLEAN NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE plans DROP COLUMN IF EXISTS portal_visible;
