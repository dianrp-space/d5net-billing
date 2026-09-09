-- +goose Up
-- +goose StatementBegin
-- gen_random_uuid() sudah built-in sejak PostgreSQL 13 (termasuk 18).
-- pgcrypto/citext butuh paket contrib; aaPanel sering tidak memasangnya.
DO $ext$
BEGIN
  CREATE EXTENSION IF NOT EXISTS pgcrypto;
EXCEPTION WHEN OTHERS THEN
  RAISE NOTICE 'pgcrypto dilewati (tidak wajib di PG 13+): %', SQLERRM;
END
$ext$;

DO $ext$
BEGIN
  CREATE EXTENSION IF NOT EXISTS citext;
EXCEPTION WHEN OTHERS THEN
  IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'citext') THEN
    CREATE DOMAIN citext AS TEXT;
  END IF;
  RAISE NOTICE 'citext extension tidak ada; domain TEXT dipakai: %', SQLERRM;
END
$ext$;

CREATE TABLE tenants (
    id          BIGSERIAL PRIMARY KEY,
    slug        TEXT NOT NULL UNIQUE,
    name        TEXT NOT NULL,
    email       CITEXT,
    phone       TEXT,
    address     TEXT,
    logo_url    TEXT,
    settings    JSONB NOT NULL DEFAULT '{}',
    is_active   BOOLEAN NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE roles (
    id          BIGSERIAL PRIMARY KEY,
    tenant_id   BIGINT REFERENCES tenants(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    slug        TEXT NOT NULL,
    permissions JSONB NOT NULL DEFAULT '[]',
    is_system   BOOLEAN NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, slug)
);

CREATE TABLE users (
    id              BIGSERIAL PRIMARY KEY,
    email           CITEXT NOT NULL UNIQUE,
    password_hash   TEXT NOT NULL,
    full_name       TEXT NOT NULL,
    phone           TEXT,
    is_active       BOOLEAN NOT NULL DEFAULT TRUE,
    totp_secret     TEXT,
    totp_enabled    BOOLEAN NOT NULL DEFAULT FALSE,
    last_login_at   TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE user_tenants (
    id          BIGSERIAL PRIMARY KEY,
    user_id     BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tenant_id   BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    role_id     BIGINT NOT NULL REFERENCES roles(id),
    is_default  BOOLEAN NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, tenant_id)
);

CREATE TABLE customers (
    id              BIGSERIAL PRIMARY KEY,
    tenant_id       BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    customer_code   TEXT NOT NULL,
    full_name       TEXT NOT NULL,
    email           CITEXT,
    phone           TEXT NOT NULL,
    password_hash   TEXT,
    address         TEXT,
    latitude        DOUBLE PRECISION,
    longitude       DOUBLE PRECISION,
    id_number       TEXT,
    notes           TEXT,
    is_active       BOOLEAN NOT NULL DEFAULT TRUE,
    portal_enabled  BOOLEAN NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, customer_code)
);

CREATE TABLE customer_addresses (
    id          BIGSERIAL PRIMARY KEY,
    tenant_id   BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    customer_id BIGINT NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
    label       TEXT NOT NULL DEFAULT 'primary',
    address     TEXT NOT NULL,
    latitude    DOUBLE PRECISION,
    longitude   DOUBLE PRECISION,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE sites (
    id          BIGSERIAL PRIMARY KEY,
    tenant_id   BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    code        TEXT NOT NULL,
    address     TEXT,
    latitude    DOUBLE PRECISION,
    longitude   DOUBLE PRECISION,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, code)
);

CREATE TABLE routers (
    id              BIGSERIAL PRIMARY KEY,
    tenant_id       BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    site_id         BIGINT REFERENCES sites(id) ON DELETE SET NULL,
    name            TEXT NOT NULL,
    address         TEXT NOT NULL,
    port            INT NOT NULL DEFAULT 8728,
    use_tls         BOOLEAN NOT NULL DEFAULT FALSE,
    provisioner     TEXT NOT NULL DEFAULT 'routeros',
    username        TEXT NOT NULL,
    password_enc    BYTEA NOT NULL,
    is_active       BOOLEAN NOT NULL DEFAULT TRUE,
    last_seen_at    TIMESTAMPTZ,
    last_error      TEXT,
    metadata        JSONB NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE ip_pools (
    id          BIGSERIAL PRIMARY KEY,
    tenant_id   BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    router_id   BIGINT REFERENCES routers(id) ON DELETE SET NULL,
    name        TEXT NOT NULL,
    network     CIDR NOT NULL,
    gateway     INET,
    dns_servers TEXT[],
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, name)
);

