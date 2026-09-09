package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dianrp/drp-billing/internal/xid"
	"github.com/jackc/pgx/v5"
)

type Invoice struct {
	ID             xid.ID     `json:"id"`
	TenantID       xid.ID     `json:"tenant_id"`
	CustomerID     xid.ID     `json:"customer_id"`
	SubscriptionID *xid.ID    `json:"subscription_id,omitempty"`
	InvoiceNumber  string     `json:"invoice_number"`
	Subtotal       int64      `json:"subtotal"`
	TaxAmount      int64      `json:"tax_amount"`
	DiscountAmount int64      `json:"discount_amount"`
	TotalAmount    int64      `json:"total_amount"`
	PaidAmount     int64      `json:"paid_amount"`
	Status         string     `json:"status"`
	DueDate        time.Time  `json:"due_date"`
	IssuedAt       *time.Time `json:"issued_at,omitempty"`
	PaidAt         *time.Time `json:"paid_at,omitempty"`
	CustomerName   string     `json:"customer_name,omitempty"`
	CustomerCode   string     `json:"customer_code,omitempty"`
}

type InvoiceItem struct {
	ID          xid.ID `json:"id"`
	Description string `json:"description"`
	Quantity    int    `json:"quantity"`
	UnitPrice   int64  `json:"unit_price"`
	Amount      int64  `json:"amount"`
}

type Payment struct {
	ID            xid.ID     `json:"id"`
	TenantID      xid.ID     `json:"tenant_id"`
	CustomerID    xid.ID     `json:"customer_id"`
	InvoiceID     *xid.ID    `json:"invoice_id,omitempty"`
	Amount        int64      `json:"amount"`
	Method        string     `json:"method"`
	Reference     *string    `json:"reference,omitempty"`
	Status        string     `json:"status"`
	PaidAt        *time.Time `json:"paid_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	CustomerName  string     `json:"customer_name,omitempty"`
	CustomerCode  string     `json:"customer_code,omitempty"`
	InvoiceNumber string     `json:"invoice_number,omitempty"`
}

func (s *Store) NextInvoiceNumber(ctx context.Context, tenantID xid.ID) (string, error) {
	var count int64
	month := time.Now().Format("200601")
	err := s.Pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM invoices WHERE tenant_id = $1 AND invoice_number LIKE $2
	`, tenantID, "INV-"+month+"-%").Scan(&count)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("INV-%s-%04d", month, count+1), nil
}

