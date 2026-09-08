-- +goose Up
ALTER TABLE payments ALTER COLUMN method DROP DEFAULT;
ALTER TABLE payments ALTER COLUMN status DROP DEFAULT;

-- +goose Down
ALTER TABLE payments ALTER COLUMN method SET DEFAULT 'manual';
ALTER TABLE payments ALTER COLUMN status SET DEFAULT 'pending';
