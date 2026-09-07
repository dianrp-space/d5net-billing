-- +goose Up
CREATE TABLE IF NOT EXISTS lead_comments (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    lead_id     UUID NOT NULL REFERENCES leads(id) ON DELETE CASCADE,
    user_id     UUID REFERENCES users(id) ON DELETE SET NULL,
    message     TEXT NOT NULL DEFAULT '',
    image_urls  JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_lead_comments_lead
    ON lead_comments (tenant_id, lead_id, created_at);

-- +goose Down
DROP TABLE IF EXISTS lead_comments;
