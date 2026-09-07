-- +goose Up
ALTER TABLE lead_comments
    ADD COLUMN IF NOT EXISTS kind TEXT NOT NULL DEFAULT 'comment';

ALTER TABLE lead_comments
    DROP CONSTRAINT IF EXISTS lead_comments_kind_check;

ALTER TABLE lead_comments
    ADD CONSTRAINT lead_comments_kind_check CHECK (kind IN ('comment', 'status_change'));

-- +goose Down
ALTER TABLE lead_comments DROP CONSTRAINT IF EXISTS lead_comments_kind_check;
ALTER TABLE lead_comments DROP COLUMN IF EXISTS kind;
