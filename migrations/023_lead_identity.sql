-- +goose Up
ALTER TABLE leads
    ADD COLUMN IF NOT EXISTS identity_type TEXT,
    ADD COLUMN IF NOT EXISTS identity_number TEXT;

-- +goose Down
ALTER TABLE leads
    DROP COLUMN IF EXISTS identity_number,
    DROP COLUMN IF EXISTS identity_type;
