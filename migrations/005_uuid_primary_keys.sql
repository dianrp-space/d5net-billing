-- +goose Up
-- +goose StatementBegin
-- Convert all app table PKs/FKs from BIGINT to UUID. FreeRADIUS tables unchanged.

-- 1) Drop RLS policies that cast app.tenant_id to bigint
DROP POLICY IF EXISTS tenant_isolation_customers ON customers;
DROP POLICY IF EXISTS tenant_isolation_plans ON plans;
DROP POLICY IF EXISTS tenant_isolation_subscriptions ON subscriptions;
DROP POLICY IF EXISTS tenant_isolation_routers ON routers;
DROP POLICY IF EXISTS tenant_isolation_invoices ON invoices;
DROP POLICY IF EXISTS tenant_isolation_payments ON payments;
DROP POLICY IF EXISTS tenant_isolation_tickets ON tickets;

-- 2) Drop all foreign key constraints on app tables
DO $$
DECLARE r RECORD;
BEGIN
  FOR r IN
    SELECT conname, conrelid::regclass AS tbl
    FROM pg_constraint
    WHERE contype = 'f'
      AND conrelid::regclass::text = ANY (ARRAY[
        'tenants',
        'roles',
        'users',
        'user_tenants',
        'customers',
        'customer_addresses',
        'sites',
        'routers',
        'ip_pools',
        'ip_assignments',
        'plans',
        'subscriptions',
        'invoices',
        'invoice_items',
        'payments',
        'payment_intents',
        'wallets',
        'wallet_transactions',
        'voucher_batches',
        'vouchers',
        'tickets',
        'ticket_messages',
        'technicians',
        'work_orders',
        'odcs',
        'odps',
        'odp_ports',
        'chart_of_accounts',
        'journal_entries',
        'journal_lines',
        'expenses',
        'notification_templates',
        'notification_queue',
        'router_metrics',
        'traffic_samples',
        'service_sessions',
        'router_command_logs',
        'router_backups',
        'audit_logs',
        'api_keys',
        'settings',
        'refresh_tokens',
        'leads',
        'resellers',
        'job_runs',
        'outbound_webhooks',
        'outbound_webhook_deliveries',
        'alerts'
      ]::text[])
  LOOP
    EXECUTE format('ALTER TABLE %s DROP CONSTRAINT %I', r.tbl, r.conname);
  END LOOP;
END $$;

-- 3) Drop primary keys
DO $$
DECLARE r RECORD;
BEGIN
  FOR r IN
    SELECT conname, conrelid::regclass AS tbl
    FROM pg_constraint
    WHERE contype = 'p'
      AND conrelid::regclass::text = ANY (ARRAY[
        'tenants',
        'roles',
        'users',
        'user_tenants',
        'customers',
        'customer_addresses',
        'sites',
        'routers',
        'ip_pools',
        'ip_assignments',
        'plans',
        'subscriptions',
        'invoices',
        'invoice_items',
        'payments',
        'payment_intents',
        'wallets',
        'wallet_transactions',
        'voucher_batches',
        'vouchers',
        'tickets',
        'ticket_messages',
        'technicians',
        'work_orders',
        'odcs',
        'odps',
        'odp_ports',
        'chart_of_accounts',
        'journal_entries',
        'journal_lines',
        'expenses',
        'notification_templates',
        'notification_queue',
        'router_metrics',
        'traffic_samples',
        'service_sessions',
        'router_command_logs',
        'router_backups',
        'audit_logs',
        'api_keys',
        'settings',
        'refresh_tokens',
        'leads',
        'resellers',
        'job_runs',
        'outbound_webhooks',
        'outbound_webhook_deliveries',
        'alerts'
      ]::text[])
  LOOP
    EXECUTE format('ALTER TABLE %s DROP CONSTRAINT %I', r.tbl, r.conname);
  END LOOP;
END $$;

-- 4) Add id_new UUID for every table
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS id_new UUID NOT NULL DEFAULT gen_random_uuid();
ALTER TABLE roles ADD COLUMN IF NOT EXISTS id_new UUID NOT NULL DEFAULT gen_random_uuid();
ALTER TABLE users ADD COLUMN IF NOT EXISTS id_new UUID NOT NULL DEFAULT gen_random_uuid();
ALTER TABLE user_tenants ADD COLUMN IF NOT EXISTS id_new UUID NOT NULL DEFAULT gen_random_uuid();
ALTER TABLE customers ADD COLUMN IF NOT EXISTS id_new UUID NOT NULL DEFAULT gen_random_uuid();
ALTER TABLE customer_addresses ADD COLUMN IF NOT EXISTS id_new UUID NOT NULL DEFAULT gen_random_uuid();
ALTER TABLE sites ADD COLUMN IF NOT EXISTS id_new UUID NOT NULL DEFAULT gen_random_uuid();
ALTER TABLE routers ADD COLUMN IF NOT EXISTS id_new UUID NOT NULL DEFAULT gen_random_uuid();
ALTER TABLE ip_pools ADD COLUMN IF NOT EXISTS id_new UUID NOT NULL DEFAULT gen_random_uuid();
ALTER TABLE ip_assignments ADD COLUMN IF NOT EXISTS id_new UUID NOT NULL DEFAULT gen_random_uuid();
ALTER TABLE plans ADD COLUMN IF NOT EXISTS id_new UUID NOT NULL DEFAULT gen_random_uuid();
ALTER TABLE subscriptions ADD COLUMN IF NOT EXISTS id_new UUID NOT NULL DEFAULT gen_random_uuid();
ALTER TABLE invoices ADD COLUMN IF NOT EXISTS id_new UUID NOT NULL DEFAULT gen_random_uuid();
ALTER TABLE invoice_items ADD COLUMN IF NOT EXISTS id_new UUID NOT NULL DEFAULT gen_random_uuid();
ALTER TABLE payments ADD COLUMN IF NOT EXISTS id_new UUID NOT NULL DEFAULT gen_random_uuid();
ALTER TABLE payment_intents ADD COLUMN IF NOT EXISTS id_new UUID NOT NULL DEFAULT gen_random_uuid();
ALTER TABLE wallets ADD COLUMN IF NOT EXISTS id_new UUID NOT NULL DEFAULT gen_random_uuid();
ALTER TABLE wallet_transactions ADD COLUMN IF NOT EXISTS id_new UUID NOT NULL DEFAULT gen_random_uuid();
ALTER TABLE voucher_batches ADD COLUMN IF NOT EXISTS id_new UUID NOT NULL DEFAULT gen_random_uuid();
ALTER TABLE vouchers ADD COLUMN IF NOT EXISTS id_new UUID NOT NULL DEFAULT gen_random_uuid();
ALTER TABLE tickets ADD COLUMN IF NOT EXISTS id_new UUID NOT NULL DEFAULT gen_random_uuid();
ALTER TABLE ticket_messages ADD COLUMN IF NOT EXISTS id_new UUID NOT NULL DEFAULT gen_random_uuid();
ALTER TABLE technicians ADD COLUMN IF NOT EXISTS id_new UUID NOT NULL DEFAULT gen_random_uuid();
ALTER TABLE work_orders ADD COLUMN IF NOT EXISTS id_new UUID NOT NULL DEFAULT gen_random_uuid();
ALTER TABLE odcs ADD COLUMN IF NOT EXISTS id_new UUID NOT NULL DEFAULT gen_random_uuid();
ALTER TABLE odps ADD COLUMN IF NOT EXISTS id_new UUID NOT NULL DEFAULT gen_random_uuid();
ALTER TABLE odp_ports ADD COLUMN IF NOT EXISTS id_new UUID NOT NULL DEFAULT gen_random_uuid();
ALTER TABLE chart_of_accounts ADD COLUMN IF NOT EXISTS id_new UUID NOT NULL DEFAULT gen_random_uuid();
ALTER TABLE journal_entries ADD COLUMN IF NOT EXISTS id_new UUID NOT NULL DEFAULT gen_random_uuid();
ALTER TABLE journal_lines ADD COLUMN IF NOT EXISTS id_new UUID NOT NULL DEFAULT gen_random_uuid();
ALTER TABLE expenses ADD COLUMN IF NOT EXISTS id_new UUID NOT NULL DEFAULT gen_random_uuid();
ALTER TABLE notification_templates ADD COLUMN IF NOT EXISTS id_new UUID NOT NULL DEFAULT gen_random_uuid();
ALTER TABLE notification_queue ADD COLUMN IF NOT EXISTS id_new UUID NOT NULL DEFAULT gen_random_uuid();
ALTER TABLE router_metrics ADD COLUMN IF NOT EXISTS id_new UUID NOT NULL DEFAULT gen_random_uuid();
ALTER TABLE traffic_samples ADD COLUMN IF NOT EXISTS id_new UUID NOT NULL DEFAULT gen_random_uuid();
ALTER TABLE service_sessions ADD COLUMN IF NOT EXISTS id_new UUID NOT NULL DEFAULT gen_random_uuid();
ALTER TABLE router_command_logs ADD COLUMN IF NOT EXISTS id_new UUID NOT NULL DEFAULT gen_random_uuid();
ALTER TABLE router_backups ADD COLUMN IF NOT EXISTS id_new UUID NOT NULL DEFAULT gen_random_uuid();
ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS id_new UUID NOT NULL DEFAULT gen_random_uuid();
ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS id_new UUID NOT NULL DEFAULT gen_random_uuid();
ALTER TABLE settings ADD COLUMN IF NOT EXISTS id_new UUID NOT NULL DEFAULT gen_random_uuid();
ALTER TABLE refresh_tokens ADD COLUMN IF NOT EXISTS id_new UUID NOT NULL DEFAULT gen_random_uuid();
ALTER TABLE leads ADD COLUMN IF NOT EXISTS id_new UUID NOT NULL DEFAULT gen_random_uuid();
ALTER TABLE resellers ADD COLUMN IF NOT EXISTS id_new UUID NOT NULL DEFAULT gen_random_uuid();
ALTER TABLE job_runs ADD COLUMN IF NOT EXISTS id_new UUID NOT NULL DEFAULT gen_random_uuid();
ALTER TABLE outbound_webhooks ADD COLUMN IF NOT EXISTS id_new UUID NOT NULL DEFAULT gen_random_uuid();
ALTER TABLE outbound_webhook_deliveries ADD COLUMN IF NOT EXISTS id_new UUID NOT NULL DEFAULT gen_random_uuid();
ALTER TABLE alerts ADD COLUMN IF NOT EXISTS id_new UUID NOT NULL DEFAULT gen_random_uuid();

