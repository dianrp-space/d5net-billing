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

const commissionSettingKey = "commission.new_customer_amount"

const (
	CommissionBasisNewCustomer = "new_customer_flat"
	CommissionBasisAcquisition = "acquisition_flat"
)

// CommissionSettings is tenant config for flat IDR commissions by type.
type CommissionSettings struct {
	NewCustomerAmount int64 `json:"new_customer_amount"`
	AcquisitionAmount int64 `json:"acquisition_amount"`
}

type CommissionEntry struct {
	ID            xid.ID     `json:"id"`
	TenantID      xid.ID     `json:"tenant_id"`
	CustomerID    xid.ID     `json:"customer_id"`
	CustomerName  string     `json:"customer_name,omitempty"`
	CustomerCode  string     `json:"customer_code,omitempty"`
	LeadID        *xid.ID    `json:"lead_id,omitempty"`
	ResellerID    *xid.ID    `json:"reseller_id,omitempty"`
	ResellerName  string     `json:"reseller_name,omitempty"`
	SalesUserID   *xid.ID    `json:"sales_user_id,omitempty"`
	SalesUserName string     `json:"sales_user_name,omitempty"`
	Amount        int64      `json:"amount"`
	Basis         string     `json:"basis"`
	Status        string     `json:"status"`
	Note          *string    `json:"note,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	PaidAt        *time.Time `json:"paid_at,omitempty"`
}

// NormalizeAttribution prefers reseller over staff when both are set.
func NormalizeAttribution(resellerID, salesUserID *xid.ID) (*xid.ID, *xid.ID) {
	if resellerID != nil && !xid.IsNil(*resellerID) {
		return resellerID, nil
	}
	if salesUserID != nil && !xid.IsNil(*salesUserID) {
		return nil, salesUserID
	}
	return nil, nil
}

// NormalizeCommissionBasis maps aliases to stored basis values.
func NormalizeCommissionBasis(basis string) string {
	switch strings.TrimSpace(strings.ToLower(basis)) {
	case CommissionBasisAcquisition, "acquisition", "akuisisi":
		return CommissionBasisAcquisition
	default:
		return CommissionBasisNewCustomer
	}
}

func (cfg CommissionSettings) AmountForBasis(basis string) int64 {
	switch NormalizeCommissionBasis(basis) {
	case CommissionBasisAcquisition:
		return cfg.AcquisitionAmount
	default:
		return cfg.NewCustomerAmount
	}
}

func (s *Store) GetCommissionSettings(ctx context.Context, tenantID xid.ID) (CommissionSettings, error) {
	var cfg CommissionSettings
	err := s.GetSettingJSON(ctx, tenantID, commissionSettingKey, &cfg)
	if errors.Is(err, ErrNotFound) {
		return CommissionSettings{}, nil
	}
	return cfg, err
}

func (s *Store) SetCommissionSettings(ctx context.Context, tenantID xid.ID, cfg CommissionSettings) error {
	if cfg.NewCustomerAmount < 0 || cfg.AcquisitionAmount < 0 {
		return fmt.Errorf("nominal komisi tidak boleh negatif")
	}
	return s.UpsertSettingJSON(ctx, tenantID, commissionSettingKey, cfg)
}

// CreditCommission creates a pending commission entry for the given basis when amount > 0 and a payee is set.
// Returns nil entry (no error) when skipped (amount 0 or no payee).
func (s *Store) CreditCommission(ctx context.Context, tenantID, customerID xid.ID, leadID *xid.ID, resellerID, salesUserID *xid.ID, basis string) (*CommissionEntry, error) {
	rID, sID := NormalizeAttribution(resellerID, salesUserID)
	if rID == nil && sID == nil {
		return nil, nil
	}
	basis = NormalizeCommissionBasis(basis)
	cfg, err := s.GetCommissionSettings(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	amount := cfg.AmountForBasis(basis)
	if amount <= 0 {
		return nil, nil
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var e CommissionEntry
	err = tx.QueryRow(ctx, `
		INSERT INTO commission_entries (tenant_id, customer_id, lead_id, reseller_id, sales_user_id, amount, basis, status)
		VALUES ($1,$2,$3,$4,$5,$6,$7,'pending')
		RETURNING id, created_at
	`, tenantID, customerID, leadID, rID, sID, amount, basis).Scan(&e.ID, &e.CreatedAt)
	if err != nil {
		return nil, err
	}
	if rID != nil {
		tag, err := tx.Exec(ctx, `
			UPDATE resellers SET balance = balance + $3 WHERE tenant_id=$1 AND id=$2
		`, tenantID, *rID, amount)
		if err != nil {
			return nil, err
		}
		if tag.RowsAffected() == 0 {
			return nil, fmt.Errorf("reseller tidak ditemukan")
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	e.TenantID = tenantID
	e.CustomerID = customerID
	e.LeadID = leadID
	e.ResellerID = rID
	e.SalesUserID = sID
	e.Amount = amount
	e.Basis = basis
	e.Status = "pending"
	return &e, nil
}

// CreditCommissionForNewCustomer is a convenience wrapper for new-customer flat commission.
func (s *Store) CreditCommissionForNewCustomer(ctx context.Context, tenantID, customerID xid.ID, leadID *xid.ID, resellerID, salesUserID *xid.ID) (*CommissionEntry, error) {
	return s.CreditCommission(ctx, tenantID, customerID, leadID, resellerID, salesUserID, CommissionBasisNewCustomer)
}

func (s *Store) ListCommissionEntries(ctx context.Context, tenantID xid.ID, status string, limit, offset int) ([]CommissionEntry, int64, error) {
	if limit <= 0 {
		limit = 50
	}
	where := `WHERE e.tenant_id=$1`
	args := []any{tenantID}
	if status == "pending" || status == "paid" || status == "void" {
		where += ` AND e.status=$2`
		args = append(args, status)
	}
	var total int64
	if err := s.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM commission_entries e `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, limit, offset)
	lim, off := len(args)-1, len(args)
	q := fmt.Sprintf(`
		SELECT e.id, e.tenant_id, e.customer_id, COALESCE(c.full_name,''), COALESCE(c.customer_code,''),
		       e.lead_id, e.reseller_id, COALESCE(r.name,''), e.sales_user_id, COALESCE(u.full_name,''),
		       e.amount, e.basis, e.status, e.note, e.created_at, e.paid_at
		FROM commission_entries e
		LEFT JOIN customers c ON c.id = e.customer_id
		LEFT JOIN resellers r ON r.id = e.reseller_id
		LEFT JOIN users u ON u.id = e.sales_user_id
		%s ORDER BY e.created_at DESC LIMIT $%d OFFSET $%d
	`, where, lim, off)
	rows, err := s.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var list []CommissionEntry
	for rows.Next() {
		var e CommissionEntry
		if err := rows.Scan(
			&e.ID, &e.TenantID, &e.CustomerID, &e.CustomerName, &e.CustomerCode,
			&e.LeadID, &e.ResellerID, &e.ResellerName, &e.SalesUserID, &e.SalesUserName,
			&e.Amount, &e.Basis, &e.Status, &e.Note, &e.CreatedAt, &e.PaidAt,
		); err != nil {
			return nil, 0, err
		}
		list = append(list, e)
	}
	return list, total, rows.Err()
}