func (s *Store) CreateInvoice(ctx context.Context, inv *Invoice, items []InvoiceItem) error {
	if err := s.SetTenantContext(ctx, inv.TenantID); err != nil {
		return err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	now := time.Now()
	err = tx.QueryRow(ctx, `
		INSERT INTO invoices (tenant_id, customer_id, subscription_id, invoice_number, subtotal, tax_amount,
		                      discount_amount, total_amount, status, due_date, issued_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING id
	`, inv.TenantID, inv.CustomerID, inv.SubscriptionID, inv.InvoiceNumber, inv.Subtotal, inv.TaxAmount,
		inv.DiscountAmount, inv.TotalAmount, inv.Status, inv.DueDate, now).Scan(&inv.ID)
	if err != nil {
		return err
	}
	inv.IssuedAt = &now

	for _, item := range items {
		_, err = tx.Exec(ctx, `
			INSERT INTO invoice_items (tenant_id, invoice_id, description, quantity, unit_price, amount)
			VALUES ($1,$2,$3,$4,$5,$6)
		`, inv.TenantID, inv.ID, item.Description, item.Quantity, item.UnitPrice, item.Amount)
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) ListInvoices(ctx context.Context, tenantID xid.ID, status, search string, limit, offset int) ([]Invoice, int64, error) {
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return nil, 0, err
	}
	where := "WHERE i.tenant_id = $1"
	args := []any{tenantID}
	if status != "" {
		args = append(args, status)
		where += fmt.Sprintf(" AND i.status = $%d", len(args))
	}
	if strings.TrimSpace(search) != "" {
		args = append(args, "%"+strings.TrimSpace(search)+"%")
		n := len(args)
		where += fmt.Sprintf(" AND (i.invoice_number ILIKE $%d OR c.full_name ILIKE $%d)", n, n)
	}
	var total int64
	if err := s.Pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM invoices i
		JOIN customers c ON c.id = i.customer_id
		`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	if limit <= 0 {
		limit = 20
	}
	args = append(args, limit, offset)
	q := `
		SELECT i.id, i.tenant_id, i.customer_id, i.subscription_id, i.invoice_number, i.subtotal, i.tax_amount,
		       i.discount_amount, i.total_amount, i.paid_amount, i.status, i.due_date, i.issued_at, i.paid_at, c.full_name
		FROM invoices i JOIN customers c ON c.id = i.customer_id
		` + where + fmt.Sprintf(" ORDER BY i.id DESC LIMIT $%d OFFSET $%d", len(args)-1, len(args))
	rows, err := s.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var list []Invoice
	for rows.Next() {
		var inv Invoice
		if err := rows.Scan(&inv.ID, &inv.TenantID, &inv.CustomerID, &inv.SubscriptionID, &inv.InvoiceNumber,
			&inv.Subtotal, &inv.TaxAmount, &inv.DiscountAmount, &inv.TotalAmount, &inv.PaidAmount,
			&inv.Status, &inv.DueDate, &inv.IssuedAt, &inv.PaidAt, &inv.CustomerName); err != nil {
			return nil, 0, err
		}
		list = append(list, inv)
	}
	return list, total, rows.Err()
}

func (s *Store) GetInvoice(ctx context.Context, tenantID xid.ID, id xid.ID) (*Invoice, []InvoiceItem, error) {
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return nil, nil, err
	}
	row := s.Pool.QueryRow(ctx, `
		SELECT i.id, i.tenant_id, i.customer_id, i.subscription_id, i.invoice_number, i.subtotal, i.tax_amount,
		       i.discount_amount, i.total_amount, i.paid_amount, i.status, i.due_date, i.issued_at, i.paid_at, c.full_name
		FROM invoices i JOIN customers c ON c.id = i.customer_id
		WHERE i.tenant_id = $1 AND i.id = $2
	`, tenantID, id)
	var inv Invoice
	err := row.Scan(&inv.ID, &inv.TenantID, &inv.CustomerID, &inv.SubscriptionID, &inv.InvoiceNumber,
		&inv.Subtotal, &inv.TaxAmount, &inv.DiscountAmount, &inv.TotalAmount, &inv.PaidAmount,
		&inv.Status, &inv.DueDate, &inv.IssuedAt, &inv.PaidAt, &inv.CustomerName)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, ErrNotFound
	}
	if err != nil {
		return nil, nil, err
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT id, description, quantity, unit_price, amount FROM invoice_items WHERE invoice_id = $1
	`, id)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var items []InvoiceItem
	for rows.Next() {
		var item InvoiceItem
		if err := rows.Scan(&item.ID, &item.Description, &item.Quantity, &item.UnitPrice, &item.Amount); err != nil {
			return nil, nil, err
		}
		items = append(items, item)
	}
	return &inv, items, rows.Err()
}

