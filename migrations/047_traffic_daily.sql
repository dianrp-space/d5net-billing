-- +goose Up
-- +goose StatementBegin

-- Rincian pemakaian trafik per pelanggan per hari (untuk grafik harian di
-- portal). Diisi oleh sampling yang sama dengan traffic_monthly, sehingga
-- tidak menambah beban polling RouterOS.
CREATE TABLE IF NOT EXISTS traffic_daily (
    tenant_id   UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    customer_id UUID NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
    day         DATE NOT NULL,
    rx_bytes    BIGINT NOT NULL DEFAULT 0,
    tx_bytes    BIGINT NOT NULL DEFAULT 0,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (tenant_id, customer_id, day)
);
CREATE INDEX IF NOT EXISTS traffic_daily_customer ON traffic_daily(customer_id);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TABLE IF EXISTS traffic_daily;

-- +goose StatementEnd