// ListCommissionEntriesForExport returns all matching entries (no pagination),
// oldest first, for CSV export and per-recipient recap.
func (s *Store) ListCommissionEntriesForExport(ctx context.Context, tenantID xid.ID, from, to *time.Time, status string) ([]CommissionEntry, error) {
	where := `WHERE e.tenant_id=$1`
	args := []any{tenantID}
	n := 2
	if status == "pending" || status == "paid" || status == "void" {
		where += fmt.Sprintf(" AND e.status=$%d", n)
		args = append(args, status)
		n++
	}
	if from != nil {
		where += fmt.Sprintf(" AND e.created_at >= $%d", n)
		args = append(args, *from)
		n++
	}
	if to != nil {
		where += fmt.Sprintf(" AND e.created_at < $%d", n)
		args = append(args, *to)
		n++
	}
	q := `
		SELECT e.id, e.tenant_id, e.customer_id, COALESCE(c.full_name,''), COALESCE(c.customer_code,''),
		       e.lead_id, e.reseller_id, COALESCE(r.name,''), e.sales_user_id, COALESCE(u.full_name,''),
		       e.amount, e.basis, e.status, e.note, e.created_at, e.paid_at
		FROM commission_entries e
		LEFT JOIN customers c ON c.id = e.customer_id
		LEFT JOIN resellers r ON r.id = e.reseller_id
		LEFT JOIN users u ON u.id = e.sales_user_id
		` + where + ` ORDER BY e.created_at ASC`
	rows, err := s.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []CommissionEntry
	for rows.Next() {
		var e CommissionEntry
		if err := rows.Scan(
			&e.ID, &e.TenantID, &e.CustomerID, &e.CustomerName, &e.CustomerCode,
			&e.LeadID, &e.ResellerID, &e.ResellerName, &e.SalesUserID, &e.SalesUserName,
			&e.Amount, &e.Basis, &e.Status, &e.Note, &e.CreatedAt, &e.PaidAt,
		); err != nil {
			return nil, err
		}
		list = append(list, e)
	}
	return list, rows.Err()
}

