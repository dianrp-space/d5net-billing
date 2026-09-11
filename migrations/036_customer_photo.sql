-- +goose Up
-- Foto profil pelanggan (portal) untuk kartu dashboard.
ALTER TABLE customers ADD COLUMN IF NOT EXISTS photo_url TEXT;

-- +goose Down
ALTER TABLE customers DROP COLUMN IF EXISTS photo_url;