-- 5) Add and populate new FK UUID columns
ALTER TABLE roles ADD COLUMN IF NOT EXISTS tenant_id_new UUID;
UPDATE roles c SET tenant_id_new = p.id_new FROM tenants p WHERE c.tenant_id = p.id;
ALTER TABLE user_tenants ADD COLUMN IF NOT EXISTS user_id_new UUID;
UPDATE user_tenants c SET user_id_new = p.id_new FROM users p WHERE c.user_id = p.id;
ALTER TABLE user_tenants ADD COLUMN IF NOT EXISTS tenant_id_new UUID;
UPDATE user_tenants c SET tenant_id_new = p.id_new FROM tenants p WHERE c.tenant_id = p.id;
ALTER TABLE user_tenants ADD COLUMN IF NOT EXISTS role_id_new UUID;
UPDATE user_tenants c SET role_id_new = p.id_new FROM roles p WHERE c.role_id = p.id;
ALTER TABLE customers ADD COLUMN IF NOT EXISTS tenant_id_new UUID;
UPDATE customers c SET tenant_id_new = p.id_new FROM tenants p WHERE c.tenant_id = p.id;
ALTER TABLE customer_addresses ADD COLUMN IF NOT EXISTS tenant_id_new UUID;
UPDATE customer_addresses c SET tenant_id_new = p.id_new FROM tenants p WHERE c.tenant_id = p.id;
ALTER TABLE customer_addresses ADD COLUMN IF NOT EXISTS customer_id_new UUID;
UPDATE customer_addresses c SET customer_id_new = p.id_new FROM customers p WHERE c.customer_id = p.id;
ALTER TABLE sites ADD COLUMN IF NOT EXISTS tenant_id_new UUID;
UPDATE sites c SET tenant_id_new = p.id_new FROM tenants p WHERE c.tenant_id = p.id;
ALTER TABLE routers ADD COLUMN IF NOT EXISTS tenant_id_new UUID;
UPDATE routers c SET tenant_id_new = p.id_new FROM tenants p WHERE c.tenant_id = p.id;
ALTER TABLE routers ADD COLUMN IF NOT EXISTS site_id_new UUID;
UPDATE routers c SET site_id_new = p.id_new FROM sites p WHERE c.site_id = p.id;
ALTER TABLE ip_pools ADD COLUMN IF NOT EXISTS tenant_id_new UUID;
UPDATE ip_pools c SET tenant_id_new = p.id_new FROM tenants p WHERE c.tenant_id = p.id;
ALTER TABLE ip_pools ADD COLUMN IF NOT EXISTS router_id_new UUID;
UPDATE ip_pools c SET router_id_new = p.id_new FROM routers p WHERE c.router_id = p.id;
ALTER TABLE ip_assignments ADD COLUMN IF NOT EXISTS tenant_id_new UUID;
UPDATE ip_assignments c SET tenant_id_new = p.id_new FROM tenants p WHERE c.tenant_id = p.id;
ALTER TABLE ip_assignments ADD COLUMN IF NOT EXISTS pool_id_new UUID;
UPDATE ip_assignments c SET pool_id_new = p.id_new FROM ip_pools p WHERE c.pool_id = p.id;
ALTER TABLE ip_assignments ADD COLUMN IF NOT EXISTS customer_id_new UUID;
UPDATE ip_assignments c SET customer_id_new = p.id_new FROM customers p WHERE c.customer_id = p.id;
ALTER TABLE plans ADD COLUMN IF NOT EXISTS tenant_id_new UUID;
UPDATE plans c SET tenant_id_new = p.id_new FROM tenants p WHERE c.tenant_id = p.id;
ALTER TABLE subscriptions ADD COLUMN IF NOT EXISTS tenant_id_new UUID;
UPDATE subscriptions c SET tenant_id_new = p.id_new FROM tenants p WHERE c.tenant_id = p.id;
ALTER TABLE subscriptions ADD COLUMN IF NOT EXISTS customer_id_new UUID;
UPDATE subscriptions c SET customer_id_new = p.id_new FROM customers p WHERE c.customer_id = p.id;
ALTER TABLE subscriptions ADD COLUMN IF NOT EXISTS plan_id_new UUID;
UPDATE subscriptions c SET plan_id_new = p.id_new FROM plans p WHERE c.plan_id = p.id;
ALTER TABLE subscriptions ADD COLUMN IF NOT EXISTS router_id_new UUID;
UPDATE subscriptions c SET router_id_new = p.id_new FROM routers p WHERE c.router_id = p.id;
ALTER TABLE invoices ADD COLUMN IF NOT EXISTS tenant_id_new UUID;
UPDATE invoices c SET tenant_id_new = p.id_new FROM tenants p WHERE c.tenant_id = p.id;
ALTER TABLE invoices ADD COLUMN IF NOT EXISTS customer_id_new UUID;
UPDATE invoices c SET customer_id_new = p.id_new FROM customers p WHERE c.customer_id = p.id;
ALTER TABLE invoices ADD COLUMN IF NOT EXISTS subscription_id_new UUID;
UPDATE invoices c SET subscription_id_new = p.id_new FROM subscriptions p WHERE c.subscription_id = p.id;
ALTER TABLE invoice_items ADD COLUMN IF NOT EXISTS tenant_id_new UUID;
UPDATE invoice_items c SET tenant_id_new = p.id_new FROM tenants p WHERE c.tenant_id = p.id;
ALTER TABLE invoice_items ADD COLUMN IF NOT EXISTS invoice_id_new UUID;
UPDATE invoice_items c SET invoice_id_new = p.id_new FROM invoices p WHERE c.invoice_id = p.id;
ALTER TABLE payments ADD COLUMN IF NOT EXISTS tenant_id_new UUID;
UPDATE payments c SET tenant_id_new = p.id_new FROM tenants p WHERE c.tenant_id = p.id;
ALTER TABLE payments ADD COLUMN IF NOT EXISTS customer_id_new UUID;
UPDATE payments c SET customer_id_new = p.id_new FROM customers p WHERE c.customer_id = p.id;
ALTER TABLE payments ADD COLUMN IF NOT EXISTS invoice_id_new UUID;
UPDATE payments c SET invoice_id_new = p.id_new FROM invoices p WHERE c.invoice_id = p.id;
ALTER TABLE payment_intents ADD COLUMN IF NOT EXISTS tenant_id_new UUID;
UPDATE payment_intents c SET tenant_id_new = p.id_new FROM tenants p WHERE c.tenant_id = p.id;
ALTER TABLE payment_intents ADD COLUMN IF NOT EXISTS customer_id_new UUID;
UPDATE payment_intents c SET customer_id_new = p.id_new FROM customers p WHERE c.customer_id = p.id;
ALTER TABLE payment_intents ADD COLUMN IF NOT EXISTS invoice_id_new UUID;
UPDATE payment_intents c SET invoice_id_new = p.id_new FROM invoices p WHERE c.invoice_id = p.id;
ALTER TABLE wallets ADD COLUMN IF NOT EXISTS tenant_id_new UUID;
UPDATE wallets c SET tenant_id_new = p.id_new FROM tenants p WHERE c.tenant_id = p.id;
ALTER TABLE wallets ADD COLUMN IF NOT EXISTS customer_id_new UUID;
UPDATE wallets c SET customer_id_new = p.id_new FROM customers p WHERE c.customer_id = p.id;
ALTER TABLE wallet_transactions ADD COLUMN IF NOT EXISTS tenant_id_new UUID;
UPDATE wallet_transactions c SET tenant_id_new = p.id_new FROM tenants p WHERE c.tenant_id = p.id;
ALTER TABLE wallet_transactions ADD COLUMN IF NOT EXISTS wallet_id_new UUID;
UPDATE wallet_transactions c SET wallet_id_new = p.id_new FROM wallets p WHERE c.wallet_id = p.id;
ALTER TABLE voucher_batches ADD COLUMN IF NOT EXISTS tenant_id_new UUID;
UPDATE voucher_batches c SET tenant_id_new = p.id_new FROM tenants p WHERE c.tenant_id = p.id;
ALTER TABLE voucher_batches ADD COLUMN IF NOT EXISTS plan_id_new UUID;
UPDATE voucher_batches c SET plan_id_new = p.id_new FROM plans p WHERE c.plan_id = p.id;
ALTER TABLE vouchers ADD COLUMN IF NOT EXISTS tenant_id_new UUID;
UPDATE vouchers c SET tenant_id_new = p.id_new FROM tenants p WHERE c.tenant_id = p.id;
ALTER TABLE vouchers ADD COLUMN IF NOT EXISTS batch_id_new UUID;
UPDATE vouchers c SET batch_id_new = p.id_new FROM voucher_batches p WHERE c.batch_id = p.id;
ALTER TABLE vouchers ADD COLUMN IF NOT EXISTS used_by_new UUID;
UPDATE vouchers c SET used_by_new = p.id_new FROM customers p WHERE c.used_by = p.id;
ALTER TABLE tickets ADD COLUMN IF NOT EXISTS tenant_id_new UUID;
UPDATE tickets c SET tenant_id_new = p.id_new FROM tenants p WHERE c.tenant_id = p.id;
ALTER TABLE tickets ADD COLUMN IF NOT EXISTS customer_id_new UUID;
UPDATE tickets c SET customer_id_new = p.id_new FROM customers p WHERE c.customer_id = p.id;
ALTER TABLE tickets ADD COLUMN IF NOT EXISTS assigned_to_new UUID;
UPDATE tickets c SET assigned_to_new = p.id_new FROM users p WHERE c.assigned_to = p.id;
ALTER TABLE ticket_messages ADD COLUMN IF NOT EXISTS tenant_id_new UUID;
UPDATE ticket_messages c SET tenant_id_new = p.id_new FROM tenants p WHERE c.tenant_id = p.id;
ALTER TABLE ticket_messages ADD COLUMN IF NOT EXISTS ticket_id_new UUID;
UPDATE ticket_messages c SET ticket_id_new = p.id_new FROM tickets p WHERE c.ticket_id = p.id;
ALTER TABLE technicians ADD COLUMN IF NOT EXISTS tenant_id_new UUID;
UPDATE technicians c SET tenant_id_new = p.id_new FROM tenants p WHERE c.tenant_id = p.id;
ALTER TABLE technicians ADD COLUMN IF NOT EXISTS user_id_new UUID;
UPDATE technicians c SET user_id_new = p.id_new FROM users p WHERE c.user_id = p.id;
ALTER TABLE work_orders ADD COLUMN IF NOT EXISTS tenant_id_new UUID;
UPDATE work_orders c SET tenant_id_new = p.id_new FROM tenants p WHERE c.tenant_id = p.id;
ALTER TABLE work_orders ADD COLUMN IF NOT EXISTS customer_id_new UUID;
UPDATE work_orders c SET customer_id_new = p.id_new FROM customers p WHERE c.customer_id = p.id;
ALTER TABLE work_orders ADD COLUMN IF NOT EXISTS subscription_id_new UUID;
UPDATE work_orders c SET subscription_id_new = p.id_new FROM subscriptions p WHERE c.subscription_id = p.id;
ALTER TABLE work_orders ADD COLUMN IF NOT EXISTS technician_id_new UUID;
UPDATE work_orders c SET technician_id_new = p.id_new FROM technicians p WHERE c.technician_id = p.id;
ALTER TABLE odcs ADD COLUMN IF NOT EXISTS tenant_id_new UUID;
UPDATE odcs c SET tenant_id_new = p.id_new FROM tenants p WHERE c.tenant_id = p.id;
ALTER TABLE odcs ADD COLUMN IF NOT EXISTS site_id_new UUID;
UPDATE odcs c SET site_id_new = p.id_new FROM sites p WHERE c.site_id = p.id;
ALTER TABLE odps ADD COLUMN IF NOT EXISTS tenant_id_new UUID;
UPDATE odps c SET tenant_id_new = p.id_new FROM tenants p WHERE c.tenant_id = p.id;
ALTER TABLE odps ADD COLUMN IF NOT EXISTS odc_id_new UUID;
UPDATE odps c SET odc_id_new = p.id_new FROM odcs p WHERE c.odc_id = p.id;
ALTER TABLE odp_ports ADD COLUMN IF NOT EXISTS tenant_id_new UUID;
UPDATE odp_ports c SET tenant_id_new = p.id_new FROM tenants p WHERE c.tenant_id = p.id;
ALTER TABLE odp_ports ADD COLUMN IF NOT EXISTS odp_id_new UUID;
UPDATE odp_ports c SET odp_id_new = p.id_new FROM odps p WHERE c.odp_id = p.id;
ALTER TABLE odp_ports ADD COLUMN IF NOT EXISTS customer_id_new UUID;
UPDATE odp_ports c SET customer_id_new = p.id_new FROM customers p WHERE c.customer_id = p.id;
ALTER TABLE odp_ports ADD COLUMN IF NOT EXISTS subscription_id_new UUID;
UPDATE odp_ports c SET subscription_id_new = p.id_new FROM subscriptions p WHERE c.subscription_id = p.id;
ALTER TABLE chart_of_accounts ADD COLUMN IF NOT EXISTS tenant_id_new UUID;
UPDATE chart_of_accounts c SET tenant_id_new = p.id_new FROM tenants p WHERE c.tenant_id = p.id;
ALTER TABLE journal_entries ADD COLUMN IF NOT EXISTS tenant_id_new UUID;
UPDATE journal_entries c SET tenant_id_new = p.id_new FROM tenants p WHERE c.tenant_id = p.id;
ALTER TABLE journal_lines ADD COLUMN IF NOT EXISTS tenant_id_new UUID;
UPDATE journal_lines c SET tenant_id_new = p.id_new FROM tenants p WHERE c.tenant_id = p.id;
ALTER TABLE journal_lines ADD COLUMN IF NOT EXISTS entry_id_new UUID;
UPDATE journal_lines c SET entry_id_new = p.id_new FROM journal_entries p WHERE c.entry_id = p.id;
ALTER TABLE journal_lines ADD COLUMN IF NOT EXISTS account_id_new UUID;
UPDATE journal_lines c SET account_id_new = p.id_new FROM chart_of_accounts p WHERE c.account_id = p.id;
ALTER TABLE expenses ADD COLUMN IF NOT EXISTS tenant_id_new UUID;
UPDATE expenses c SET tenant_id_new = p.id_new FROM tenants p WHERE c.tenant_id = p.id;
ALTER TABLE expenses ADD COLUMN IF NOT EXISTS account_id_new UUID;
UPDATE expenses c SET account_id_new = p.id_new FROM chart_of_accounts p WHERE c.account_id = p.id;
ALTER TABLE notification_templates ADD COLUMN IF NOT EXISTS tenant_id_new UUID;
UPDATE notification_templates c SET tenant_id_new = p.id_new FROM tenants p WHERE c.tenant_id = p.id;
ALTER TABLE notification_queue ADD COLUMN IF NOT EXISTS tenant_id_new UUID;
UPDATE notification_queue c SET tenant_id_new = p.id_new FROM tenants p WHERE c.tenant_id = p.id;
ALTER TABLE router_metrics ADD COLUMN IF NOT EXISTS tenant_id_new UUID;
UPDATE router_metrics c SET tenant_id_new = p.id_new FROM tenants p WHERE c.tenant_id = p.id;
ALTER TABLE router_metrics ADD COLUMN IF NOT EXISTS router_id_new UUID;
UPDATE router_metrics c SET router_id_new = p.id_new FROM routers p WHERE c.router_id = p.id;
ALTER TABLE traffic_samples ADD COLUMN IF NOT EXISTS tenant_id_new UUID;
UPDATE traffic_samples c SET tenant_id_new = p.id_new FROM tenants p WHERE c.tenant_id = p.id;
ALTER TABLE traffic_samples ADD COLUMN IF NOT EXISTS router_id_new UUID;
UPDATE traffic_samples c SET router_id_new = p.id_new FROM routers p WHERE c.router_id = p.id;
ALTER TABLE traffic_samples ADD COLUMN IF NOT EXISTS subscription_id_new UUID;
UPDATE traffic_samples c SET subscription_id_new = p.id_new FROM subscriptions p WHERE c.subscription_id = p.id;
ALTER TABLE service_sessions ADD COLUMN IF NOT EXISTS tenant_id_new UUID;
UPDATE service_sessions c SET tenant_id_new = p.id_new FROM tenants p WHERE c.tenant_id = p.id;
ALTER TABLE service_sessions ADD COLUMN IF NOT EXISTS subscription_id_new UUID;
UPDATE service_sessions c SET subscription_id_new = p.id_new FROM subscriptions p WHERE c.subscription_id = p.id;
ALTER TABLE service_sessions ADD COLUMN IF NOT EXISTS router_id_new UUID;
UPDATE service_sessions c SET router_id_new = p.id_new FROM routers p WHERE c.router_id = p.id;
ALTER TABLE router_command_logs ADD COLUMN IF NOT EXISTS tenant_id_new UUID;
UPDATE router_command_logs c SET tenant_id_new = p.id_new FROM tenants p WHERE c.tenant_id = p.id;
ALTER TABLE router_command_logs ADD COLUMN IF NOT EXISTS router_id_new UUID;
UPDATE router_command_logs c SET router_id_new = p.id_new FROM routers p WHERE c.router_id = p.id;
ALTER TABLE router_command_logs ADD COLUMN IF NOT EXISTS user_id_new UUID;
UPDATE router_command_logs c SET user_id_new = p.id_new FROM users p WHERE c.user_id = p.id;
ALTER TABLE router_backups ADD COLUMN IF NOT EXISTS tenant_id_new UUID;
UPDATE router_backups c SET tenant_id_new = p.id_new FROM tenants p WHERE c.tenant_id = p.id;
ALTER TABLE router_backups ADD COLUMN IF NOT EXISTS router_id_new UUID;
UPDATE router_backups c SET router_id_new = p.id_new FROM routers p WHERE c.router_id = p.id;
ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS tenant_id_new UUID;
UPDATE audit_logs c SET tenant_id_new = p.id_new FROM tenants p WHERE c.tenant_id = p.id;
ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS user_id_new UUID;
UPDATE audit_logs c SET user_id_new = p.id_new FROM users p WHERE c.user_id = p.id;
ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS tenant_id_new UUID;
UPDATE api_keys c SET tenant_id_new = p.id_new FROM tenants p WHERE c.tenant_id = p.id;
ALTER TABLE settings ADD COLUMN IF NOT EXISTS tenant_id_new UUID;
UPDATE settings c SET tenant_id_new = p.id_new FROM tenants p WHERE c.tenant_id = p.id;
ALTER TABLE refresh_tokens ADD COLUMN IF NOT EXISTS user_id_new UUID;
UPDATE refresh_tokens c SET user_id_new = p.id_new FROM users p WHERE c.user_id = p.id;
ALTER TABLE leads ADD COLUMN IF NOT EXISTS tenant_id_new UUID;
UPDATE leads c SET tenant_id_new = p.id_new FROM tenants p WHERE c.tenant_id = p.id;
ALTER TABLE leads ADD COLUMN IF NOT EXISTS odp_id_new UUID;
UPDATE leads c SET odp_id_new = p.id_new FROM odps p WHERE c.odp_id = p.id;
ALTER TABLE resellers ADD COLUMN IF NOT EXISTS tenant_id_new UUID;
UPDATE resellers c SET tenant_id_new = p.id_new FROM tenants p WHERE c.tenant_id = p.id;
ALTER TABLE resellers ADD COLUMN IF NOT EXISTS user_id_new UUID;
UPDATE resellers c SET user_id_new = p.id_new FROM users p WHERE c.user_id = p.id;
ALTER TABLE job_runs ADD COLUMN IF NOT EXISTS tenant_id_new UUID;
UPDATE job_runs c SET tenant_id_new = p.id_new FROM tenants p WHERE c.tenant_id = p.id;
ALTER TABLE outbound_webhooks ADD COLUMN IF NOT EXISTS tenant_id_new UUID;
UPDATE outbound_webhooks c SET tenant_id_new = p.id_new FROM tenants p WHERE c.tenant_id = p.id;
ALTER TABLE outbound_webhook_deliveries ADD COLUMN IF NOT EXISTS tenant_id_new UUID;
UPDATE outbound_webhook_deliveries c SET tenant_id_new = p.id_new FROM tenants p WHERE c.tenant_id = p.id;
ALTER TABLE outbound_webhook_deliveries ADD COLUMN IF NOT EXISTS webhook_id_new UUID;
UPDATE outbound_webhook_deliveries c SET webhook_id_new = p.id_new FROM outbound_webhooks p WHERE c.webhook_id = p.id;
ALTER TABLE alerts ADD COLUMN IF NOT EXISTS tenant_id_new UUID;
UPDATE alerts c SET tenant_id_new = p.id_new FROM tenants p WHERE c.tenant_id = p.id;