func (s *Store) GetCommissionEntry(ctx context.Context, tenantID, id xid.ID) (*CommissionEntry, error) {
	var e CommissionEntry
	err := s.Pool.QueryRow(ctx, `
		SELECT e.id, e.tenant_id, e.customer_id, COALESCE(c.full_name,''), COALESCE(c.customer_code,''),
		       e.lead_id, e.reseller_id, COALESCE(r.name,''), e.sales_user_id, COALESCE(u.full_name,''),
		       e.amount, e.basis, e.status, e.note, e.created_at, e.paid_at
		FROM commission_entries e
		LEFT JOIN customers c ON c.id = e.customer_id
		LEFT JOIN resellers r ON r.id = e.reseller_id
		LEFT JOIN users u ON u.id = e.sales_user_id
		WHERE e.tenant_id=$1 AND e.id=$2
	`, tenantID, id).Scan(
		&e.ID, &e.TenantID, &e.CustomerID, &e.CustomerName, &e.CustomerCode,
		&e.LeadID, &e.ResellerID, &e.ResellerName, &e.SalesUserID, &e.SalesUserName,
		&e.Amount, &e.Basis, &e.Status, &e.Note, &e.CreatedAt, &e.PaidAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

// MarkCommissionPaid sets status=paid (no balance change; balance already credited on create for reseller).
func (s *Store) MarkCommissionPaid(ctx context.Context, tenantID, id xid.ID) error {
	now := time.Now()
	tag, err := s.Pool.Exec(ctx, `
		UPDATE commission_entries SET status='paid', paid_at=$3
		WHERE tenant_id=$1 AND id=$2 AND status='pending'
	`, tenantID, id, now)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// VoidCommissionEntry voids a pending/paid entry and reverses reseller balance if applicable.
func (s *Store) VoidCommissionEntry(ctx context.Context, tenantID, id xid.ID) error {
	e, err := s.GetCommissionEntry(ctx, tenantID, id)
	if err != nil {
		return err
	}
	if e.Status == "void" {
		return nil
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `
		UPDATE commission_entries SET status='void' WHERE tenant_id=$1 AND id=$2 AND status IN ('pending','paid')
	`, tenantID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	if e.ResellerID != nil {
		_, err = tx.Exec(ctx, `
			UPDATE resellers SET balance = GREATEST(0, balance - $3) WHERE tenant_id=$1 AND id=$2
		`, tenantID, *e.ResellerID, e.Amount)
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
