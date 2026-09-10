-- +goose Up
-- +goose StatementBegin

CREATE TABLE IF NOT EXISTS plan_discounts (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    plan_id     UUID REFERENCES plans(id) ON DELETE CASCADE,
    kind        TEXT NOT NULL DEFAULT 'percent',
    value       BIGINT NOT NULL CHECK (value > 0),
    starts_on   DATE NOT NULL,
    ends_on     DATE NOT NULL,
    audience    TEXT NOT NULL DEFAULT 'all',
    is_active   BOOLEAN NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT plan_discounts_kind_chk CHECK (kind IN ('percent', 'amount')),
    CONSTRAINT plan_discounts_audience_chk CHECK (audience IN ('all', 'selected')),
    CONSTRAINT plan_discounts_range_chk CHECK (ends_on >= starts_on),
    CONSTRAINT plan_discounts_percent_chk CHECK (kind <> 'percent' OR value <= 100)
);

CREATE INDEX IF NOT EXISTS idx_plan_discounts_tenant_active
    ON plan_discounts (tenant_id, is_active, starts_on, ends_on);

CREATE TABLE IF NOT EXISTS plan_discount_customers (
    discount_id UUID NOT NULL REFERENCES plan_discounts(id) ON DELETE CASCADE,
    customer_id UUID NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
    PRIMARY KEY (discount_id, customer_id)
);

CREATE INDEX IF NOT EXISTS idx_plan_discount_customers_customer
    ON plan_discount_customers (customer_id);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TABLE IF EXISTS plan_discount_customers;
DROP TABLE IF EXISTS plan_discounts;

-- +goose StatementEnd