-- Soft FK mappings (no constraint originally)
ALTER TABLE ip_assignments ADD COLUMN IF NOT EXISTS subscription_id_new UUID;
UPDATE ip_assignments c SET subscription_id_new = p.id_new FROM subscriptions p WHERE c.subscription_id IS NOT NULL AND c.subscription_id = p.id;
ALTER TABLE ticket_messages ADD COLUMN IF NOT EXISTS sender_id_new UUID;
UPDATE ticket_messages c SET sender_id_new = p.id_new FROM users p WHERE c.sender_id IS NOT NULL AND c.sender_id = p.id;

-- Opaque entity id columns -> UUID (null out old bigint values)
ALTER TABLE journal_entries ADD COLUMN IF NOT EXISTS source_id_new UUID;
ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS entity_id_new UUID;
ALTER TABLE alerts ADD COLUMN IF NOT EXISTS entity_id_new UUID;

-- 6) Drop old bigint PK and FK columns
ALTER TABLE journal_entries DROP COLUMN IF EXISTS source_id;
ALTER TABLE journal_entries RENAME COLUMN source_id_new TO source_id;
ALTER TABLE audit_logs DROP COLUMN IF EXISTS entity_id;
ALTER TABLE audit_logs RENAME COLUMN entity_id_new TO entity_id;
ALTER TABLE alerts DROP COLUMN IF EXISTS entity_id;
ALTER TABLE alerts RENAME COLUMN entity_id_new TO entity_id;
ALTER TABLE ip_assignments DROP COLUMN IF EXISTS subscription_id;
ALTER TABLE ip_assignments RENAME COLUMN subscription_id_new TO subscription_id;
ALTER TABLE ticket_messages DROP COLUMN IF EXISTS sender_id;
ALTER TABLE ticket_messages RENAME COLUMN sender_id_new TO sender_id;
ALTER TABLE roles DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE roles RENAME COLUMN tenant_id_new TO tenant_id;
ALTER TABLE user_tenants DROP COLUMN IF EXISTS user_id;
ALTER TABLE user_tenants RENAME COLUMN user_id_new TO user_id;
ALTER TABLE user_tenants DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE user_tenants RENAME COLUMN tenant_id_new TO tenant_id;
ALTER TABLE user_tenants DROP COLUMN IF EXISTS role_id;
ALTER TABLE user_tenants RENAME COLUMN role_id_new TO role_id;
ALTER TABLE customers DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE customers RENAME COLUMN tenant_id_new TO tenant_id;
ALTER TABLE customer_addresses DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE customer_addresses RENAME COLUMN tenant_id_new TO tenant_id;
ALTER TABLE customer_addresses DROP COLUMN IF EXISTS customer_id;
ALTER TABLE customer_addresses RENAME COLUMN customer_id_new TO customer_id;
ALTER TABLE sites DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE sites RENAME COLUMN tenant_id_new TO tenant_id;
ALTER TABLE routers DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE routers RENAME COLUMN tenant_id_new TO tenant_id;
ALTER TABLE routers DROP COLUMN IF EXISTS site_id;
ALTER TABLE routers RENAME COLUMN site_id_new TO site_id;
ALTER TABLE ip_pools DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE ip_pools RENAME COLUMN tenant_id_new TO tenant_id;
ALTER TABLE ip_pools DROP COLUMN IF EXISTS router_id;
ALTER TABLE ip_pools RENAME COLUMN router_id_new TO router_id;
ALTER TABLE ip_assignments DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE ip_assignments RENAME COLUMN tenant_id_new TO tenant_id;
ALTER TABLE ip_assignments DROP COLUMN IF EXISTS pool_id;
ALTER TABLE ip_assignments RENAME COLUMN pool_id_new TO pool_id;
ALTER TABLE ip_assignments DROP COLUMN IF EXISTS customer_id;
ALTER TABLE ip_assignments RENAME COLUMN customer_id_new TO customer_id;
ALTER TABLE plans DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE plans RENAME COLUMN tenant_id_new TO tenant_id;
ALTER TABLE subscriptions DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE subscriptions RENAME COLUMN tenant_id_new TO tenant_id;
ALTER TABLE subscriptions DROP COLUMN IF EXISTS customer_id;
ALTER TABLE subscriptions RENAME COLUMN customer_id_new TO customer_id;
ALTER TABLE subscriptions DROP COLUMN IF EXISTS plan_id;
ALTER TABLE subscriptions RENAME COLUMN plan_id_new TO plan_id;
ALTER TABLE subscriptions DROP COLUMN IF EXISTS router_id;
ALTER TABLE subscriptions RENAME COLUMN router_id_new TO router_id;
ALTER TABLE invoices DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE invoices RENAME COLUMN tenant_id_new TO tenant_id;
ALTER TABLE invoices DROP COLUMN IF EXISTS customer_id;
ALTER TABLE invoices RENAME COLUMN customer_id_new TO customer_id;
ALTER TABLE invoices DROP COLUMN IF EXISTS subscription_id;
ALTER TABLE invoices RENAME COLUMN subscription_id_new TO subscription_id;
ALTER TABLE invoice_items DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE invoice_items RENAME COLUMN tenant_id_new TO tenant_id;
ALTER TABLE invoice_items DROP COLUMN IF EXISTS invoice_id;
ALTER TABLE invoice_items RENAME COLUMN invoice_id_new TO invoice_id;
ALTER TABLE payments DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE payments RENAME COLUMN tenant_id_new TO tenant_id;
ALTER TABLE payments DROP COLUMN IF EXISTS customer_id;
ALTER TABLE payments RENAME COLUMN customer_id_new TO customer_id;
ALTER TABLE payments DROP COLUMN IF EXISTS invoice_id;
ALTER TABLE payments RENAME COLUMN invoice_id_new TO invoice_id;
ALTER TABLE payment_intents DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE payment_intents RENAME COLUMN tenant_id_new TO tenant_id;
ALTER TABLE payment_intents DROP COLUMN IF EXISTS customer_id;
ALTER TABLE payment_intents RENAME COLUMN customer_id_new TO customer_id;
ALTER TABLE payment_intents DROP COLUMN IF EXISTS invoice_id;
ALTER TABLE payment_intents RENAME COLUMN invoice_id_new TO invoice_id;
ALTER TABLE wallets DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE wallets RENAME COLUMN tenant_id_new TO tenant_id;
ALTER TABLE wallets DROP COLUMN IF EXISTS customer_id;
ALTER TABLE wallets RENAME COLUMN customer_id_new TO customer_id;
ALTER TABLE wallet_transactions DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE wallet_transactions RENAME COLUMN tenant_id_new TO tenant_id;
ALTER TABLE wallet_transactions DROP COLUMN IF EXISTS wallet_id;
ALTER TABLE wallet_transactions RENAME COLUMN wallet_id_new TO wallet_id;
ALTER TABLE voucher_batches DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE voucher_batches RENAME COLUMN tenant_id_new TO tenant_id;
ALTER TABLE voucher_batches DROP COLUMN IF EXISTS plan_id;
ALTER TABLE voucher_batches RENAME COLUMN plan_id_new TO plan_id;
ALTER TABLE vouchers DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE vouchers RENAME COLUMN tenant_id_new TO tenant_id;
ALTER TABLE vouchers DROP COLUMN IF EXISTS batch_id;
ALTER TABLE vouchers RENAME COLUMN batch_id_new TO batch_id;
ALTER TABLE vouchers DROP COLUMN IF EXISTS used_by;
ALTER TABLE vouchers RENAME COLUMN used_by_new TO used_by;
ALTER TABLE tickets DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE tickets RENAME COLUMN tenant_id_new TO tenant_id;
ALTER TABLE tickets DROP COLUMN IF EXISTS customer_id;
ALTER TABLE tickets RENAME COLUMN customer_id_new TO customer_id;
ALTER TABLE tickets DROP COLUMN IF EXISTS assigned_to;
ALTER TABLE tickets RENAME COLUMN assigned_to_new TO assigned_to;
ALTER TABLE ticket_messages DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE ticket_messages RENAME COLUMN tenant_id_new TO tenant_id;
ALTER TABLE ticket_messages DROP COLUMN IF EXISTS ticket_id;
ALTER TABLE ticket_messages RENAME COLUMN ticket_id_new TO ticket_id;
ALTER TABLE technicians DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE technicians RENAME COLUMN tenant_id_new TO tenant_id;
ALTER TABLE technicians DROP COLUMN IF EXISTS user_id;
ALTER TABLE technicians RENAME COLUMN user_id_new TO user_id;
ALTER TABLE work_orders DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE work_orders RENAME COLUMN tenant_id_new TO tenant_id;
ALTER TABLE work_orders DROP COLUMN IF EXISTS customer_id;
ALTER TABLE work_orders RENAME COLUMN customer_id_new TO customer_id;
ALTER TABLE work_orders DROP COLUMN IF EXISTS subscription_id;
ALTER TABLE work_orders RENAME COLUMN subscription_id_new TO subscription_id;
ALTER TABLE work_orders DROP COLUMN IF EXISTS technician_id;
ALTER TABLE work_orders RENAME COLUMN technician_id_new TO technician_id;
ALTER TABLE odcs DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE odcs RENAME COLUMN tenant_id_new TO tenant_id;
ALTER TABLE odcs DROP COLUMN IF EXISTS site_id;
ALTER TABLE odcs RENAME COLUMN site_id_new TO site_id;
ALTER TABLE odps DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE odps RENAME COLUMN tenant_id_new TO tenant_id;
ALTER TABLE odps DROP COLUMN IF EXISTS odc_id;
ALTER TABLE odps RENAME COLUMN odc_id_new TO odc_id;
ALTER TABLE odp_ports DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE odp_ports RENAME COLUMN tenant_id_new TO tenant_id;
ALTER TABLE odp_ports DROP COLUMN IF EXISTS odp_id;
ALTER TABLE odp_ports RENAME COLUMN odp_id_new TO odp_id;
ALTER TABLE odp_ports DROP COLUMN IF EXISTS customer_id;
ALTER TABLE odp_ports RENAME COLUMN customer_id_new TO customer_id;
ALTER TABLE odp_ports DROP COLUMN IF EXISTS subscription_id;
ALTER TABLE odp_ports RENAME COLUMN subscription_id_new TO subscription_id;
ALTER TABLE chart_of_accounts DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE chart_of_accounts RENAME COLUMN tenant_id_new TO tenant_id;
ALTER TABLE journal_entries DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE journal_entries RENAME COLUMN tenant_id_new TO tenant_id;
ALTER TABLE journal_lines DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE journal_lines RENAME COLUMN tenant_id_new TO tenant_id;
ALTER TABLE journal_lines DROP COLUMN IF EXISTS entry_id;
ALTER TABLE journal_lines RENAME COLUMN entry_id_new TO entry_id;
ALTER TABLE journal_lines DROP COLUMN IF EXISTS account_id;
ALTER TABLE journal_lines RENAME COLUMN account_id_new TO account_id;
ALTER TABLE expenses DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE expenses RENAME COLUMN tenant_id_new TO tenant_id;
ALTER TABLE expenses DROP COLUMN IF EXISTS account_id;
ALTER TABLE expenses RENAME COLUMN account_id_new TO account_id;
ALTER TABLE notification_templates DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE notification_templates RENAME COLUMN tenant_id_new TO tenant_id;
ALTER TABLE notification_queue DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE notification_queue RENAME COLUMN tenant_id_new TO tenant_id;
ALTER TABLE router_metrics DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE router_metrics RENAME COLUMN tenant_id_new TO tenant_id;
ALTER TABLE router_metrics DROP COLUMN IF EXISTS router_id;
ALTER TABLE router_metrics RENAME COLUMN router_id_new TO router_id;
ALTER TABLE traffic_samples DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE traffic_samples RENAME COLUMN tenant_id_new TO tenant_id;
ALTER TABLE traffic_samples DROP COLUMN IF EXISTS router_id;
ALTER TABLE traffic_samples RENAME COLUMN router_id_new TO router_id;
ALTER TABLE traffic_samples DROP COLUMN IF EXISTS subscription_id;
ALTER TABLE traffic_samples RENAME COLUMN subscription_id_new TO subscription_id;
ALTER TABLE service_sessions DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE service_sessions RENAME COLUMN tenant_id_new TO tenant_id;
ALTER TABLE service_sessions DROP COLUMN IF EXISTS subscription_id;
ALTER TABLE service_sessions RENAME COLUMN subscription_id_new TO subscription_id;
ALTER TABLE service_sessions DROP COLUMN IF EXISTS router_id;
ALTER TABLE service_sessions RENAME COLUMN router_id_new TO router_id;
ALTER TABLE router_command_logs DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE router_command_logs RENAME COLUMN tenant_id_new TO tenant_id;
ALTER TABLE router_command_logs DROP COLUMN IF EXISTS router_id;
ALTER TABLE router_command_logs RENAME COLUMN router_id_new TO router_id;
ALTER TABLE router_command_logs DROP COLUMN IF EXISTS user_id;
ALTER TABLE router_command_logs RENAME COLUMN user_id_new TO user_id;
ALTER TABLE router_backups DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE router_backups RENAME COLUMN tenant_id_new TO tenant_id;
ALTER TABLE router_backups DROP COLUMN IF EXISTS router_id;
ALTER TABLE router_backups RENAME COLUMN router_id_new TO router_id;
ALTER TABLE audit_logs DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE audit_logs RENAME COLUMN tenant_id_new TO tenant_id;
ALTER TABLE audit_logs DROP COLUMN IF EXISTS user_id;
ALTER TABLE audit_logs RENAME COLUMN user_id_new TO user_id;
ALTER TABLE api_keys DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE api_keys RENAME COLUMN tenant_id_new TO tenant_id;
ALTER TABLE settings DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE settings RENAME COLUMN tenant_id_new TO tenant_id;
ALTER TABLE refresh_tokens DROP COLUMN IF EXISTS user_id;
ALTER TABLE refresh_tokens RENAME COLUMN user_id_new TO user_id;
ALTER TABLE leads DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE leads RENAME COLUMN tenant_id_new TO tenant_id;
ALTER TABLE leads DROP COLUMN IF EXISTS odp_id;
ALTER TABLE leads RENAME COLUMN odp_id_new TO odp_id;
ALTER TABLE resellers DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE resellers RENAME COLUMN tenant_id_new TO tenant_id;
ALTER TABLE resellers DROP COLUMN IF EXISTS user_id;
ALTER TABLE resellers RENAME COLUMN user_id_new TO user_id;
ALTER TABLE job_runs DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE job_runs RENAME COLUMN tenant_id_new TO tenant_id;
ALTER TABLE outbound_webhooks DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE outbound_webhooks RENAME COLUMN tenant_id_new TO tenant_id;
ALTER TABLE outbound_webhook_deliveries DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE outbound_webhook_deliveries RENAME COLUMN tenant_id_new TO tenant_id;
ALTER TABLE outbound_webhook_deliveries DROP COLUMN IF EXISTS webhook_id;
ALTER TABLE outbound_webhook_deliveries RENAME COLUMN webhook_id_new TO webhook_id;
ALTER TABLE alerts DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE alerts RENAME COLUMN tenant_id_new TO tenant_id;
ALTER TABLE tenants DROP COLUMN IF EXISTS id;
ALTER TABLE tenants RENAME COLUMN id_new TO id;
ALTER TABLE roles DROP COLUMN IF EXISTS id;
ALTER TABLE roles RENAME COLUMN id_new TO id;
ALTER TABLE users DROP COLUMN IF EXISTS id;
ALTER TABLE users RENAME COLUMN id_new TO id;
ALTER TABLE user_tenants DROP COLUMN IF EXISTS id;
ALTER TABLE user_tenants RENAME COLUMN id_new TO id;
ALTER TABLE customers DROP COLUMN IF EXISTS id;
ALTER TABLE customers RENAME COLUMN id_new TO id;
ALTER TABLE customer_addresses DROP COLUMN IF EXISTS id;
ALTER TABLE customer_addresses RENAME COLUMN id_new TO id;
ALTER TABLE sites DROP COLUMN IF EXISTS id;
ALTER TABLE sites RENAME COLUMN id_new TO id;
ALTER TABLE routers DROP COLUMN IF EXISTS id;
ALTER TABLE routers RENAME COLUMN id_new TO id;
ALTER TABLE ip_pools DROP COLUMN IF EXISTS id;
ALTER TABLE ip_pools RENAME COLUMN id_new TO id;
ALTER TABLE ip_assignments DROP COLUMN IF EXISTS id;
ALTER TABLE ip_assignments RENAME COLUMN id_new TO id;
ALTER TABLE plans DROP COLUMN IF EXISTS id;
ALTER TABLE plans RENAME COLUMN id_new TO id;
ALTER TABLE subscriptions DROP COLUMN IF EXISTS id;
ALTER TABLE subscriptions RENAME COLUMN id_new TO id;
ALTER TABLE invoices DROP COLUMN IF EXISTS id;
ALTER TABLE invoices RENAME COLUMN id_new TO id;
ALTER TABLE invoice_items DROP COLUMN IF EXISTS id;
ALTER TABLE invoice_items RENAME COLUMN id_new TO id;
ALTER TABLE payments DROP COLUMN IF EXISTS id;
ALTER TABLE payments RENAME COLUMN id_new TO id;
ALTER TABLE payment_intents DROP COLUMN IF EXISTS id;
ALTER TABLE payment_intents RENAME COLUMN id_new TO id;
ALTER TABLE wallets DROP COLUMN IF EXISTS id;
ALTER TABLE wallets RENAME COLUMN id_new TO id;
ALTER TABLE wallet_transactions DROP COLUMN IF EXISTS id;
ALTER TABLE wallet_transactions RENAME COLUMN id_new TO id;
ALTER TABLE voucher_batches DROP COLUMN IF EXISTS id;
ALTER TABLE voucher_batches RENAME COLUMN id_new TO id;
ALTER TABLE vouchers DROP COLUMN IF EXISTS id;
ALTER TABLE vouchers RENAME COLUMN id_new TO id;
ALTER TABLE tickets DROP COLUMN IF EXISTS id;
ALTER TABLE tickets RENAME COLUMN id_new TO id;
ALTER TABLE ticket_messages DROP COLUMN IF EXISTS id;
ALTER TABLE ticket_messages RENAME COLUMN id_new TO id;
ALTER TABLE technicians DROP COLUMN IF EXISTS id;
ALTER TABLE technicians RENAME COLUMN id_new TO id;
ALTER TABLE work_orders DROP COLUMN IF EXISTS id;
ALTER TABLE work_orders RENAME COLUMN id_new TO id;
ALTER TABLE odcs DROP COLUMN IF EXISTS id;
ALTER TABLE odcs RENAME COLUMN id_new TO id;
ALTER TABLE odps DROP COLUMN IF EXISTS id;
ALTER TABLE odps RENAME COLUMN id_new TO id;
ALTER TABLE odp_ports DROP COLUMN IF EXISTS id;
ALTER TABLE odp_ports RENAME COLUMN id_new TO id;
ALTER TABLE chart_of_accounts DROP COLUMN IF EXISTS id;
ALTER TABLE chart_of_accounts RENAME COLUMN id_new TO id;
ALTER TABLE journal_entries DROP COLUMN IF EXISTS id;
ALTER TABLE journal_entries RENAME COLUMN id_new TO id;
ALTER TABLE journal_lines DROP COLUMN IF EXISTS id;
ALTER TABLE journal_lines RENAME COLUMN id_new TO id;
ALTER TABLE expenses DROP COLUMN IF EXISTS id;
ALTER TABLE expenses RENAME COLUMN id_new TO id;
ALTER TABLE notification_templates DROP COLUMN IF EXISTS id;
ALTER TABLE notification_templates RENAME COLUMN id_new TO id;
ALTER TABLE notification_queue DROP COLUMN IF EXISTS id;
ALTER TABLE notification_queue RENAME COLUMN id_new TO id;
ALTER TABLE router_metrics DROP COLUMN IF EXISTS id;
ALTER TABLE router_metrics RENAME COLUMN id_new TO id;
ALTER TABLE traffic_samples DROP COLUMN IF EXISTS id;
ALTER TABLE traffic_samples RENAME COLUMN id_new TO id;
ALTER TABLE service_sessions DROP COLUMN IF EXISTS id;
ALTER TABLE service_sessions RENAME COLUMN id_new TO id;
ALTER TABLE router_command_logs DROP COLUMN IF EXISTS id;
ALTER TABLE router_command_logs RENAME COLUMN id_new TO id;
ALTER TABLE router_backups DROP COLUMN IF EXISTS id;
ALTER TABLE router_backups RENAME COLUMN id_new TO id;
ALTER TABLE audit_logs DROP COLUMN IF EXISTS id;
ALTER TABLE audit_logs RENAME COLUMN id_new TO id;
ALTER TABLE api_keys DROP COLUMN IF EXISTS id;
ALTER TABLE api_keys RENAME COLUMN id_new TO id;
ALTER TABLE settings DROP COLUMN IF EXISTS id;
ALTER TABLE settings RENAME COLUMN id_new TO id;
ALTER TABLE refresh_tokens DROP COLUMN IF EXISTS id;
ALTER TABLE refresh_tokens RENAME COLUMN id_new TO id;
ALTER TABLE leads DROP COLUMN IF EXISTS id;
ALTER TABLE leads RENAME COLUMN id_new TO id;
ALTER TABLE resellers DROP COLUMN IF EXISTS id;
ALTER TABLE resellers RENAME COLUMN id_new TO id;
ALTER TABLE job_runs DROP COLUMN IF EXISTS id;
ALTER TABLE job_runs RENAME COLUMN id_new TO id;
ALTER TABLE outbound_webhooks DROP COLUMN IF EXISTS id;
ALTER TABLE outbound_webhooks RENAME COLUMN id_new TO id;
ALTER TABLE outbound_webhook_deliveries DROP COLUMN IF EXISTS id;
ALTER TABLE outbound_webhook_deliveries RENAME COLUMN id_new TO id;
ALTER TABLE alerts DROP COLUMN IF EXISTS id;
ALTER TABLE alerts RENAME COLUMN id_new TO id;

