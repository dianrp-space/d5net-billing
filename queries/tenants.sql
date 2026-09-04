-- name: GetTenantByID :one
SELECT id, slug, name, is_active FROM tenants WHERE id = $1;

-- name: CountActiveCustomers :one
SELECT COUNT(*) FROM customers WHERE tenant_id = $1 AND is_active = true;
