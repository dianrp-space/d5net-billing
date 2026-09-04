package store

import (
	"context"
	"errors"
	"github.com/dianrp/drp-billing/internal/xid"
	"time"

	"github.com/jackc/pgx/v5"
)

type Ticket struct {
	ID          xid.ID     `json:"id"`
	TenantID    xid.ID     `json:"tenant_id"`
	CustomerID  *xid.ID    `json:"customer_id,omitempty"`
	Subject     string     `json:"subject"`
	Description *string    `json:"description,omitempty"`
	Category    string     `json:"category"`
	Priority    string     `json:"priority"`
	Status      string     `json:"status"`
	AssignedTo  *xid.ID    `json:"assigned_to,omitempty"`
	SLADueAt    *time.Time `json:"sla_due_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

func (s *Store) ListTickets(ctx context.Context, tenantID xid.ID, status string, limit, offset int) ([]Ticket, int64, error) {
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return nil, 0, err
	}
	where := "WHERE tenant_id = $1"
	args := []any{tenantID}
	if status != "" {
		where += " AND status = $2"
		args = append(args, status)
	}
	var total int64
	if err := s.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM tickets "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	if limit <= 0 {
		limit = 20
	}
	args = append(args, limit, offset)
	rows, err := s.Pool.Query(ctx, `
		SELECT id, tenant_id, customer_id, subject, description, category, priority, status, assigned_to, sla_due_at, created_at
		FROM tickets `+where+` ORDER BY id DESC LIMIT $`+itoa(len(args)-1)+` OFFSET $`+itoa(len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var list []Ticket
	for rows.Next() {
		var t Ticket
		if err := rows.Scan(&t.ID, &t.TenantID, &t.CustomerID, &t.Subject, &t.Description, &t.Category, &t.Priority, &t.Status, &t.AssignedTo, &t.SLADueAt, &t.CreatedAt); err != nil {
			return nil, 0, err
		}
		list = append(list, t)
	}
	return list, total, rows.Err()
}

func (s *Store) CreateTicket(ctx context.Context, t *Ticket) error {
	if err := s.SetTenantContext(ctx, t.TenantID); err != nil {
		return err
	}
	sla := time.Now().Add(24 * time.Hour)
	if t.Priority == "high" {
		sla = time.Now().Add(4 * time.Hour)
	}
	return s.Pool.QueryRow(ctx, `
		INSERT INTO tickets (tenant_id, customer_id, subject, description, category, priority, status, sla_due_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id, created_at
	`, t.TenantID, t.CustomerID, t.Subject, t.Description, t.Category, t.Priority, t.Status, sla).Scan(&t.ID, &t.CreatedAt)
}

func (s *Store) UpdateTicketStatus(ctx context.Context, tenantID xid.ID, id xid.ID, status string) error {
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return err
	}
	_, err := s.Pool.Exec(ctx, `
		UPDATE tickets SET status=$3, resolved_at=CASE WHEN $3='resolved' THEN NOW() ELSE resolved_at END, updated_at=NOW()
		WHERE tenant_id=$1 AND id=$2
	`, tenantID, id, status)
	return err
}

type WorkOrder struct {
	ID           xid.ID     `json:"id"`
	TenantID     xid.ID     `json:"tenant_id"`
	CustomerID   *xid.ID    `json:"customer_id,omitempty"`
	TechnicianID *xid.ID    `json:"technician_id,omitempty"`
	Type         string     `json:"type"`
	Status       string     `json:"status"`
	ScheduledAt  *time.Time `json:"scheduled_at,omitempty"`
	Notes        *string    `json:"notes,omitempty"`
}

func (s *Store) CreateWorkOrder(ctx context.Context, wo *WorkOrder) error {
	return s.Pool.QueryRow(ctx, `
		INSERT INTO work_orders (tenant_id, customer_id, technician_id, type, status, scheduled_at, notes)
		VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id
	`, wo.TenantID, wo.CustomerID, wo.TechnicianID, wo.Type, wo.Status, wo.ScheduledAt, wo.Notes).Scan(&wo.ID)
}

func (s *Store) CheckInWorkOrder(ctx context.Context, tenantID xid.ID, id xid.ID, lat, lng float64) error {
	_, err := s.Pool.Exec(ctx, `
		UPDATE work_orders SET check_in_at=NOW(), check_in_lat=$3, check_in_lng=$4, status='in_progress', updated_at=NOW()
		WHERE tenant_id=$1 AND id=$2
	`, tenantID, id, lat, lng)
	return err
}

type VoucherBatch struct {
	ID        xid.ID     `json:"id"`
	TenantID  xid.ID     `json:"tenant_id"`
	PlanID    *xid.ID    `json:"plan_id,omitempty"`
	Name      string     `json:"name"`
	Quantity  int        `json:"quantity"`
	Price     int64      `json:"price"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

func (s *Store) CreateVoucherBatch(ctx context.Context, b *VoucherBatch, codes []string) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	err = tx.QueryRow(ctx, `
		INSERT INTO voucher_batches (tenant_id, plan_id, name, quantity, price, expires_at)
		VALUES ($1,$2,$3,$4,$5,$6) RETURNING id
	`, b.TenantID, b.PlanID, b.Name, len(codes), b.Price, b.ExpiresAt).Scan(&b.ID)
	if err != nil {
		return err
	}
	for _, code := range codes {
		_, err = tx.Exec(ctx, `
			INSERT INTO vouchers (tenant_id, batch_id, code, expires_at) VALUES ($1,$2,$3,$4)
		`, b.TenantID, b.ID, code, b.ExpiresAt)
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) RedeemVoucher(ctx context.Context, tenantID xid.ID, code string, customerID xid.ID) error {
	result, err := s.Pool.Exec(ctx, `
		UPDATE vouchers SET status='used', used_by=$3, used_at=NOW()
		WHERE tenant_id=$1 AND code=$2 AND status='available'
	`, tenantID, code, customerID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

type ODP struct {
	ID        xid.ID   `json:"id"`
	TenantID  xid.ID   `json:"tenant_id"`
	ClusterID *xid.ID  `json:"cluster_id,omitempty"`
	ODCID     *xid.ID  `json:"odc_id,omitempty"`
	Name      string   `json:"name"`
	Code      string   `json:"code"`
	Address   *string  `json:"address,omitempty"`
	Latitude  *float64 `json:"latitude,omitempty"`
	Longitude *float64 `json:"longitude,omitempty"`
	PortCount int      `json:"port_count"`
	UsedPorts int      `json:"used_ports"`
	FreePorts int      `json:"free_ports"`
}

type ODPPort struct {
	ID               xid.ID  `json:"id"`
	TenantID         xid.ID  `json:"tenant_id"`
	ODPID            xid.ID  `json:"odp_id"`
	PortNumber       int     `json:"port_number"`
	Status           string  `json:"status"`
	CustomerID       *xid.ID `json:"customer_id,omitempty"`
	SubscriptionID   *xid.ID `json:"subscription_id,omitempty"`
	CustomerName     string  `json:"customer_name,omitempty"`
	SubscriptionUser string  `json:"subscription_username,omitempty"`
}

func (s *Store) ListODPs(ctx context.Context, tenantID xid.ID) ([]ODP, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT o.id, o.tenant_id, o.cluster_id, o.odc_id, o.name, o.code, o.address, o.latitude, o.longitude, o.port_count,
		       COALESCE((SELECT COUNT(*) FROM odp_ports p WHERE p.odp_id = o.id AND p.status = 'used'), 0),
		       COALESCE((SELECT COUNT(*) FROM odp_ports p WHERE p.odp_id = o.id AND p.status = 'available'), 0)
		FROM odps o WHERE o.tenant_id = $1 ORDER BY o.name
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []ODP
	for rows.Next() {
		var o ODP
		if err := rows.Scan(&o.ID, &o.TenantID, &o.ClusterID, &o.ODCID, &o.Name, &o.Code, &o.Address, &o.Latitude, &o.Longitude, &o.PortCount, &o.UsedPorts, &o.FreePorts); err != nil {
			return nil, err
		}
		list = append(list, o)
	}
	return list, rows.Err()
}

func (s *Store) GetODP(ctx context.Context, tenantID, id xid.ID) (*ODP, error) {
	row := s.Pool.QueryRow(ctx, `
		SELECT o.id, o.tenant_id, o.cluster_id, o.odc_id, o.name, o.code, o.address, o.latitude, o.longitude, o.port_count,
		       COALESCE((SELECT COUNT(*) FROM odp_ports p WHERE p.odp_id = o.id AND p.status = 'used'), 0),
		       COALESCE((SELECT COUNT(*) FROM odp_ports p WHERE p.odp_id = o.id AND p.status = 'available'), 0)
		FROM odps o WHERE o.tenant_id = $1 AND o.id = $2
	`, tenantID, id)
	var o ODP
	err := row.Scan(&o.ID, &o.TenantID, &o.ClusterID, &o.ODCID, &o.Name, &o.Code, &o.Address, &o.Latitude, &o.Longitude, &o.PortCount, &o.UsedPorts, &o.FreePorts)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &o, nil
}

func (s *Store) CreateODP(ctx context.Context, o *ODP) error {
	err := s.Pool.QueryRow(ctx, `
		INSERT INTO odps (tenant_id, cluster_id, odc_id, name, code, address, latitude, longitude, port_count)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id
	`, o.TenantID, o.ClusterID, o.ODCID, o.Name, o.Code, o.Address, o.Latitude, o.Longitude, o.PortCount).Scan(&o.ID)
	if err != nil {
		return err
	}
	for i := 1; i <= o.PortCount; i++ {
		_, err = s.Pool.Exec(ctx, `
			INSERT INTO odp_ports (tenant_id, odp_id, port_number, status) VALUES ($1,$2,$3,'available')
		`, o.TenantID, o.ID, i)
		if err != nil {
			return err
		}
	}
	o.FreePorts = o.PortCount
	o.UsedPorts = 0
	return nil
}

func (s *Store) UpdateODP(ctx context.Context, o *ODP) error {
	tag, err := s.Pool.Exec(ctx, `
		UPDATE odps SET name=$3, code=$4, address=$5, latitude=$6, longitude=$7, odc_id=$8, cluster_id=$9
		WHERE tenant_id=$1 AND id=$2
	`, o.TenantID, o.ID, o.Name, o.Code, o.Address, o.Latitude, o.Longitude, o.ODCID, o.ClusterID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) GetODPByCode(ctx context.Context, tenantID xid.ID, code string) (*ODP, error) {
	row := s.Pool.QueryRow(ctx, `
		SELECT o.id, o.tenant_id, o.cluster_id, o.odc_id, o.name, o.code, o.address, o.latitude, o.longitude, o.port_count,
		       COALESCE((SELECT COUNT(*) FROM odp_ports p WHERE p.odp_id = o.id AND p.status = 'used'), 0),
		       COALESCE((SELECT COUNT(*) FROM odp_ports p WHERE p.odp_id = o.id AND p.status = 'available'), 0)
		FROM odps o WHERE o.tenant_id = $1 AND o.code = $2
	`, tenantID, code)
	var o ODP
	err := row.Scan(&o.ID, &o.TenantID, &o.ClusterID, &o.ODCID, &o.Name, &o.Code, &o.Address, &o.Latitude, &o.Longitude, &o.PortCount, &o.UsedPorts, &o.FreePorts)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &o, nil
}

func (s *Store) DeleteODP(ctx context.Context, tenantID, id xid.ID) error {
	tag, err := s.Pool.Exec(ctx, `DELETE FROM odps WHERE tenant_id=$1 AND id=$2`, tenantID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ListODPPorts(ctx context.Context, tenantID, odpID xid.ID) ([]ODPPort, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT p.id, p.tenant_id, p.odp_id, p.port_number, p.status, p.customer_id, p.subscription_id,
		       COALESCE(c.full_name, ''), COALESCE(sub.username, '')
		FROM odp_ports p
		LEFT JOIN customers c ON c.id = p.customer_id
		LEFT JOIN subscriptions sub ON sub.id = p.subscription_id
		WHERE p.tenant_id = $1 AND p.odp_id = $2
		ORDER BY p.port_number
	`, tenantID, odpID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []ODPPort
	for rows.Next() {
		var p ODPPort
		if err := rows.Scan(&p.ID, &p.TenantID, &p.ODPID, &p.PortNumber, &p.Status, &p.CustomerID, &p.SubscriptionID, &p.CustomerName, &p.SubscriptionUser); err != nil {
			return nil, err
		}
		list = append(list, p)
	}
	return list, rows.Err()
}

func (s *Store) AssignODPPort(ctx context.Context, tenantID xid.ID, odpID xid.ID, portNum int, customerID *xid.ID, subscriptionID *xid.ID) error {
	tag, err := s.Pool.Exec(ctx, `
		UPDATE odp_ports SET status='used', customer_id=$4, subscription_id=$5
		WHERE tenant_id=$1 AND odp_id=$2 AND port_number=$3 AND status='available'
	`, tenantID, odpID, portNum, customerID, subscriptionID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// AssignNextAvailableODPPort reserves the lowest available port on an ODP.
func (s *Store) AssignNextAvailableODPPort(ctx context.Context, tenantID, odpID, customerID, subscriptionID xid.ID) (*ODPPort, error) {
	row := s.Pool.QueryRow(ctx, `
		WITH next AS (
			SELECT id FROM odp_ports
			WHERE tenant_id = $1 AND odp_id = $2 AND status = 'available'
			ORDER BY port_number
			LIMIT 1
			FOR UPDATE SKIP LOCKED
		)
		UPDATE odp_ports p
		SET status = 'used', customer_id = $3, subscription_id = $4
		FROM next
		WHERE p.id = next.id
		RETURNING p.id, p.tenant_id, p.odp_id, p.port_number, p.status, p.customer_id, p.subscription_id
	`, tenantID, odpID, customerID, subscriptionID)
	var p ODPPort
	err := row.Scan(&p.ID, &p.TenantID, &p.ODPID, &p.PortNumber, &p.Status, &p.CustomerID, &p.SubscriptionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *Store) ReleaseODPPortBySubscription(ctx context.Context, tenantID, subscriptionID xid.ID) error {
	_, err := s.Pool.Exec(ctx, `
		UPDATE odp_ports
		SET status = 'available', customer_id = NULL, subscription_id = NULL
		WHERE tenant_id = $1 AND subscription_id = $2
	`, tenantID, subscriptionID)
	return err
}

func (s *Store) FindNearestAvailableODP(ctx context.Context, tenantID xid.ID, lat, lng float64) (*ODP, error) {
	row := s.Pool.QueryRow(ctx, `
		SELECT o.id, o.tenant_id, o.cluster_id, o.odc_id, o.name, o.code, o.address, o.latitude, o.longitude, o.port_count,
		       COALESCE((SELECT COUNT(*) FROM odp_ports p WHERE p.odp_id = o.id AND p.status = 'used'), 0),
		       COALESCE((SELECT COUNT(*) FROM odp_ports p WHERE p.odp_id = o.id AND p.status = 'available'), 0)
		FROM odps o
		WHERE o.tenant_id = $1
		  AND o.latitude IS NOT NULL AND o.longitude IS NOT NULL
		  AND EXISTS (SELECT 1 FROM odp_ports p WHERE p.odp_id = o.id AND p.status = 'available')
		ORDER BY ((o.latitude - $2)^2 + (o.longitude - $3)^2) ASC
		LIMIT 1
	`, tenantID, lat, lng)
	var o ODP
	err := row.Scan(&o.ID, &o.TenantID, &o.ClusterID, &o.ODCID, &o.Name, &o.Code, &o.Address, &o.Latitude, &o.Longitude, &o.PortCount, &o.UsedPorts, &o.FreePorts)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &o, nil
}

type JournalEntry struct {
	ID          xid.ID        `json:"id"`
	TenantID    xid.ID        `json:"tenant_id"`
	EntryDate   string        `json:"entry_date"`
	Description string        `json:"description"`
	Lines       []JournalLine `json:"lines"`
}

type JournalLine struct {
	AccountID xid.ID `json:"account_id"`
	Debit     int64  `json:"debit"`
	Credit    int64  `json:"credit"`
}

func (s *Store) PostJournalEntry(ctx context.Context, e *JournalEntry) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	err = tx.QueryRow(ctx, `
		INSERT INTO journal_entries (tenant_id, entry_date, description) VALUES ($1,$2,$3) RETURNING id
	`, e.TenantID, e.EntryDate, e.Description).Scan(&e.ID)
	if err != nil {
		return err
	}
	for _, line := range e.Lines {
		_, err = tx.Exec(ctx, `
			INSERT INTO journal_lines (tenant_id, entry_id, account_id, debit, credit) VALUES ($1,$2,$3,$4,$5)
		`, e.TenantID, e.ID, line.AccountID, line.Debit, line.Credit)
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) RecordPaymentJournal(ctx context.Context, tenantID xid.ID, amount int64, cashAccountID xid.ID, revenueAccountID xid.ID, ref string) error {
	return s.PostJournalEntry(ctx, &JournalEntry{
		TenantID:    tenantID,
		EntryDate:   time.Now().Format("2006-01-02"),
		Description: "Pembayaran " + ref,
		Lines: []JournalLine{
			{AccountID: cashAccountID, Debit: amount},
			{AccountID: revenueAccountID, Credit: amount},
		},
	})
}

func (s *Store) DashboardStats(ctx context.Context, tenantID xid.ID) (map[string]any, error) {
	stats := make(map[string]any)
	_ = s.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM customers WHERE tenant_id=$1 AND is_active=true`, tenantID).Scan(new(xid.ID))
	var activeCustomers, activeSubs, unpaidInvoices, monthlyRevenue int64
	_ = s.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM customers WHERE tenant_id=$1 AND is_active=true`, tenantID).Scan(&activeCustomers)
	_ = s.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM subscriptions WHERE tenant_id=$1 AND status='active'`, tenantID).Scan(&activeSubs)
	_ = s.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM invoices WHERE tenant_id=$1 AND status IN ('issued','partial','overdue')`, tenantID).Scan(&unpaidInvoices)
	_ = s.Pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(amount),0) FROM payments WHERE tenant_id=$1 AND status='paid'
		AND paid_at >= date_trunc('month', NOW())
	`, tenantID).Scan(&monthlyRevenue)
	stats["active_customers"] = activeCustomers
	stats["active_subscriptions"] = activeSubs
	stats["unpaid_invoices"] = unpaidInvoices
	stats["monthly_revenue"] = monthlyRevenue
	return stats, nil
}

func (s *Store) RevenueChart(ctx context.Context, tenantID xid.ID, months int) ([]map[string]any, error) {
	if months <= 0 {
		months = 6
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT to_char(date_trunc('month', paid_at), 'YYYY-MM') as month, COALESCE(SUM(amount),0)
		FROM payments
		WHERE tenant_id=$1 AND status='paid'
		  AND paid_at >= NOW() - ($2 * INTERVAL '1 month')
		GROUP BY 1 ORDER BY 1
	`, tenantID, months)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []map[string]any
	for rows.Next() {
		var month string
		var total int64
		if err := rows.Scan(&month, &total); err != nil {
			return nil, err
		}
		list = append(list, map[string]any{"month": month, "revenue": total})
	}
	return list, rows.Err()
}

func (s *Store) ListTenants(ctx context.Context) ([]Tenant, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT id, slug, name, email, phone, is_active, created_at FROM tenants ORDER BY id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []Tenant
	for rows.Next() {
		var t Tenant
		if err := rows.Scan(&t.ID, &t.Slug, &t.Name, &t.Email, &t.Phone, &t.IsActive, &t.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, t)
	}
	return list, rows.Err()
}
