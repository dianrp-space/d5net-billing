-- +goose Up
-- +goose StatementBegin

ALTER TABLE ticket_messages
  ADD COLUMN IF NOT EXISTS image_urls JSONB NOT NULL DEFAULT '[]'::jsonb;

ALTER TABLE ticket_messages
  ALTER COLUMN message SET DEFAULT '';

-- allow empty message when images are attached
ALTER TABLE ticket_messages
  ALTER COLUMN message DROP NOT NULL;

UPDATE ticket_messages SET message = COALESCE(message, '');

ALTER TABLE ticket_messages
  ALTER COLUMN message SET NOT NULL;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

ALTER TABLE ticket_messages DROP COLUMN IF EXISTS image_urls;

-- +goose StatementEnd