-- 7) Restore PRIMARY KEY
ALTER TABLE tenants ADD PRIMARY KEY (id);
ALTER TABLE roles ADD PRIMARY KEY (id);
ALTER TABLE users ADD PRIMARY KEY (id);
ALTER TABLE user_tenants ADD PRIMARY KEY (id);
ALTER TABLE customers ADD PRIMARY KEY (id);
ALTER TABLE customer_addresses ADD PRIMARY KEY (id);
ALTER TABLE sites ADD PRIMARY KEY (id);
ALTER TABLE routers ADD PRIMARY KEY (id);
ALTER TABLE ip_pools ADD PRIMARY KEY (id);
ALTER TABLE ip_assignments ADD PRIMARY KEY (id);
ALTER TABLE plans ADD PRIMARY KEY (id);
ALTER TABLE subscriptions ADD PRIMARY KEY (id);
ALTER TABLE invoices ADD PRIMARY KEY (id);
ALTER TABLE invoice_items ADD PRIMARY KEY (id);
ALTER TABLE payments ADD PRIMARY KEY (id);
ALTER TABLE payment_intents ADD PRIMARY KEY (id);
ALTER TABLE wallets ADD PRIMARY KEY (id);
ALTER TABLE wallet_transactions ADD PRIMARY KEY (id);
ALTER TABLE voucher_batches ADD PRIMARY KEY (id);
ALTER TABLE vouchers ADD PRIMARY KEY (id);
ALTER TABLE tickets ADD PRIMARY KEY (id);
ALTER TABLE ticket_messages ADD PRIMARY KEY (id);
ALTER TABLE technicians ADD PRIMARY KEY (id);
ALTER TABLE work_orders ADD PRIMARY KEY (id);
ALTER TABLE odcs ADD PRIMARY KEY (id);
ALTER TABLE odps ADD PRIMARY KEY (id);
ALTER TABLE odp_ports ADD PRIMARY KEY (id);
ALTER TABLE chart_of_accounts ADD PRIMARY KEY (id);
ALTER TABLE journal_entries ADD PRIMARY KEY (id);
ALTER TABLE journal_lines ADD PRIMARY KEY (id);
ALTER TABLE expenses ADD PRIMARY KEY (id);
ALTER TABLE notification_templates ADD PRIMARY KEY (id);
ALTER TABLE notification_queue ADD PRIMARY KEY (id);
ALTER TABLE router_metrics ADD PRIMARY KEY (id);
ALTER TABLE traffic_samples ADD PRIMARY KEY (id);
ALTER TABLE service_sessions ADD PRIMARY KEY (id);
ALTER TABLE router_command_logs ADD PRIMARY KEY (id);
ALTER TABLE router_backups ADD PRIMARY KEY (id);
ALTER TABLE audit_logs ADD PRIMARY KEY (id);
ALTER TABLE api_keys ADD PRIMARY KEY (id);
ALTER TABLE settings ADD PRIMARY KEY (id);
ALTER TABLE refresh_tokens ADD PRIMARY KEY (id);
ALTER TABLE leads ADD PRIMARY KEY (id);
ALTER TABLE resellers ADD PRIMARY KEY (id);
ALTER TABLE job_runs ADD PRIMARY KEY (id);
ALTER TABLE outbound_webhooks ADD PRIMARY KEY (id);
ALTER TABLE outbound_webhook_deliveries ADD PRIMARY KEY (id);
ALTER TABLE alerts ADD PRIMARY KEY (id);

