-- +goose Up
-- Jurnal-kan beban lama yang belum memiliki entri jurnal: debit akun beban,
-- kredit akun kas. Tanpa ini, buku besar/neraca saldo hanya memuat pembayaran
-- dan tidak seimbang dengan laporan laba-rugi.
WITH exp_acc AS (
    SELECT DISTINCT ON (tenant_id) tenant_id, id AS account_id
    FROM chart_of_accounts
    WHERE is_active AND LOWER(type) = 'expense'
    ORDER BY tenant_id, code
), cash_acc AS (
    SELECT DISTINCT ON (tenant_id) tenant_id, id AS account_id
    FROM chart_of_accounts
    WHERE is_active AND LOWER(type) = 'asset'
    ORDER BY tenant_id, code
), missing AS (
    SELECT e.id, e.tenant_id, e.amount, e.category, e.expense_date, e.description,
           ea.account_id AS expense_account, ca.account_id AS cash_account
    FROM expenses e
    JOIN exp_acc ea ON ea.tenant_id = e.tenant_id
    JOIN cash_acc ca ON ca.tenant_id = e.tenant_id
    WHERE NOT EXISTS (
        SELECT 1 FROM journal_entries je
        WHERE je.tenant_id = e.tenant_id
          AND je.source_type = 'expense'
          AND je.source_id = e.id
    )
), ins AS (
    INSERT INTO journal_entries (tenant_id, entry_date, reference, description, source_type, source_id)
    SELECT tenant_id, expense_date, description, 'Beban ' || category, 'expense', id
    FROM missing
    RETURNING id, tenant_id, source_id
)
INSERT INTO journal_lines (tenant_id, entry_id, account_id, debit, credit)
SELECT m.tenant_id, ins.id, m.expense_account, m.amount, 0
FROM missing m JOIN ins ON ins.source_id = m.id AND ins.tenant_id = m.tenant_id
UNION ALL
SELECT m.tenant_id, ins.id, m.cash_account, 0, m.amount
FROM missing m JOIN ins ON ins.source_id = m.id AND ins.tenant_id = m.tenant_id;

UPDATE expenses e
SET account_id = ea.account_id
FROM (
    SELECT DISTINCT ON (tenant_id) tenant_id, id AS account_id
    FROM chart_of_accounts
    WHERE is_active AND LOWER(type) = 'expense'
    ORDER BY tenant_id, code
) ea
WHERE e.tenant_id = ea.tenant_id AND e.account_id IS NULL;

-- +goose Down
DELETE FROM journal_entries WHERE source_type = 'expense';
