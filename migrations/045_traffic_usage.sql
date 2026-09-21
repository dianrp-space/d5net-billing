-- +goose Up
-- +goose StatementBegin

-- Akumulasi pemakaian trafik per pelanggan per bulan (dari sampling counter
-- RouterOS API oleh monitor poller; tanpa perlu RADIUS accounting).
CREATE TABLE IF NOT EXISTS traffic_monthly (
    tenant_id       UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    customer_id     UUID NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
    subscription_id UUID REFERENCES subscriptions(id) ON DELETE SET NULL,
    username        TEXT NOT NULL DEFAULT '',
    month           DATE NOT NULL,
    rx_bytes        BIGINT NOT NULL DEFAULT 0,
    tx_bytes        BIGINT NOT NULL DEFAULT 0,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (tenant_id, customer_id, month)
);
CREATE INDEX IF NOT EXISTS traffic_monthly_customer ON traffic_monthly(customer_id);

-- Sampel counter terakhir per sesi router (untuk hitung delta antar sampling;
-- counter interface me-reset saat sesi putus sehingga delta = nilai saat ini).
CREATE TABLE IF NOT EXISTS traffic_last_sample (
    tenant_id   UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    router_id   UUID NOT NULL REFERENCES routers(id) ON DELETE CASCADE,
    username    TEXT NOT NULL DEFAULT '',
    rx_bytes    BIGINT NOT NULL DEFAULT 0,
    tx_bytes    BIGINT NOT NULL DEFAULT 0,
    sampled_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (tenant_id, router_id, username)
);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TABLE IF EXISTS traffic_last_sample;
DROP TABLE IF EXISTS traffic_monthly;

-- +goose StatementEnd
