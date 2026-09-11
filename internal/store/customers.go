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

type Customer struct {
	ID             xid.ID     `json:"id"`
	TenantID       xid.ID     `json:"tenant_id"`
	ClusterID      *xid.ID    `json:"cluster_id,omitempty"`
	CustomerCode   string     `json:"customer_code"`
	FullName       string     `json:"full_name"`
	Email          *string    `json:"email,omitempty"`
	Phone          string     `json:"phone"`
	Address        *string    `json:"address,omitempty"`
	Latitude       *float64   `json:"latitude,omitempty"`
	Longitude      *float64   `json:"longitude,omitempty"`
	IdentityType   *string    `json:"identity_type,omitempty"`
	IdentityNumber *string    `json:"identity_number,omitempty"`
	IsActive       bool       `json:"is_active"`
	PortalEnabled  bool       `json:"portal_enabled"`
	DismantledAt   *time.Time `json:"dismantled_at,omitempty"`
	ServiceStatus  string     `json:"service_status"`
	// SubStatus is the aggregate live subscription state (active/isolir/overdue)
	// used to derive ServiceStatus; not serialized on its own.
	SubStatus     string    `json:"-"`
	PasswordHash  string    `json:"-"`
	CreatedAt     time.Time `json:"created_at"`
	ClusterName   string    `json:"cluster_name,omitempty"`
	ClusterCode   string    `json:"cluster_code,omitempty"`
	ResellerID    *xid.ID   `json:"reseller_id,omitempty"`
	ResellerName  string    `json:"reseller_name,omitempty"`
	SalesUserID   *xid.ID   `json:"sales_user_id,omitempty"`
	SalesUserName string    `json:"sales_user_name,omitempty"`
}

func (c *Customer) IsDismantled() bool {
	return c != nil && c.DismantledAt != nil
}

func (c *Customer) FillServiceStatus() {
	if c == nil {
		return
	}
	if c.DismantledAt != nil {
		c.ServiceStatus = "dismantled"
		return
	}
	if !c.IsActive {
		c.ServiceStatus = "inactive"
		return
	}
	switch c.SubStatus {
	case "isolir", "overdue", "active":
		c.ServiceStatus = c.SubStatus
	default:
		// Customer aktif tanpa langganan live.
		c.ServiceStatus = "active"
	}
}

type CustomerFilter struct {
	TenantID      xid.ID
	ClusterID     *xid.ID
	Search        string
	IsActive      *bool
	ServiceStatus string
	Limit         int
	Offset        int
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
	switch strings.ToLower(strings.TrimSpace(f.ServiceStatus)) {
	case "dismantled", "cabut":
		where += " AND c.dismantled_at IS NOT NULL"
	case "isolir", "suspend", "suspended":
		where += " AND c.dismantled_at IS NULL AND c.is_active = true AND EXISTS (" +
			"SELECT 1 FROM subscriptions s WHERE s.tenant_id = c.tenant_id AND s.customer_id = c.id AND s.status = 'suspended')"
	case "overdue", "tunggakan":
		where += " AND c.dismantled_at IS NULL AND c.is_active = true AND EXISTS (" +
			"SELECT 1 FROM subscriptions s WHERE s.tenant_id = c.tenant_id AND s.customer_id = c.id AND s.status = 'overdue')" +
			" AND NOT EXISTS (SELECT 1 FROM subscriptions s WHERE s.tenant_id = c.tenant_id AND s.customer_id = c.id AND s.status = 'suspended')"
	case "active", "aktif":
		where += " AND c.dismantled_at IS NULL AND c.is_active = true AND NOT EXISTS (" +
			"SELECT 1 FROM subscriptions s WHERE s.tenant_id = c.tenant_id AND s.customer_id = c.id AND s.status IN ('suspended','overdue'))"
	case "inactive", "nonaktif":
		where += " AND c.dismantled_at IS NULL AND c.is_active = false"
	default:
		if f.IsActive != nil {
			if *f.IsActive {
				where += " AND c.is_active = true AND c.dismantled_at IS NULL"
			} else {
				where += " AND c.is_active = false"
			}
		}
	}
	if f.Search != "" {
		where += fmt.Sprintf(" AND (c.full_name ILIKE $%d OR c.phone ILIKE $%d OR c.customer_code ILIKE $%d OR COALESCE(c.email,'') ILIKE $%d)", n, n, n, n)
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
	q := customerSelect + fmt.Sprintf(`
		%s ORDER BY c.created_at DESC LIMIT %d OFFSET %d
	`, where, limit, f.Offset)
	rows, err := s.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var list []Customer
	for rows.Next() {
		c, err := scanCustomer(rows)
		if err != nil {
			return nil, 0, err
		}
		list = append(list, *c)
	}
	return list, total, rows.Err()
}

func customerScanDest(c *Customer) []any {
	return []any{
		&c.ID, &c.TenantID, &c.ClusterID, &c.CustomerCode, &c.FullName, &c.Email, &c.Phone, &c.Address,
		&c.Latitude, &c.Longitude, &c.IdentityType, &c.IdentityNumber, &c.IsActive, &c.PortalEnabled, &c.DismantledAt, &c.CreatedAt,
		&c.ClusterName, &c.ClusterCode, &c.ResellerID, &c.ResellerName, &c.SalesUserID, &c.SalesUserName, &c.SubStatus,
	}
}

func scanCustomer(row interface{ Scan(dest ...any) error }) (*Customer, error) {
	var c Customer
	err := row.Scan(customerScanDest(&c)...)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	c.FillServiceStatus()
	return &c, nil
}

const customerSelect = `
	SELECT c.id, c.tenant_id, c.cluster_id, c.customer_code, c.full_name, c.email, c.phone, c.address,
	       c.latitude, c.longitude, c.identity_type, c.identity_number, c.is_active, c.portal_enabled, c.dismantled_at, c.created_at,
	       COALESCE(s.name, ''), COALESCE(s.code, ''),
	       c.reseller_id, COALESCE(r.name, ''), c.sales_user_id, COALESCE(u.full_name, ''),
	       COALESCE((
	           SELECT CASE
	               WHEN bool_or(sub.status = 'suspended') THEN 'isolir'
	               WHEN bool_or(sub.status = 'overdue') THEN 'overdue'
	               WHEN bool_or(sub.status = 'active') THEN 'active'
	               ELSE ''
	           END
	           FROM subscriptions sub
	           WHERE sub.tenant_id = c.tenant_id AND sub.customer_id = c.id
	             AND sub.status IN ('active','suspended','overdue')
	       ), '')
	FROM customers c
	LEFT JOIN sites s ON s.id = c.cluster_id
	LEFT JOIN resellers r ON r.id = c.reseller_id
	LEFT JOIN users u ON u.id = c.sales_user_id
`

func (s *Store) GetCustomer(ctx context.Context, tenantID xid.ID, id xid.ID) (*Customer, error) {
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return nil, err
	}
	return scanCustomer(s.Pool.QueryRow(ctx, customerSelect+`
		WHERE c.tenant_id = $1 AND c.id = $2
	`, tenantID, id))
}

