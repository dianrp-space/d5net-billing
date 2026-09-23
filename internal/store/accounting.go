package store

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/dianrp-space/d5net-billing/internal/xid"
	"github.com/jackc/pgx/v5"
)

type Account struct {
	ID   xid.ID `json:"id"`
	Code string `json:"code"`
	Name string `json:"name"`
	Type string `json:"type"`
}

type Expense struct {
	ID          xid.ID    `json:"id"`
	Amount      int64     `json:"amount"`
	Category    string    `json:"category"`
	Description *string   `json:"description,omitempty"`
	ExpenseDate string    `json:"expense_date"`
	CreatedAt   time.Time `json:"created_at"`
}

func (s *Store) ListAccounts(ctx context.Context, tenantID xid.ID) ([]Account, error) {
	rows, err := s.Pool.Query(ctx, `SELECT id, code, name, type FROM chart_of_accounts WHERE tenant_id=$1 ORDER BY code`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []Account
	for rows.Next() {
		var a Account
		if err := rows.Scan(&a.ID, &a.Code, &a.Name, &a.Type); err != nil {
			return nil, err
		}
		list = append(list, a)
	}
	return list, rows.Err()
}

func (s *Store) ListExpenses(ctx context.Context, tenantID xid.ID, limit, offset int) ([]Expense, int64, error) {
	if limit <= 0 {
		limit = 50
	}
	var total int64
	if err := s.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM expenses WHERE tenant_id=$1`, tenantID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT id, amount, category, description, expense_date::text, created_at
		FROM expenses WHERE tenant_id=$1
		ORDER BY expense_date DESC, created_at DESC
		LIMIT $2 OFFSET $3
	`, tenantID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var list []Expense
	for rows.Next() {
		var e Expense
		if err := rows.Scan(&e.ID, &e.Amount, &e.Category, &e.Description, &e.ExpenseDate, &e.CreatedAt); err != nil {
			return nil, 0, err
		}
		list = append(list, e)
	}
	return list, total, rows.Err()
}

func (s *Store) CreateExpense(ctx context.Context, tenantID xid.ID, amount int64, category, desc, date string) error {
	if amount <= 0 {
		return fmt.Errorf("nominal beban harus > 0")
	}
	category = strings.TrimSpace(category)
	if category == "" {
		category = "ops"
	}
	if date == "" {
		date = s.TenantNow(ctx, tenantID).Format("2006-01-02")
	}
	// Simpan beban sekaligus jurnalnya (debit beban, kredit kas) dalam satu
	// transaksi agar buku besar dan neraca saldo tetap seimbang.
	return s.withTenant(ctx, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		var expenseAcc, cashAcc xid.ID
		_ = tx.QueryRow(ctx, `
			SELECT id FROM chart_of_accounts
			WHERE tenant_id=$1 AND is_active=true AND LOWER(type)='expense'
			ORDER BY code LIMIT 1`, tenantID).Scan(&expenseAcc)
		_ = tx.QueryRow(ctx, `
			SELECT id FROM chart_of_accounts
			WHERE tenant_id=$1 AND is_active=true AND LOWER(type)='asset'
			ORDER BY code LIMIT 1`, tenantID).Scan(&cashAcc)

		var accountArg any
		if !xid.IsNil(expenseAcc) {
			accountArg = expenseAcc
		}
		var expenseID xid.ID
		if err := tx.QueryRow(ctx, `
			INSERT INTO expenses (tenant_id, account_id, amount, category, description, expense_date)
			VALUES ($1,$2,$3,$4,$5,$6) RETURNING id
		`, tenantID, accountArg, amount, category, desc, date).Scan(&expenseID); err != nil {
			return err
		}
		if xid.IsNil(expenseAcc) || xid.IsNil(cashAcc) {
			return nil
		}
		var entryID xid.ID
		if err := tx.QueryRow(ctx, `
			INSERT INTO journal_entries (tenant_id, entry_date, description, source_type, source_id)
			VALUES ($1,$2,$3,'expense',$4) RETURNING id
		`, tenantID, date, "Beban "+category, expenseID).Scan(&entryID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO journal_lines (tenant_id, entry_id, account_id, debit, credit)
			VALUES ($1,$2,$3,$4,0), ($1,$2,$5,0,$4)
		`, tenantID, entryID, expenseAcc, amount, cashAcc)
		return err
	})
}

func (s *Store) AgingReceivable(ctx context.Context, tenantID xid.ID) (map[string]int64, error) {
	out := map[string]int64{"current": 0, "d30": 0, "d60": 0, "d90": 0}
	rows, err := s.Pool.Query(ctx, `
		SELECT
			CASE
				WHEN due_date >= CURRENT_DATE THEN 'current'
				WHEN due_date >= CURRENT_DATE - 30 THEN 'd30'
				WHEN due_date >= CURRENT_DATE - 60 THEN 'd60'
				ELSE 'd90'
			END AS bucket,
			COALESCE(SUM(total_amount - paid_amount),0)
		FROM invoices
		WHERE tenant_id=$1 AND deleted_at IS NULL AND status IN ('issued','partial','overdue')
		GROUP BY 1
	`, tenantID)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var b string
		var v int64
		if err := rows.Scan(&b, &v); err != nil {
			return out, err
		}
		out[b] = v
	}
	return out, rows.Err()
}

func (s *Store) CashFlow(ctx context.Context, tenantID xid.ID, months int) ([]map[string]any, error) {
	if months <= 0 {
		months = 6
	}
	type agg struct{ in, out int64 }
	byMonth := map[string]*agg{}
	inRows, err := s.Pool.Query(ctx, `
		SELECT to_char(date_trunc('month', paid_at), 'YYYY-MM'), COALESCE(SUM(amount),0)
		FROM payments WHERE tenant_id=$1 AND status='paid' AND paid_at IS NOT NULL AND deleted_at IS NULL
		  AND sandbox = false
		  AND paid_at >= NOW() - ($2 * INTERVAL '1 month')
		GROUP BY 1
	`, tenantID, months)
	if err != nil {
		return nil, err
	}
	for inRows.Next() {
		var m string
		var v int64
		if err := inRows.Scan(&m, &v); err != nil {
			inRows.Close()
			return nil, err
		}
		if byMonth[m] == nil {
			byMonth[m] = &agg{}
		}
		byMonth[m].in = v
	}
	inRows.Close()
	outRows, err := s.Pool.Query(ctx, `
		SELECT to_char(date_trunc('month', expense_date::timestamptz), 'YYYY-MM'), COALESCE(SUM(amount),0)
		FROM expenses WHERE tenant_id=$1
		  AND expense_date >= (NOW() - ($2 * INTERVAL '1 month'))::date
		GROUP BY 1
	`, tenantID, months)
	if err != nil {
		return nil, err
	}
	for outRows.Next() {
		var m string
		var v int64
		if err := outRows.Scan(&m, &v); err != nil {
			outRows.Close()
			return nil, err
		}
		if byMonth[m] == nil {
			byMonth[m] = &agg{}
		}
		byMonth[m].out = v
	}
	outRows.Close()
	keys := make([]string, 0, len(byMonth))
	for k := range byMonth {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	list := make([]map[string]any, 0, len(keys))
	for _, k := range keys {
		a := byMonth[k]
		list = append(list, map[string]any{"month": k, "inflow": a.in, "outflow": a.out, "net": a.in - a.out})
	}
	return list, nil
}

func (s *Store) CreateReseller(ctx context.Context, tenantID xid.ID, name, phone string, commission float64) (xid.ID, error) {
	var id xid.ID
	err := s.Pool.QueryRow(ctx, `
		INSERT INTO resellers (tenant_id, name, phone, commission_percent) VALUES ($1,$2,$3,$4) RETURNING id
	`, tenantID, name, phone, commission).Scan(&id)
	return id, err
}