func (s *Store) RecordPayment(ctx context.Context, p *Payment) error {
	if err := s.SetTenantContext(ctx, p.TenantID); err != nil {
		return err
	}
	method := strings.TrimSpace(p.Method)
	if method == "" {
		return fmt.Errorf("metode pembayaran wajib")
	}
	status := strings.ToLower(strings.TrimSpace(p.Status))
	if status == "" {
		status = "paid"
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	now := time.Now()
	var paidAt *time.Time
	if status == "paid" || status == "success" {
		paidAt = &now
	}
	err = tx.QueryRow(ctx, `
		INSERT INTO payments (tenant_id, customer_id, invoice_id, amount, method, reference, status, paid_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id, created_at
	`, p.TenantID, p.CustomerID, p.InvoiceID, p.Amount, method, p.Reference, status, paidAt).Scan(&p.ID, &p.CreatedAt)
	if err != nil {
		return err
	}
	p.Method = method
	p.Status = status
	p.PaidAt = paidAt

	if p.InvoiceID != nil && (status == "paid" || status == "success") {
		_, err = tx.Exec(ctx, `
			UPDATE invoices SET paid_amount = paid_amount + $3,
				status = CASE WHEN paid_amount + $3 >= total_amount THEN 'paid' ELSE 'partial' END,
				paid_at = CASE WHEN paid_amount + $3 >= total_amount THEN $4 ELSE paid_at END,
				updated_at = NOW()
			WHERE tenant_id = $1 AND id = $2
		`, p.TenantID, *p.InvoiceID, p.Amount, now)
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) ListPayments(ctx context.Context, tenantID xid.ID, limit, offset int) ([]Payment, int64, error) {
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return nil, 0, err
	}
	var total int64
	if err := s.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM payments WHERE tenant_id = $1`, tenantID).Scan(&total); err != nil {
		return nil, 0, err
	}
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT id, tenant_id, customer_id, invoice_id, amount, method, reference, status, paid_at, created_at
		FROM payments WHERE tenant_id = $1 ORDER BY id DESC LIMIT $2 OFFSET $3
	`, tenantID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var list []Payment
	for rows.Next() {
		var p Payment
		if err := rows.Scan(&p.ID, &p.TenantID, &p.CustomerID, &p.InvoiceID, &p.Amount, &p.Method, &p.Reference, &p.Status, &p.PaidAt, &p.CreatedAt); err != nil {
			return nil, 0, err
		}
		list = append(list, p)
	}
	return list, total, rows.Err()
}

func (s *Store) ListDunningInvoices(ctx context.Context, tenantID xid.ID) ([]Invoice, error) {
	// Unpaid invoices due within 7 days or overdue up to 3 days (H-7 … H+3).
	rows, err := s.Pool.Query(ctx, `
		SELECT i.id, i.tenant_id, i.customer_id, i.subscription_id, i.invoice_number, i.subtotal, i.tax_amount,
		       i.discount_amount, i.total_amount, i.paid_amount, i.status, i.due_date, i.issued_at, i.paid_at, c.full_name
		FROM invoices i JOIN customers c ON c.id = i.customer_id
		WHERE i.tenant_id = $1
		  AND i.status IN ('issued','partial','overdue')
		  AND i.total_amount > i.paid_amount
		  AND i.due_date BETWEEN CURRENT_DATE - INTERVAL '3 days' AND CURRENT_DATE + INTERVAL '7 days'
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []Invoice
	for rows.Next() {
		var inv Invoice
		if err := rows.Scan(&inv.ID, &inv.TenantID, &inv.CustomerID, &inv.SubscriptionID, &inv.InvoiceNumber,
			&inv.Subtotal, &inv.TaxAmount, &inv.DiscountAmount, &inv.TotalAmount, &inv.PaidAmount,
			&inv.Status, &inv.DueDate, &inv.IssuedAt, &inv.PaidAt, &inv.CustomerName); err != nil {
			return nil, err
		}
		list = append(list, inv)
	}
	return list, rows.Err()
}

func (s *Store) ListOverdueInvoices(ctx context.Context, tenantID xid.ID) ([]Invoice, error) {
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return nil, err
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT i.id, i.tenant_id, i.customer_id, i.subscription_id, i.invoice_number, i.subtotal, i.tax_amount,
		       i.discount_amount, i.total_amount, i.paid_amount, i.status, i.due_date, i.issued_at, i.paid_at, c.full_name
		FROM invoices i JOIN customers c ON c.id = i.customer_id
		WHERE i.tenant_id = $1 AND i.status IN ('issued','partial','overdue') AND i.due_date < CURRENT_DATE
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []Invoice
	for rows.Next() {
		var inv Invoice
		if err := rows.Scan(&inv.ID, &inv.TenantID, &inv.CustomerID, &inv.SubscriptionID, &inv.InvoiceNumber,
			&inv.Subtotal, &inv.TaxAmount, &inv.DiscountAmount, &inv.TotalAmount, &inv.PaidAmount,
			&inv.Status, &inv.DueDate, &inv.IssuedAt, &inv.PaidAt, &inv.CustomerName); err != nil {
			return nil, err
		}
		list = append(list, inv)
	}
	return list, rows.Err()
}

func (s *Store) SumOverdueUnpaidForSubscription(ctx context.Context, tenantID xid.ID, subscriptionID xid.ID) (int64, error) {
	var sum int64
	err := s.Pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(total_amount - paid_amount), 0)
		FROM invoices
		WHERE tenant_id = $1 AND subscription_id = $2
		  AND status IN ('issued','partial','overdue')
		  AND due_date < CURRENT_DATE
		  AND total_amount > paid_amount
	`, tenantID, subscriptionID).Scan(&sum)
	return sum, err
}

// SubscriptionHasPastDueUnpaid reports whether the subscription still has unpaid invoices
// past due_date + tenant isolir_grace_days (same rule as auto-isolir).
func (s *Store) SubscriptionHasPastDueUnpaid(ctx context.Context, tenantID, subscriptionID xid.ID) (bool, error) {
	var n int
	err := s.Pool.QueryRow(ctx, `
		SELECT COUNT(*)::int
		FROM invoices i
		WHERE i.tenant_id = $1 AND i.subscription_id = $2
		  AND i.status IN ('issued','partial','overdue')
		  AND i.total_amount > i.paid_amount
		  AND (i.due_date::timestamptz + make_interval(days => $3)) < NOW()
	`, tenantID, subscriptionID, s.IsolirGraceDays(ctx, tenantID)).Scan(&n)
	return n > 0, err
}

func (s *Store) ListCustomerPayments(ctx context.Context, tenantID xid.ID, customerID xid.ID, limit int) ([]Payment, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT p.id, p.tenant_id, p.customer_id, p.invoice_id, p.amount, p.method, p.reference, p.status, p.paid_at, p.created_at,
		       COALESCE(i.invoice_number, '')
		FROM payments p
		LEFT JOIN invoices i ON i.id = p.invoice_id AND i.tenant_id = p.tenant_id
		WHERE p.tenant_id = $1 AND p.customer_id = $2
		ORDER BY COALESCE(p.paid_at, p.created_at) DESC LIMIT $3
	`, tenantID, customerID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []Payment
	for rows.Next() {
		var p Payment
		if err := rows.Scan(&p.ID, &p.TenantID, &p.CustomerID, &p.InvoiceID, &p.Amount, &p.Method, &p.Reference, &p.Status, &p.PaidAt, &p.CreatedAt, &p.InvoiceNumber); err != nil {
			return nil, err
		}
		list = append(list, p)
	}
	return list, rows.Err()
}

func (s *Store) GetWallet(ctx context.Context, tenantID xid.ID, customerID xid.ID) (int64, error) {
	var balance int64
	err := s.Pool.QueryRow(ctx, `
		SELECT COALESCE((SELECT balance FROM wallets WHERE tenant_id=$1 AND customer_id=$2), 0)
	`, tenantID, customerID).Scan(&balance)
	return balance, err
}

func (s *Store) CreditWallet(ctx context.Context, tenantID xid.ID, customerID xid.ID, amount int64, txType, ref, desc string) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var walletID xid.ID
	err = tx.QueryRow(ctx, `
		INSERT INTO wallets (tenant_id, customer_id, balance) VALUES ($1,$2,$3)
		ON CONFLICT (tenant_id, customer_id) DO UPDATE SET balance = wallets.balance + $3, updated_at = NOW()
		RETURNING id
	`, tenantID, customerID, amount).Scan(&walletID)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO wallet_transactions (tenant_id, wallet_id, amount, type, reference, description)
		VALUES ($1,$2,$3,$4,$5,$6)
	`, tenantID, walletID, amount, txType, ref, desc)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