func formatSiteLabel(name, code string) string {
	name = strings.TrimSpace(name)
	code = strings.TrimSpace(code)
	switch {
	case name != "" && code != "" && !strings.EqualFold(name, code):
		return name + " (" + code + ")"
	case name != "":
		return name
	default:
		return code
	}
}

// CustomerNetworkLabels returns the customer's cluster and NAS/router names for ops alerts.
// Cluster prefers the customer's site; if empty, falls back to the subscription router's site.
// Router comes from the latest provisionable subscription that has a router_id.
func (s *Store) CustomerNetworkLabels(ctx context.Context, tenantID, customerID xid.ID) (cluster, router string, err error) {
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return "", "", err
	}
	var clusterName, clusterCode, routerName, routerClusterName, routerClusterCode string
	err = s.Pool.QueryRow(ctx, `
		SELECT COALESCE(site.name, ''), COALESCE(site.code, ''),
		       COALESCE(r.name, ''), COALESCE(rsite.name, ''), COALESCE(rsite.code, '')
		FROM customers c
		LEFT JOIN sites site ON site.id = c.cluster_id
		LEFT JOIN LATERAL (
			SELECT s.router_id
			FROM subscriptions s
			WHERE s.tenant_id = c.tenant_id AND s.customer_id = c.id AND s.router_id IS NOT NULL
			ORDER BY CASE s.status
				WHEN 'active' THEN 0
				WHEN 'overdue' THEN 1
				WHEN 'suspended' THEN 2
				ELSE 3
			END, s.id DESC
			LIMIT 1
		) sub ON true
		LEFT JOIN routers r ON r.id = sub.router_id AND r.tenant_id = c.tenant_id
		LEFT JOIN sites rsite ON rsite.id = r.site_id
		WHERE c.tenant_id = $1 AND c.id = $2
	`, tenantID, customerID).Scan(&clusterName, &clusterCode, &routerName, &routerClusterName, &routerClusterCode)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", ErrNotFound
	}
	if err != nil {
		return "", "", err
	}
	cluster = formatSiteLabel(clusterName, clusterCode)
	if cluster == "" {
		cluster = formatSiteLabel(routerClusterName, routerClusterCode)
	}
	return cluster, strings.TrimSpace(routerName), nil
}

func (s *Store) CreateCustomer(ctx context.Context, c *Customer) error {
	if err := s.SetTenantContext(ctx, c.TenantID); err != nil {
		return err
	}
	c.ResellerID, c.SalesUserID = NormalizeAttribution(c.ResellerID, c.SalesUserID)
	var hash any
	if strings.TrimSpace(c.PasswordHash) != "" {
		hash = c.PasswordHash
	}
	return s.Pool.QueryRow(ctx, `
		INSERT INTO customers (tenant_id, cluster_id, customer_code, full_name, email, phone, address,
		                       latitude, longitude, identity_type, identity_number, is_active, portal_enabled, password_hash, reseller_id, sales_user_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16) RETURNING id, created_at
	`, c.TenantID, c.ClusterID, c.CustomerCode, c.FullName, c.Email, c.Phone, c.Address,
		c.Latitude, c.Longitude, c.IdentityType, c.IdentityNumber, c.IsActive, c.PortalEnabled, hash, c.ResellerID, c.SalesUserID).Scan(&c.ID, &c.CreatedAt)
}

