-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS job_runs (
    id          BIGSERIAL PRIMARY KEY,
    tenant_id   BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    job_name    TEXT NOT NULL,
    job_key     TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'done',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, job_name, job_key)
);

CREATE TABLE IF NOT EXISTS outbound_webhooks (
    id          BIGSERIAL PRIMARY KEY,
    tenant_id   BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    url         TEXT NOT NULL,
    secret      TEXT,
    events      TEXT[] NOT NULL DEFAULT '{}',
    is_active   BOOLEAN NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS outbound_webhook_deliveries (
    id          BIGSERIAL PRIMARY KEY,
    tenant_id   BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    webhook_id  BIGINT NOT NULL REFERENCES outbound_webhooks(id) ON DELETE CASCADE,
    event       TEXT NOT NULL,
    payload     JSONB NOT NULL,
    status      TEXT NOT NULL DEFAULT 'pending',
    attempts    INT NOT NULL DEFAULT 0,
    last_error  TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    sent_at     TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS alerts (
    id          BIGSERIAL PRIMARY KEY,
    tenant_id   BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    severity    TEXT NOT NULL DEFAULT 'warn',
    kind        TEXT NOT NULL,
    title       TEXT NOT NULL,
    message     TEXT NOT NULL,
    entity_type TEXT,
    entity_id   BIGINT,
    is_acked    BOOLEAN NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_alerts_tenant_created ON alerts(tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_job_runs_lookup ON job_runs(tenant_id, job_name, job_key);

INSERT INTO notification_templates (tenant_id, channel, event, subject, body)
SELECT t.id, 'whatsapp', 'invoice_reminder', NULL,
       'Halo, tagihan {{invoice_number}} sebesar Rp {{amount}} jatuh tempo {{due_date}}. Bayar via portal pelanggan.'
FROM tenants t
WHERE NOT EXISTS (
  SELECT 1 FROM notification_templates nt
  WHERE nt.tenant_id = t.id AND nt.event = 'invoice_reminder' AND nt.channel = 'whatsapp'
);

INSERT INTO notification_templates (tenant_id, channel, event, subject, body)
SELECT t.id, 'whatsapp', 'payment_confirmation', NULL,
       'Pembayaran tagihan {{invoice_number}} sebesar Rp {{amount}} berhasil diterima. Terima kasih!'
FROM tenants t
WHERE NOT EXISTS (
  SELECT 1 FROM notification_templates nt
  WHERE nt.tenant_id = t.id AND nt.event = 'payment_confirmation' AND nt.channel = 'whatsapp'
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS outbound_webhook_deliveries, outbound_webhooks, alerts, job_runs;
-- +goose StatementEnd