CREATE TABLE ip_assignments (
    id              BIGSERIAL PRIMARY KEY,
    tenant_id       BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    pool_id         BIGINT NOT NULL REFERENCES ip_pools(id) ON DELETE CASCADE,
    customer_id     BIGINT REFERENCES customers(id) ON DELETE SET NULL,
    subscription_id BIGINT,
    ip_address      INET NOT NULL,
    mac_address     MACADDR,
    status          TEXT NOT NULL DEFAULT 'assigned',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (pool_id, ip_address)
);

CREATE TABLE plans (
    id                  BIGSERIAL PRIMARY KEY,
    tenant_id           BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name                TEXT NOT NULL,
    code                TEXT NOT NULL,
    service_type        TEXT NOT NULL DEFAULT 'pppoe',
    price               BIGINT NOT NULL DEFAULT 0,
    billing_cycle       TEXT NOT NULL DEFAULT 'monthly',
    download_mbps       INT NOT NULL DEFAULT 10,
    upload_mbps         INT NOT NULL DEFAULT 10,
    quota_gb            INT,
    fup_download_mbps   INT,
    fup_upload_mbps     INT,
    profile_name        TEXT,
    isolir_profile      TEXT,
    grace_days          INT NOT NULL DEFAULT 3,
    tax_percent         NUMERIC(5,2) NOT NULL DEFAULT 0,
    is_active           BOOLEAN NOT NULL DEFAULT TRUE,
    metadata            JSONB NOT NULL DEFAULT '{}',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, code)
);

CREATE TABLE subscriptions (
    id              BIGSERIAL PRIMARY KEY,
    tenant_id       BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    customer_id     BIGINT NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
    plan_id         BIGINT NOT NULL REFERENCES plans(id),
    router_id       BIGINT REFERENCES routers(id) ON DELETE SET NULL,
    username        TEXT NOT NULL,
    password        TEXT,
    service_type    TEXT NOT NULL DEFAULT 'pppoe',
    status          TEXT NOT NULL DEFAULT 'pending',
    ip_address      INET,
    mac_address     MACADDR,
    started_at      TIMESTAMPTZ,
    expires_at      TIMESTAMPTZ,
    next_bill_at    TIMESTAMPTZ,
    suspended_at    TIMESTAMPTZ,
    metadata        JSONB NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, username)
);

CREATE TABLE invoices (
    id              BIGSERIAL PRIMARY KEY,
    tenant_id       BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    customer_id     BIGINT NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
    subscription_id BIGINT REFERENCES subscriptions(id) ON DELETE SET NULL,
    invoice_number  TEXT NOT NULL,
    period_start    DATE,
    period_end      DATE,
    subtotal        BIGINT NOT NULL DEFAULT 0,
    tax_amount      BIGINT NOT NULL DEFAULT 0,
    discount_amount BIGINT NOT NULL DEFAULT 0,
    total_amount    BIGINT NOT NULL DEFAULT 0,
    paid_amount     BIGINT NOT NULL DEFAULT 0,
    status          TEXT NOT NULL DEFAULT 'draft',
    due_date        DATE NOT NULL,
    issued_at       TIMESTAMPTZ,
    paid_at         TIMESTAMPTZ,
    notes           TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, invoice_number)
);