-- 8) Set DEFAULT gen_random_uuid() on id columns
ALTER TABLE tenants ALTER COLUMN id SET DEFAULT gen_random_uuid();
ALTER TABLE roles ALTER COLUMN id SET DEFAULT gen_random_uuid();
ALTER TABLE users ALTER COLUMN id SET DEFAULT gen_random_uuid();
ALTER TABLE user_tenants ALTER COLUMN id SET DEFAULT gen_random_uuid();
ALTER TABLE customers ALTER COLUMN id SET DEFAULT gen_random_uuid();
ALTER TABLE customer_addresses ALTER COLUMN id SET DEFAULT gen_random_uuid();
ALTER TABLE sites ALTER COLUMN id SET DEFAULT gen_random_uuid();
ALTER TABLE routers ALTER COLUMN id SET DEFAULT gen_random_uuid();
ALTER TABLE ip_pools ALTER COLUMN id SET DEFAULT gen_random_uuid();
ALTER TABLE ip_assignments ALTER COLUMN id SET DEFAULT gen_random_uuid();
ALTER TABLE plans ALTER COLUMN id SET DEFAULT gen_random_uuid();
ALTER TABLE subscriptions ALTER COLUMN id SET DEFAULT gen_random_uuid();
ALTER TABLE invoices ALTER COLUMN id SET DEFAULT gen_random_uuid();
ALTER TABLE invoice_items ALTER COLUMN id SET DEFAULT gen_random_uuid();
ALTER TABLE payments ALTER COLUMN id SET DEFAULT gen_random_uuid();
ALTER TABLE payment_intents ALTER COLUMN id SET DEFAULT gen_random_uuid();
ALTER TABLE wallets ALTER COLUMN id SET DEFAULT gen_random_uuid();
ALTER TABLE wallet_transactions ALTER COLUMN id SET DEFAULT gen_random_uuid();
ALTER TABLE voucher_batches ALTER COLUMN id SET DEFAULT gen_random_uuid();
ALTER TABLE vouchers ALTER COLUMN id SET DEFAULT gen_random_uuid();
ALTER TABLE tickets ALTER COLUMN id SET DEFAULT gen_random_uuid();
ALTER TABLE ticket_messages ALTER COLUMN id SET DEFAULT gen_random_uuid();
ALTER TABLE technicians ALTER COLUMN id SET DEFAULT gen_random_uuid();
ALTER TABLE work_orders ALTER COLUMN id SET DEFAULT gen_random_uuid();
ALTER TABLE odcs ALTER COLUMN id SET DEFAULT gen_random_uuid();
ALTER TABLE odps ALTER COLUMN id SET DEFAULT gen_random_uuid();
ALTER TABLE odp_ports ALTER COLUMN id SET DEFAULT gen_random_uuid();
ALTER TABLE chart_of_accounts ALTER COLUMN id SET DEFAULT gen_random_uuid();
ALTER TABLE journal_entries ALTER COLUMN id SET DEFAULT gen_random_uuid();
ALTER TABLE journal_lines ALTER COLUMN id SET DEFAULT gen_random_uuid();
ALTER TABLE expenses ALTER COLUMN id SET DEFAULT gen_random_uuid();
ALTER TABLE notification_templates ALTER COLUMN id SET DEFAULT gen_random_uuid();
ALTER TABLE notification_queue ALTER COLUMN id SET DEFAULT gen_random_uuid();
ALTER TABLE router_metrics ALTER COLUMN id SET DEFAULT gen_random_uuid();
ALTER TABLE traffic_samples ALTER COLUMN id SET DEFAULT gen_random_uuid();
ALTER TABLE service_sessions ALTER COLUMN id SET DEFAULT gen_random_uuid();
ALTER TABLE router_command_logs ALTER COLUMN id SET DEFAULT gen_random_uuid();
ALTER TABLE router_backups ALTER COLUMN id SET DEFAULT gen_random_uuid();
ALTER TABLE audit_logs ALTER COLUMN id SET DEFAULT gen_random_uuid();
ALTER TABLE api_keys ALTER COLUMN id SET DEFAULT gen_random_uuid();
ALTER TABLE settings ALTER COLUMN id SET DEFAULT gen_random_uuid();
ALTER TABLE refresh_tokens ALTER COLUMN id SET DEFAULT gen_random_uuid();
ALTER TABLE leads ALTER COLUMN id SET DEFAULT gen_random_uuid();
ALTER TABLE resellers ALTER COLUMN id SET DEFAULT gen_random_uuid();
ALTER TABLE job_runs ALTER COLUMN id SET DEFAULT gen_random_uuid();
ALTER TABLE outbound_webhooks ALTER COLUMN id SET DEFAULT gen_random_uuid();
ALTER TABLE outbound_webhook_deliveries ALTER COLUMN id SET DEFAULT gen_random_uuid();
ALTER TABLE alerts ALTER COLUMN id SET DEFAULT gen_random_uuid();

