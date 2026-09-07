-- +goose Up
ALTER TABLE users
    ADD COLUMN IF NOT EXISTS avatar_url TEXT;

-- +goose Down
ALTER TABLE users
    DROP COLUMN IF EXISTS avatar_url;
