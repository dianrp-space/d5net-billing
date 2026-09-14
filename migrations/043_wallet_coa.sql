-- +goose Up
-- Akun liabilitas "Saldo pelanggan" untuk fitur saldo/topup: topup menambah
-- liabilitas (kredit), auto-pay tagihan mengurangi liabilitas (debit).
INSERT INTO chart_of_accounts (tenant_id, code, name, type)
SELECT t.id, '2100', 'Saldo pelanggan', 'liability'
FROM tenants t
WHERE NOT EXISTS (
    SELECT 1 FROM chart_of_accounts a
    WHERE a.tenant_id = t.id AND a.code = '2100'
);

-- +goose Down
DELETE FROM chart_of_accounts WHERE code = '2100' AND name = 'Saldo pelanggan';