-- 9) NOT NULL on required FK columns (tenant_id etc that were NOT NULL)
ALTER TABLE user_tenants ALTER COLUMN user_id SET NOT NULL;
ALTER TABLE user_tenants ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE user_tenants ALTER COLUMN role_id SET NOT NULL;
ALTER TABLE customers ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE customer_addresses ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE customer_addresses ALTER COLUMN customer_id SET NOT NULL;
ALTER TABLE sites ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE routers ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE ip_pools ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE ip_assignments ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE ip_assignments ALTER COLUMN pool_id SET NOT NULL;
ALTER TABLE plans ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE subscriptions ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE subscriptions ALTER COLUMN customer_id SET NOT NULL;
ALTER TABLE subscriptions ALTER COLUMN plan_id SET NOT NULL;
ALTER TABLE invoices ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE invoices ALTER COLUMN customer_id SET NOT NULL;
ALTER TABLE invoice_items ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE invoice_items ALTER COLUMN invoice_id SET NOT NULL;
ALTER TABLE payments ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE payments ALTER COLUMN customer_id SET NOT NULL;
ALTER TABLE payment_intents ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE payment_intents ALTER COLUMN customer_id SET NOT NULL;
ALTER TABLE wallets ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE wallets ALTER COLUMN customer_id SET NOT NULL;
ALTER TABLE wallet_transactions ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE wallet_transactions ALTER COLUMN wallet_id SET NOT NULL;
ALTER TABLE voucher_batches ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE vouchers ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE vouchers ALTER COLUMN batch_id SET NOT NULL;
ALTER TABLE tickets ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE ticket_messages ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE ticket_messages ALTER COLUMN ticket_id SET NOT NULL;
ALTER TABLE technicians ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE work_orders ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE odcs ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE odps ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE odp_ports ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE odp_ports ALTER COLUMN odp_id SET NOT NULL;
ALTER TABLE chart_of_accounts ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE journal_entries ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE journal_lines ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE journal_lines ALTER COLUMN entry_id SET NOT NULL;
ALTER TABLE journal_lines ALTER COLUMN account_id SET NOT NULL;
ALTER TABLE expenses ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE notification_templates ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE notification_queue ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE router_metrics ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE router_metrics ALTER COLUMN router_id SET NOT NULL;
ALTER TABLE traffic_samples ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE service_sessions ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE router_command_logs ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE router_command_logs ALTER COLUMN router_id SET NOT NULL;
ALTER TABLE router_backups ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE router_backups ALTER COLUMN router_id SET NOT NULL;
ALTER TABLE api_keys ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE settings ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE refresh_tokens ALTER COLUMN user_id SET NOT NULL;
ALTER TABLE leads ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE resellers ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE job_runs ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE outbound_webhooks ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE outbound_webhook_deliveries ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE outbound_webhook_deliveries ALTER COLUMN webhook_id SET NOT NULL;
ALTER TABLE alerts ALTER COLUMN tenant_id SET NOT NULL;

