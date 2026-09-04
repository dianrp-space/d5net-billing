package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/dianrp/drp-billing/internal/xid"
	"github.com/jackc/pgx/v5"
)

type Customer struct {
	ID            xid.ID    `json:"id"`
	TenantID      xid.ID    `json:"tenant_id"`
	ClusterID     *xid.ID   `json:"cluster_id,omitempty"`
	CustomerCode  string    `json:"customer_code"`
	FullName      string    `json:"full_name"`
	Email         *string   `json:"email,omitempty"`
	Phone         string    `json:"phone"`
	Address       *string   `json:"address,omitempty"`
	Latitude      *float64  `json:"latitude,omitempty"`
	Longitude     *float64  `json:"longitude,omitempty"`
	IsActive      bool      `json:"is_active"`
	PortalEnabled bool      `json:"portal_enabled"`
	CreatedAt     time.Time `json:"created_at"`
	ClusterName   string    `json:"cluster_name,omitempty"`
	ClusterCode   string    `json:"cluster_code,omitempty"`
}

type CustomerFilter struct {
	TenantID  xid.ID
	ClusterID *xid.ID
	Search    string
	Limit     int
	Offset    int
}

func (s *Store) ListCustomers(ctx context.Context, f CustomerFilter) ([]Customer, int64, error) {
	if err := s.SetTenantContext(ctx, f.TenantID); err != nil {
		return nil, 0, err
	}
	where := "WHERE c.tenant_id = $1"
	args := []any{f.TenantID}
	n := 2
	if f.ClusterID != nil {
		where += fmt.Sprintf(" AND c.cluster_id = $%d", n)
		args = append(args, *f.ClusterID)
		n++
	}
	if f.Search != "" {
		where += fmt.Sprintf(" AND (c.full_name ILIKE $%d OR c.phone ILIKE $%d OR c.customer_code ILIKE $%d)", n, n, n)
		args = append(args, "%"+f.Search+"%")
	}
	var total int64
	countQ := fmt.Sprintf("SELECT COUNT(*) FROM customers c %s", where)
	if err := s.Pool.QueryRow(ctx, countQ, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	limit := f.Limit
	if limit <= 0 {
		limit = 20
	}
	q := fmt.Sprintf(`
		SELECT c.id, c.tenant_id, c.cluster_id, c.customer_code, c.full_name, c.email, c.phone, c.address,
		       c.latitude, c.longitude, c.is_active, c.portal_enabled, c.created_at,
		       COALESCE(s.name, ''), COALESCE(s.code, '')
		FROM customers c
		LEFT JOIN sites s ON s.id = c.cluster_id
		%s ORDER BY c.created_at DESC LIMIT %d OFFSET %d
	`, where, limit, f.Offset)
	rows, err := s.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var list []Customer
	for rows.Next() {
		var c Customer
		if err := rows.Scan(&c.ID, &c.TenantID, &c.ClusterID, &c.CustomerCode, &c.FullName, &c.Email, &c.Phone, &c.Address,
			&c.Latitude, &c.Longitude, &c.IsActive, &c.PortalEnabled, &c.CreatedAt, &c.ClusterName, &c.ClusterCode); err != nil {
			return nil, 0, err
		}
		list = append(list, c)
	}
	return list, total, rows.Err()
}

func scanCustomer(row pgx.Row) (*Customer, error) {
	var c Customer
	err := row.Scan(&c.ID, &c.TenantID, &c.ClusterID, &c.CustomerCode, &c.FullName, &c.Email, &c.Phone, &c.Address,
		&c.Latitude, &c.Longitude, &c.IsActive, &c.PortalEnabled, &c.CreatedAt, &c.ClusterName, &c.ClusterCode)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

const customerSelect = `
	SELECT c.id, c.tenant_id, c.cluster_id, c.customer_code, c.full_name, c.email, c.phone, c.address,
	       c.latitude, c.longitude, c.is_active, c.portal_enabled, c.created_at,
	       COALESCE(s.name, ''), COALESCE(s.code, '')
	FROM customers c
	LEFT JOIN sites s ON s.id = c.cluster_id
`

func (s *Store) GetCustomer(ctx context.Context, tenantID xid.ID, id xid.ID) (*Customer, error) {
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return nil, err
	}
	return scanCustomer(s.Pool.QueryRow(ctx, customerSelect+`
		WHERE c.tenant_id = $1 AND c.id = $2
	`, tenantID, id))
}

func (s *Store) CreateCustomer(ctx context.Context, c *Customer) error {
	if err := s.SetTenantContext(ctx, c.TenantID); err != nil {
		return err
	}
	return s.Pool.QueryRow(ctx, `
		INSERT INTO customers (tenant_id, cluster_id, customer_code, full_name, email, phone, address,
		                       latitude, longitude, is_active, portal_enabled)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING id, created_at
	`, c.TenantID, c.ClusterID, c.CustomerCode, c.FullName, c.Email, c.Phone, c.Address,
		c.Latitude, c.Longitude, c.IsActive, c.PortalEnabled).Scan(&c.ID, &c.CreatedAt)
}

func (s *Store) UpdateCustomer(ctx context.Context, c *Customer) error {
	if err := s.SetTenantContext(ctx, c.TenantID); err != nil {
		return err
	}
	tag, err := s.Pool.Exec(ctx, `
		UPDATE customers SET cluster_id=$3, full_name=$4, email=$5, phone=$6, address=$7,
		                      customer_code=$8, latitude=$9, longitude=$10,
		                      is_active=$11, portal_enabled=$12, updated_at=NOW()
		WHERE tenant_id=$1 AND id=$2
	`, c.TenantID, c.ID, c.ClusterID, c.FullName, c.Email, c.Phone, c.Address, c.CustomerCode,
		c.Latitude, c.Longitude, c.IsActive, c.PortalEnabled)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteCustomer(ctx context.Context, tenantID xid.ID, id xid.ID) error {
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return err
	}
	tag, err := s.Pool.Exec(ctx, `DELETE FROM customers WHERE tenant_id=$1 AND id=$2`, tenantID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) GetCustomerByPhone(ctx context.Context, tenantID xid.ID, phone string) (*Customer, error) {
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return nil, err
	}
	return scanCustomer(s.Pool.QueryRow(ctx, customerSelect+`
		WHERE c.tenant_id = $1 AND c.phone = $2
	`, tenantID, phone))
}

func (s *Store) NextCustomerCode(ctx context.Context, tenantID xid.ID) (string, error) {
	var count int64
	err := s.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM customers WHERE tenant_id = $1`, tenantID).Scan(&count)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("CUST-%05d", count+1), nil
}

// ListCustomersWithCoords returns customers that have map coordinates (for FTTH map).
func (s *Store) ListCustomersWithCoords(ctx context.Context, tenantID xid.ID) ([]Customer, error) {
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return nil, err
	}
	rows, err := s.Pool.Query(ctx, customerSelect+`
		WHERE c.tenant_id = $1 AND c.latitude IS NOT NULL AND c.longitude IS NOT NULL
		ORDER BY c.full_name
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []Customer
	for rows.Next() {
		var c Customer
		if err := rows.Scan(&c.ID, &c.TenantID, &c.ClusterID, &c.CustomerCode, &c.FullName, &c.Email, &c.Phone, &c.Address,
			&c.Latitude, &c.Longitude, &c.IsActive, &c.PortalEnabled, &c.CreatedAt, &c.ClusterName, &c.ClusterCode); err != nil {
			return nil, err
		}
		list = append(list, c)
	}
	return list, rows.Err()
}