func (s *Store) UpdateCustomer(ctx context.Context, c *Customer) error {
	if err := s.SetTenantContext(ctx, c.TenantID); err != nil {
		return err
	}
	c.ResellerID, c.SalesUserID = NormalizeAttribution(c.ResellerID, c.SalesUserID)
	var (
		tag interface{ RowsAffected() int64 }
		err error
	)
	if strings.TrimSpace(c.PasswordHash) != "" {
		tag, err = s.Pool.Exec(ctx, `
			UPDATE customers SET cluster_id=$3, full_name=$4, email=$5, phone=$6, address=$7,
			                      customer_code=$8, latitude=$9, longitude=$10,
			                      identity_type=$11, identity_number=$12,
			                      is_active=$13, portal_enabled=$14, password_hash=$15,
			                      reseller_id=$16, sales_user_id=$17, updated_at=NOW()
			WHERE tenant_id=$1 AND id=$2
		`, c.TenantID, c.ID, c.ClusterID, c.FullName, c.Email, c.Phone, c.Address, c.CustomerCode,
			c.Latitude, c.Longitude, c.IdentityType, c.IdentityNumber, c.IsActive, c.PortalEnabled, c.PasswordHash, c.ResellerID, c.SalesUserID)
	} else {
		tag, err = s.Pool.Exec(ctx, `
			UPDATE customers SET cluster_id=$3, full_name=$4, email=$5, phone=$6, address=$7,
			                      customer_code=$8, latitude=$9, longitude=$10,
			                      identity_type=$11, identity_number=$12,
			                      is_active=$13, portal_enabled=$14,
			                      reseller_id=$15, sales_user_id=$16, updated_at=NOW()
			WHERE tenant_id=$1 AND id=$2
		`, c.TenantID, c.ID, c.ClusterID, c.FullName, c.Email, c.Phone, c.Address, c.CustomerCode,
			c.Latitude, c.Longitude, c.IdentityType, c.IdentityNumber, c.IsActive, c.PortalEnabled, c.ResellerID, c.SalesUserID)
	}
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

func (s *Store) GetCustomerByCode(ctx context.Context, tenantID xid.ID, code string) (*Customer, error) {
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return nil, err
	}
	return scanCustomer(s.Pool.QueryRow(ctx, customerSelect+`
		WHERE c.tenant_id = $1 AND c.customer_code = $2
	`, tenantID, code))
}

func (s *Store) GetCustomerByPhone(ctx context.Context, tenantID xid.ID, phone string) (*Customer, error) {
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return nil, err
	}
	return scanCustomer(s.Pool.QueryRow(ctx, customerSelect+`
		WHERE c.tenant_id = $1 AND c.phone = $2
	`, tenantID, phone))
}

// ListCustomersByPhone returns ALL customers sharing one phone number (e.g. one
// payer for several installations), oldest first. Used by the portal combined login.
func (s *Store) ListCustomersByPhone(ctx context.Context, tenantID xid.ID, phone string) ([]Customer, error) {
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return nil, err
	}
	rows, err := s.Pool.Query(ctx, customerSelect+`
		WHERE c.tenant_id = $1 AND c.phone = $2
		ORDER BY c.created_at ASC
	`, tenantID, phone)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []Customer
	for rows.Next() {
		c, err := scanCustomer(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, *c)
	}
	return list, rows.Err()
}

func (s *Store) NextCustomerCode(ctx context.Context, tenantID xid.ID) (string, error) {
	now := time.Now()
	for i := 0; i < 100; i++ {
		var count int64
		err := s.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM customers WHERE tenant_id = $1`, tenantID).Scan(&count)
		if err != nil {
			return "", err
		}
		code := fmt.Sprintf("%s-%s%04d", DefaultCustomerCodePrefix, now.Format("200601"), count+1+int64(i))
		taken, err := s.customerCodeTaken(ctx, tenantID, code)
		if err != nil {
			return "", err
		}
		if !taken {
			return code, nil
		}
	}
	return "", fmt.Errorf("gagal membuat kode pelanggan unik")
}

func (s *Store) customerCodeTaken(ctx context.Context, tenantID xid.ID, code string) (bool, error) {
	var exists bool
	err := s.Pool.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM customers WHERE tenant_id = $1 AND customer_code = $2)
	`, tenantID, code).Scan(&exists)
	return exists, err
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
		c, err := scanCustomer(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, *c)
	}
	return list, rows.Err()
}

func (s *Store) DismantleCustomer(ctx context.Context, tenantID, id xid.ID) error {
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return err
	}
	tag, err := s.Pool.Exec(ctx, `
		UPDATE customers
		SET is_active = false, portal_enabled = false, dismantled_at = NOW(), updated_at = NOW()
		WHERE tenant_id = $1 AND id = $2 AND dismantled_at IS NULL
	`, tenantID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