-- 10) Recreate foreign keys
ALTER TABLE roles ADD CONSTRAINT roles_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE;
ALTER TABLE user_tenants ADD CONSTRAINT user_tenants_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
ALTER TABLE user_tenants ADD CONSTRAINT user_tenants_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE;
ALTER TABLE user_tenants ADD CONSTRAINT user_tenants_role_id_fkey FOREIGN KEY (role_id) REFERENCES roles(id);
ALTER TABLE customers ADD CONSTRAINT customers_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE;
ALTER TABLE customer_addresses ADD CONSTRAINT customer_addresses_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE;
ALTER TABLE customer_addresses ADD CONSTRAINT customer_addresses_customer_id_fkey FOREIGN KEY (customer_id) REFERENCES customers(id) ON DELETE CASCADE;
ALTER TABLE sites ADD CONSTRAINT sites_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE;
ALTER TABLE routers ADD CONSTRAINT routers_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE;
ALTER TABLE routers ADD CONSTRAINT routers_site_id_fkey FOREIGN KEY (site_id) REFERENCES sites(id) ON DELETE SET NULL;
ALTER TABLE ip_pools ADD CONSTRAINT ip_pools_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE;
ALTER TABLE ip_pools ADD CONSTRAINT ip_pools_router_id_fkey FOREIGN KEY (router_id) REFERENCES routers(id) ON DELETE SET NULL;
ALTER TABLE ip_assignments ADD CONSTRAINT ip_assignments_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE;
ALTER TABLE ip_assignments ADD CONSTRAINT ip_assignments_pool_id_fkey FOREIGN KEY (pool_id) REFERENCES ip_pools(id) ON DELETE CASCADE;
ALTER TABLE ip_assignments ADD CONSTRAINT ip_assignments_customer_id_fkey FOREIGN KEY (customer_id) REFERENCES customers(id) ON DELETE SET NULL;
ALTER TABLE plans ADD CONSTRAINT plans_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE;
ALTER TABLE subscriptions ADD CONSTRAINT subscriptions_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE;
ALTER TABLE subscriptions ADD CONSTRAINT subscriptions_customer_id_fkey FOREIGN KEY (customer_id) REFERENCES customers(id) ON DELETE CASCADE;
ALTER TABLE subscriptions ADD CONSTRAINT subscriptions_plan_id_fkey FOREIGN KEY (plan_id) REFERENCES plans(id);
ALTER TABLE subscriptions ADD CONSTRAINT subscriptions_router_id_fkey FOREIGN KEY (router_id) REFERENCES routers(id) ON DELETE SET NULL;
ALTER TABLE invoices ADD CONSTRAINT invoices_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE;
ALTER TABLE invoices ADD CONSTRAINT invoices_customer_id_fkey FOREIGN KEY (customer_id) REFERENCES customers(id) ON DELETE CASCADE;
ALTER TABLE invoices ADD CONSTRAINT invoices_subscription_id_fkey FOREIGN KEY (subscription_id) REFERENCES subscriptions(id) ON DELETE SET NULL;
ALTER TABLE invoice_items ADD CONSTRAINT invoice_items_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE;
ALTER TABLE invoice_items ADD CONSTRAINT invoice_items_invoice_id_fkey FOREIGN KEY (invoice_id) REFERENCES invoices(id) ON DELETE CASCADE;
ALTER TABLE payments ADD CONSTRAINT payments_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE;
ALTER TABLE payments ADD CONSTRAINT payments_customer_id_fkey FOREIGN KEY (customer_id) REFERENCES customers(id) ON DELETE CASCADE;
ALTER TABLE payments ADD CONSTRAINT payments_invoice_id_fkey FOREIGN KEY (invoice_id) REFERENCES invoices(id) ON DELETE SET NULL;
ALTER TABLE payment_intents ADD CONSTRAINT payment_intents_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE;
ALTER TABLE payment_intents ADD CONSTRAINT payment_intents_customer_id_fkey FOREIGN KEY (customer_id) REFERENCES customers(id) ON DELETE CASCADE;
ALTER TABLE payment_intents ADD CONSTRAINT payment_intents_invoice_id_fkey FOREIGN KEY (invoice_id) REFERENCES invoices(id) ON DELETE SET NULL;
ALTER TABLE wallets ADD CONSTRAINT wallets_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE;
ALTER TABLE wallets ADD CONSTRAINT wallets_customer_id_fkey FOREIGN KEY (customer_id) REFERENCES customers(id) ON DELETE CASCADE;
ALTER TABLE wallet_transactions ADD CONSTRAINT wallet_transactions_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE;
ALTER TABLE wallet_transactions ADD CONSTRAINT wallet_transactions_wallet_id_fkey FOREIGN KEY (wallet_id) REFERENCES wallets(id) ON DELETE CASCADE;
ALTER TABLE voucher_batches ADD CONSTRAINT voucher_batches_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE;
ALTER TABLE voucher_batches ADD CONSTRAINT voucher_batches_plan_id_fkey FOREIGN KEY (plan_id) REFERENCES plans(id) ON DELETE SET NULL;
ALTER TABLE vouchers ADD CONSTRAINT vouchers_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE;
ALTER TABLE vouchers ADD CONSTRAINT vouchers_batch_id_fkey FOREIGN KEY (batch_id) REFERENCES voucher_batches(id) ON DELETE CASCADE;
ALTER TABLE vouchers ADD CONSTRAINT vouchers_used_by_fkey FOREIGN KEY (used_by) REFERENCES customers(id) ON DELETE SET NULL;
ALTER TABLE tickets ADD CONSTRAINT tickets_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE;
ALTER TABLE tickets ADD CONSTRAINT tickets_customer_id_fkey FOREIGN KEY (customer_id) REFERENCES customers(id) ON DELETE SET NULL;
ALTER TABLE tickets ADD CONSTRAINT tickets_assigned_to_fkey FOREIGN KEY (assigned_to) REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE ticket_messages ADD CONSTRAINT ticket_messages_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE;
ALTER TABLE ticket_messages ADD CONSTRAINT ticket_messages_ticket_id_fkey FOREIGN KEY (ticket_id) REFERENCES tickets(id) ON DELETE CASCADE;
ALTER TABLE technicians ADD CONSTRAINT technicians_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE;
ALTER TABLE technicians ADD CONSTRAINT technicians_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE work_orders ADD CONSTRAINT work_orders_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE;
ALTER TABLE work_orders ADD CONSTRAINT work_orders_customer_id_fkey FOREIGN KEY (customer_id) REFERENCES customers(id) ON DELETE SET NULL;
ALTER TABLE work_orders ADD CONSTRAINT work_orders_subscription_id_fkey FOREIGN KEY (subscription_id) REFERENCES subscriptions(id) ON DELETE SET NULL;
ALTER TABLE work_orders ADD CONSTRAINT work_orders_technician_id_fkey FOREIGN KEY (technician_id) REFERENCES technicians(id) ON DELETE SET NULL;
ALTER TABLE odcs ADD CONSTRAINT odcs_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE;
ALTER TABLE odcs ADD CONSTRAINT odcs_site_id_fkey FOREIGN KEY (site_id) REFERENCES sites(id) ON DELETE SET NULL;
ALTER TABLE odps ADD CONSTRAINT odps_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE;
ALTER TABLE odps ADD CONSTRAINT odps_odc_id_fkey FOREIGN KEY (odc_id) REFERENCES odcs(id) ON DELETE SET NULL;
ALTER TABLE odp_ports ADD CONSTRAINT odp_ports_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE;
ALTER TABLE odp_ports ADD CONSTRAINT odp_ports_odp_id_fkey FOREIGN KEY (odp_id) REFERENCES odps(id) ON DELETE CASCADE;
ALTER TABLE odp_ports ADD CONSTRAINT odp_ports_customer_id_fkey FOREIGN KEY (customer_id) REFERENCES customers(id) ON DELETE SET NULL;
ALTER TABLE odp_ports ADD CONSTRAINT odp_ports_subscription_id_fkey FOREIGN KEY (subscription_id) REFERENCES subscriptions(id) ON DELETE SET NULL;
ALTER TABLE chart_of_accounts ADD CONSTRAINT chart_of_accounts_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE;
ALTER TABLE journal_entries ADD CONSTRAINT journal_entries_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE;
ALTER TABLE journal_lines ADD CONSTRAINT journal_lines_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE;
ALTER TABLE journal_lines ADD CONSTRAINT journal_lines_entry_id_fkey FOREIGN KEY (entry_id) REFERENCES journal_entries(id) ON DELETE CASCADE;
ALTER TABLE journal_lines ADD CONSTRAINT journal_lines_account_id_fkey FOREIGN KEY (account_id) REFERENCES chart_of_accounts(id);
ALTER TABLE expenses ADD CONSTRAINT expenses_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE;
ALTER TABLE expenses ADD CONSTRAINT expenses_account_id_fkey FOREIGN KEY (account_id) REFERENCES chart_of_accounts(id) ON DELETE SET NULL;
ALTER TABLE notification_templates ADD CONSTRAINT notification_templates_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE;
ALTER TABLE notification_queue ADD CONSTRAINT notification_queue_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE;
ALTER TABLE router_metrics ADD CONSTRAINT router_metrics_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE;
ALTER TABLE router_metrics ADD CONSTRAINT router_metrics_router_id_fkey FOREIGN KEY (router_id) REFERENCES routers(id) ON DELETE CASCADE;
ALTER TABLE traffic_samples ADD CONSTRAINT traffic_samples_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE;
ALTER TABLE traffic_samples ADD CONSTRAINT traffic_samples_router_id_fkey FOREIGN KEY (router_id) REFERENCES routers(id) ON DELETE SET NULL;
ALTER TABLE traffic_samples ADD CONSTRAINT traffic_samples_subscription_id_fkey FOREIGN KEY (subscription_id) REFERENCES subscriptions(id) ON DELETE SET NULL;
ALTER TABLE service_sessions ADD CONSTRAINT service_sessions_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE;
ALTER TABLE service_sessions ADD CONSTRAINT service_sessions_subscription_id_fkey FOREIGN KEY (subscription_id) REFERENCES subscriptions(id) ON DELETE SET NULL;
ALTER TABLE service_sessions ADD CONSTRAINT service_sessions_router_id_fkey FOREIGN KEY (router_id) REFERENCES routers(id) ON DELETE SET NULL;
ALTER TABLE router_command_logs ADD CONSTRAINT router_command_logs_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE;
ALTER TABLE router_command_logs ADD CONSTRAINT router_command_logs_router_id_fkey FOREIGN KEY (router_id) REFERENCES routers(id) ON DELETE CASCADE;
ALTER TABLE router_command_logs ADD CONSTRAINT router_command_logs_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE router_backups ADD CONSTRAINT router_backups_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE;
ALTER TABLE router_backups ADD CONSTRAINT router_backups_router_id_fkey FOREIGN KEY (router_id) REFERENCES routers(id) ON DELETE CASCADE;
ALTER TABLE audit_logs ADD CONSTRAINT audit_logs_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE SET NULL;
ALTER TABLE audit_logs ADD CONSTRAINT audit_logs_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE api_keys ADD CONSTRAINT api_keys_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE;
ALTER TABLE settings ADD CONSTRAINT settings_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE;
ALTER TABLE refresh_tokens ADD CONSTRAINT refresh_tokens_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
ALTER TABLE leads ADD CONSTRAINT leads_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE;
ALTER TABLE leads ADD CONSTRAINT leads_odp_id_fkey FOREIGN KEY (odp_id) REFERENCES odps(id) ON DELETE SET NULL;
ALTER TABLE resellers ADD CONSTRAINT resellers_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE;
ALTER TABLE resellers ADD CONSTRAINT resellers_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE job_runs ADD CONSTRAINT job_runs_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE;
ALTER TABLE outbound_webhooks ADD CONSTRAINT outbound_webhooks_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE;
ALTER TABLE outbound_webhook_deliveries ADD CONSTRAINT outbound_webhook_deliveries_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE;
ALTER TABLE outbound_webhook_deliveries ADD CONSTRAINT outbound_webhook_deliveries_webhook_id_fkey FOREIGN KEY (webhook_id) REFERENCES outbound_webhooks(id) ON DELETE CASCADE;
ALTER TABLE alerts ADD CONSTRAINT alerts_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE;

