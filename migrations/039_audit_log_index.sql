-- +goose Up
-- Index audit log untuk halaman Audit Log (urut terbaru per tenant).
CREATE INDEX IF NOT EXISTS idx_audit_logs_tenant_created
    ON audit_logs(tenant_id, created_at DESC);

-- +goose Down
DROP INDEX IF EXISTS idx_audit_logs_tenant_created;