CREATE TABLE invoice_items (
    id          BIGSERIAL PRIMARY KEY,
    tenant_id   BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    invoice_id  BIGINT NOT NULL REFERENCES invoices(id) ON DELETE CASCADE,
    description TEXT NOT NULL,
    quantity    INT NOT NULL DEFAULT 1,
    unit_price  BIGINT NOT NULL DEFAULT 0,
    amount      BIGINT NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE payments (
    id              BIGSERIAL PRIMARY KEY,
    tenant_id       BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    customer_id     BIGINT NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
    invoice_id      BIGINT REFERENCES invoices(id) ON DELETE SET NULL,
    amount          BIGINT NOT NULL,
    method          TEXT NOT NULL DEFAULT 'manual',
    reference       TEXT,
    status          TEXT NOT NULL DEFAULT 'pending',
    paid_at         TIMESTAMPTZ,
    metadata        JSONB NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE payment_intents (
    id              BIGSERIAL PRIMARY KEY,
    tenant_id       BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    customer_id     BIGINT NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
    invoice_id      BIGINT REFERENCES invoices(id) ON DELETE SET NULL,
    provider        TEXT NOT NULL,
    external_id     TEXT,
    amount          BIGINT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'pending',
    checkout_url    TEXT,
    expires_at      TIMESTAMPTZ,
    metadata        JSONB NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE wallets (
    id          BIGSERIAL PRIMARY KEY,
    tenant_id   BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    customer_id BIGINT NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
    balance     BIGINT NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, customer_id)
);

CREATE TABLE wallet_transactions (
    id          BIGSERIAL PRIMARY KEY,
    tenant_id   BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    wallet_id   BIGINT NOT NULL REFERENCES wallets(id) ON DELETE CASCADE,
    amount      BIGINT NOT NULL,
    type        TEXT NOT NULL,
    reference   TEXT,
    description TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE voucher_batches (
    id          BIGSERIAL PRIMARY KEY,
    tenant_id   BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    plan_id     BIGINT REFERENCES plans(id) ON DELETE SET NULL,
    name        TEXT NOT NULL,
    quantity    INT NOT NULL DEFAULT 0,
    price       BIGINT NOT NULL DEFAULT 0,
    expires_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE vouchers (
    id          BIGSERIAL PRIMARY KEY,
    tenant_id   BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    batch_id    BIGINT NOT NULL REFERENCES voucher_batches(id) ON DELETE CASCADE,
    code        TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'available',
    used_by     BIGINT REFERENCES customers(id) ON DELETE SET NULL,
    used_at     TIMESTAMPTZ,
    expires_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, code)
);

CREATE TABLE tickets (
    id          BIGSERIAL PRIMARY KEY,
    tenant_id   BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    customer_id BIGINT REFERENCES customers(id) ON DELETE SET NULL,
    subject     TEXT NOT NULL,
    description TEXT,
    category    TEXT NOT NULL DEFAULT 'general',
    priority    TEXT NOT NULL DEFAULT 'normal',
    status      TEXT NOT NULL DEFAULT 'open',
    assigned_to BIGINT REFERENCES users(id) ON DELETE SET NULL,
    sla_due_at  TIMESTAMPTZ,
    resolved_at TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE ticket_messages (
    id          BIGSERIAL PRIMARY KEY,
    tenant_id   BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    ticket_id   BIGINT NOT NULL REFERENCES tickets(id) ON DELETE CASCADE,
    sender_type TEXT NOT NULL,
    sender_id   BIGINT,
    message     TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE technicians (
    id          BIGSERIAL PRIMARY KEY,
    tenant_id   BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id     BIGINT REFERENCES users(id) ON DELETE SET NULL,
    full_name   TEXT NOT NULL,
    phone       TEXT NOT NULL,
    is_active   BOOLEAN NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE work_orders (
    id              BIGSERIAL PRIMARY KEY,
    tenant_id       BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    customer_id     BIGINT REFERENCES customers(id) ON DELETE SET NULL,
    subscription_id BIGINT REFERENCES subscriptions(id) ON DELETE SET NULL,
    technician_id   BIGINT REFERENCES technicians(id) ON DELETE SET NULL,
    type            TEXT NOT NULL DEFAULT 'installation',
    status          TEXT NOT NULL DEFAULT 'pending',
    scheduled_at    TIMESTAMPTZ,
    check_in_at     TIMESTAMPTZ,
    check_in_lat    DOUBLE PRECISION,
    check_in_lng    DOUBLE PRECISION,
    completed_at    TIMESTAMPTZ,
    notes           TEXT,
    photos          JSONB NOT NULL DEFAULT '[]',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE odcs (
    id          BIGSERIAL PRIMARY KEY,
    tenant_id   BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    site_id     BIGINT REFERENCES sites(id) ON DELETE SET NULL,
    name        TEXT NOT NULL,
    code        TEXT NOT NULL,
    address     TEXT,
    latitude    DOUBLE PRECISION,
    longitude   DOUBLE PRECISION,
    capacity    INT NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, code)
);

CREATE TABLE odps (
    id          BIGSERIAL PRIMARY KEY,
    tenant_id   BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    odc_id      BIGINT REFERENCES odcs(id) ON DELETE SET NULL,
    name        TEXT NOT NULL,
    code        TEXT NOT NULL,
    address     TEXT,
    latitude    DOUBLE PRECISION,
    longitude   DOUBLE PRECISION,
    port_count  INT NOT NULL DEFAULT 8,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, code)
);

CREATE TABLE odp_ports (
    id              BIGSERIAL PRIMARY KEY,
    tenant_id       BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    odp_id          BIGINT NOT NULL REFERENCES odps(id) ON DELETE CASCADE,
    port_number     INT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'available',
    customer_id     BIGINT REFERENCES customers(id) ON DELETE SET NULL,
    subscription_id BIGINT REFERENCES subscriptions(id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (odp_id, port_number)
);

CREATE TABLE chart_of_accounts (
    id          BIGSERIAL PRIMARY KEY,
    tenant_id   BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    code        TEXT NOT NULL,
    name        TEXT NOT NULL,
    type        TEXT NOT NULL,
    is_active   BOOLEAN NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, code)
);

CREATE TABLE journal_entries (
    id          BIGSERIAL PRIMARY KEY,
    tenant_id   BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    entry_date  DATE NOT NULL,
    reference   TEXT,
    description TEXT NOT NULL,
    source_type TEXT,
    source_id   BIGINT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE journal_lines (
    id          BIGSERIAL PRIMARY KEY,
    tenant_id   BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    entry_id    BIGINT NOT NULL REFERENCES journal_entries(id) ON DELETE CASCADE,
    account_id  BIGINT NOT NULL REFERENCES chart_of_accounts(id),
    debit       BIGINT NOT NULL DEFAULT 0,
    credit      BIGINT NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE expenses (
    id          BIGSERIAL PRIMARY KEY,
    tenant_id   BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    account_id  BIGINT REFERENCES chart_of_accounts(id) ON DELETE SET NULL,
    amount      BIGINT NOT NULL,
    category    TEXT NOT NULL,
    description TEXT,
    expense_date DATE NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE notification_templates (
    id          BIGSERIAL PRIMARY KEY,
    tenant_id   BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    channel     TEXT NOT NULL,
    event       TEXT NOT NULL,
    subject     TEXT,
    body        TEXT NOT NULL,
    is_active   BOOLEAN NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, channel, event)
);

CREATE TABLE notification_queue (
    id          BIGSERIAL PRIMARY KEY,
    tenant_id   BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    channel     TEXT NOT NULL,
    recipient   TEXT NOT NULL,
    subject     TEXT,
    body        TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'pending',
    attempts    INT NOT NULL DEFAULT 0,
    scheduled_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    sent_at     TIMESTAMPTZ,
    error       TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE router_metrics (
    id          BIGSERIAL PRIMARY KEY,
    tenant_id   BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    router_id   BIGINT NOT NULL REFERENCES routers(id) ON DELETE CASCADE,
    cpu_load    NUMERIC(5,2),
    memory_used BIGINT,
    uptime      BIGINT,
    active_sessions INT NOT NULL DEFAULT 0,
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE traffic_samples (
    id              BIGSERIAL PRIMARY KEY,
    tenant_id       BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    router_id       BIGINT REFERENCES routers(id) ON DELETE SET NULL,
    subscription_id BIGINT REFERENCES subscriptions(id) ON DELETE SET NULL,
    rx_bytes        BIGINT NOT NULL DEFAULT 0,
    tx_bytes        BIGINT NOT NULL DEFAULT 0,
    recorded_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE service_sessions (
    id              BIGSERIAL PRIMARY KEY,
    tenant_id       BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    subscription_id BIGINT REFERENCES subscriptions(id) ON DELETE SET NULL,
    router_id       BIGINT REFERENCES routers(id) ON DELETE SET NULL,
    username        TEXT NOT NULL,
    ip_address      INET,
    mac_address     MACADDR,
    started_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ended_at        TIMESTAMPTZ,
    rx_bytes        BIGINT NOT NULL DEFAULT 0,
    tx_bytes        BIGINT NOT NULL DEFAULT 0
);

CREATE TABLE router_command_logs (
    id          BIGSERIAL PRIMARY KEY,
    tenant_id   BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    router_id   BIGINT NOT NULL REFERENCES routers(id) ON DELETE CASCADE,
    user_id     BIGINT REFERENCES users(id) ON DELETE SET NULL,
    command     TEXT NOT NULL,
    result      TEXT,
    success     BOOLEAN NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE router_backups (
    id          BIGSERIAL PRIMARY KEY,
    tenant_id   BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    router_id   BIGINT NOT NULL REFERENCES routers(id) ON DELETE CASCADE,
    filename    TEXT NOT NULL,
    content     TEXT NOT NULL,
    checksum    TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE audit_logs (
    id          BIGSERIAL PRIMARY KEY,
    tenant_id   BIGINT REFERENCES tenants(id) ON DELETE SET NULL,
    user_id     BIGINT REFERENCES users(id) ON DELETE SET NULL,
    action      TEXT NOT NULL,
    entity_type TEXT,
    entity_id   BIGINT,
    metadata    JSONB NOT NULL DEFAULT '{}',
    ip_address  INET,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE api_keys (
    id          BIGSERIAL PRIMARY KEY,
    tenant_id   BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    key_hash    TEXT NOT NULL,
    permissions JSONB NOT NULL DEFAULT '[]',
    last_used_at TIMESTAMPTZ,
    expires_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE settings (
    id          BIGSERIAL PRIMARY KEY,
    tenant_id   BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    key         TEXT NOT NULL,
    value       JSONB NOT NULL DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, key)
);

CREATE TABLE refresh_tokens (
    id          BIGSERIAL PRIMARY KEY,
    user_id     BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash  TEXT NOT NULL UNIQUE,
    expires_at  TIMESTAMPTZ NOT NULL,
    revoked_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE leads (
    id          BIGSERIAL PRIMARY KEY,
    tenant_id   BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    full_name   TEXT NOT NULL,
    phone       TEXT NOT NULL,
    email       CITEXT,
    address     TEXT,
    latitude    DOUBLE PRECISION,
    longitude   DOUBLE PRECISION,
    odp_id      BIGINT REFERENCES odps(id) ON DELETE SET NULL,
    status      TEXT NOT NULL DEFAULT 'new',
    notes       TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE resellers (
    id          BIGSERIAL PRIMARY KEY,
    tenant_id   BIGINT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id     BIGINT REFERENCES users(id) ON DELETE SET NULL,
    name        TEXT NOT NULL,
    phone       TEXT,
    commission_percent NUMERIC(5,2) NOT NULL DEFAULT 0,
    balance     BIGINT NOT NULL DEFAULT 0,
    is_active   BOOLEAN NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- RLS policies
ALTER TABLE customers ENABLE ROW LEVEL SECURITY;
ALTER TABLE plans ENABLE ROW LEVEL SECURITY;
ALTER TABLE subscriptions ENABLE ROW LEVEL SECURITY;
ALTER TABLE routers ENABLE ROW LEVEL SECURITY;
ALTER TABLE invoices ENABLE ROW LEVEL SECURITY;
ALTER TABLE payments ENABLE ROW LEVEL SECURITY;
ALTER TABLE tickets ENABLE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation_customers ON customers
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::bigint);
CREATE POLICY tenant_isolation_plans ON plans
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::bigint);
CREATE POLICY tenant_isolation_subscriptions ON subscriptions
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::bigint);
CREATE POLICY tenant_isolation_routers ON routers
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::bigint);
CREATE POLICY tenant_isolation_invoices ON invoices
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::bigint);
CREATE POLICY tenant_isolation_payments ON payments
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::bigint);
CREATE POLICY tenant_isolation_tickets ON tickets
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::bigint);

CREATE INDEX idx_customers_tenant ON customers(tenant_id);
CREATE INDEX idx_subscriptions_tenant ON subscriptions(tenant_id);
CREATE INDEX idx_subscriptions_status ON subscriptions(tenant_id, status);
CREATE INDEX idx_invoices_tenant_status ON invoices(tenant_id, status);
CREATE INDEX idx_invoices_due ON invoices(tenant_id, due_date);
CREATE INDEX idx_payments_tenant ON payments(tenant_id);
CREATE INDEX idx_router_metrics_router_time ON router_metrics(router_id, recorded_at DESC);
CREATE INDEX idx_traffic_samples_sub_time ON traffic_samples(subscription_id, recorded_at DESC);
CREATE INDEX idx_notification_queue_pending ON notification_queue(status, scheduled_at) WHERE status = 'pending';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS resellers, leads, refresh_tokens, settings, api_keys, audit_logs;
DROP TABLE IF EXISTS router_backups, router_command_logs, service_sessions, traffic_samples, router_metrics;
DROP TABLE IF EXISTS notification_queue, notification_templates, expenses, journal_lines, journal_entries;
DROP TABLE IF EXISTS chart_of_accounts, odp_ports, odps, odcs, work_orders, technicians;
DROP TABLE IF EXISTS ticket_messages, tickets, vouchers, voucher_batches, wallet_transactions, wallets;
DROP TABLE IF EXISTS payment_intents, payments, invoice_items, invoices, subscriptions, plans;
DROP TABLE IF EXISTS ip_assignments, ip_pools, routers, sites, customer_addresses, customers;
DROP TABLE IF EXISTS user_tenants, users, roles, tenants;
-- +goose StatementEnd