-- 11) Recreate RLS policies with uuid cast
CREATE POLICY tenant_isolation_customers ON customers
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
CREATE POLICY tenant_isolation_plans ON plans
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
CREATE POLICY tenant_isolation_subscriptions ON subscriptions
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
CREATE POLICY tenant_isolation_routers ON routers
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
CREATE POLICY tenant_isolation_invoices ON invoices
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
CREATE POLICY tenant_isolation_payments ON payments
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
CREATE POLICY tenant_isolation_tickets ON tickets
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);


-- 12) Recreate UNIQUE constraints dropped with bigint columns
ALTER TABLE roles ADD CONSTRAINT roles_tenant_id_slug_key UNIQUE (tenant_id, slug);
ALTER TABLE user_tenants ADD CONSTRAINT user_tenants_user_id_tenant_id_key UNIQUE (user_id, tenant_id);
ALTER TABLE customers ADD CONSTRAINT customers_tenant_id_customer_code_key UNIQUE (tenant_id, customer_code);
ALTER TABLE sites ADD CONSTRAINT sites_tenant_id_code_key UNIQUE (tenant_id, code);
ALTER TABLE ip_pools ADD CONSTRAINT ip_pools_tenant_id_name_key UNIQUE (tenant_id, name);
ALTER TABLE ip_assignments ADD CONSTRAINT ip_assignments_pool_id_ip_address_key UNIQUE (pool_id, ip_address);
ALTER TABLE plans ADD CONSTRAINT plans_tenant_id_code_key UNIQUE (tenant_id, code);
ALTER TABLE subscriptions ADD CONSTRAINT subscriptions_tenant_id_username_key UNIQUE (tenant_id, username);
ALTER TABLE invoices ADD CONSTRAINT invoices_tenant_id_invoice_number_key UNIQUE (tenant_id, invoice_number);
ALTER TABLE wallets ADD CONSTRAINT wallets_tenant_id_customer_id_key UNIQUE (tenant_id, customer_id);
ALTER TABLE vouchers ADD CONSTRAINT vouchers_tenant_id_code_key UNIQUE (tenant_id, code);
ALTER TABLE odcs ADD CONSTRAINT odcs_tenant_id_code_key UNIQUE (tenant_id, code);
ALTER TABLE odps ADD CONSTRAINT odps_tenant_id_code_key UNIQUE (tenant_id, code);
ALTER TABLE odp_ports ADD CONSTRAINT odp_ports_odp_id_port_number_key UNIQUE (odp_id, port_number);
ALTER TABLE chart_of_accounts ADD CONSTRAINT chart_of_accounts_tenant_id_code_key UNIQUE (tenant_id, code);
ALTER TABLE notification_templates ADD CONSTRAINT notification_templates_tenant_id_channel_event_key UNIQUE (tenant_id, channel, event);
ALTER TABLE settings ADD CONSTRAINT settings_tenant_id_key_key UNIQUE (tenant_id, key);
ALTER TABLE job_runs ADD CONSTRAINT job_runs_tenant_id_job_name_job_key_key UNIQUE (tenant_id, job_name, job_key);

-- 13) Recreate indexes that referenced converted columns
CREATE INDEX IF NOT EXISTS idx_customers_tenant ON customers(tenant_id);
CREATE INDEX IF NOT EXISTS idx_subscriptions_tenant ON subscriptions(tenant_id);
CREATE INDEX IF NOT EXISTS idx_subscriptions_status ON subscriptions(tenant_id, status);
CREATE INDEX IF NOT EXISTS idx_invoices_tenant_status ON invoices(tenant_id, status);
CREATE INDEX IF NOT EXISTS idx_invoices_due ON invoices(tenant_id, due_date);
CREATE INDEX IF NOT EXISTS idx_payments_tenant ON payments(tenant_id);
CREATE INDEX IF NOT EXISTS idx_router_metrics_router_time ON router_metrics(router_id, recorded_at DESC);
CREATE INDEX IF NOT EXISTS idx_traffic_samples_sub_time ON traffic_samples(subscription_id, recorded_at DESC);
CREATE INDEX IF NOT EXISTS idx_notification_queue_pending ON notification_queue(status, scheduled_at) WHERE status = 'pending';
CREATE INDEX IF NOT EXISTS idx_alerts_tenant_created ON alerts(tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_job_runs_lookup ON job_runs(tenant_id, job_name, job_key);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
SELECT 1 / 0; -- irreversible: UUID primary keys cannot be converted back to BIGINT
-- +goose StatementEnd
