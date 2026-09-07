package store

import (
	"context"
	"errors"
	"github.com/dianrp/drp-billing/internal/xid"
	"time"

	"github.com/jackc/pgx/v5"
)

type Plan struct {
	ID            xid.ID  `json:"id"`
	TenantID      xid.ID  `json:"tenant_id"`
	Name          string  `json:"name"`
	Code          string  `json:"code"`
	ServiceType   string  `json:"service_type"`
	Price         int64   `json:"price"`
	BillingCycle  string  `json:"billing_cycle"`
	DownloadMbps  int     `json:"download_mbps"`
	UploadMbps    int     `json:"upload_mbps"`
	QuotaGB       *int    `json:"quota_gb,omitempty"`
	LimitUptime   *string `json:"limit_uptime,omitempty"`
	SharedUsers   *int    `json:"shared_users,omitempty"`
	ProfileName   *string `json:"profile_name,omitempty"`
	IsolirProfile *string `json:"isolir_profile,omitempty"`
	GraceDays     int     `json:"grace_days"`
	TaxPercent    float64 `json:"tax_percent"`
	IsActive      bool    `json:"is_active"`
}

func (s *Store) ListPlans(ctx context.Context, tenantID xid.ID) ([]Plan, error) {
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return nil, err
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT id, tenant_id, name, code, service_type, price, billing_cycle, download_mbps, upload_mbps,
		       quota_gb, limit_uptime, shared_users, profile_name, isolir_profile, grace_days, tax_percent, is_active
		FROM plans WHERE tenant_id = $1 ORDER BY name
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []Plan
	for rows.Next() {
		var p Plan
		if err := rows.Scan(&p.ID, &p.TenantID, &p.Name, &p.Code, &p.ServiceType, &p.Price, &p.BillingCycle,
			&p.DownloadMbps, &p.UploadMbps, &p.QuotaGB, &p.LimitUptime, &p.SharedUsers,
			&p.ProfileName, &p.IsolirProfile, &p.GraceDays, &p.TaxPercent, &p.IsActive); err != nil {
			return nil, err
		}
		list = append(list, p)
	}
	return list, rows.Err()
}

func (s *Store) GetPlan(ctx context.Context, tenantID xid.ID, id xid.ID) (*Plan, error) {
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return nil, err
	}
	row := s.Pool.QueryRow(ctx, `
		SELECT id, tenant_id, name, code, service_type, price, billing_cycle, download_mbps, upload_mbps,
		       quota_gb, limit_uptime, shared_users, profile_name, isolir_profile, grace_days, tax_percent, is_active
		FROM plans WHERE tenant_id = $1 AND id = $2
	`, tenantID, id)
	var p Plan
	err := row.Scan(&p.ID, &p.TenantID, &p.Name, &p.Code, &p.ServiceType, &p.Price, &p.BillingCycle,
		&p.DownloadMbps, &p.UploadMbps, &p.QuotaGB, &p.LimitUptime, &p.SharedUsers,
		&p.ProfileName, &p.IsolirProfile, &p.GraceDays, &p.TaxPercent, &p.IsActive)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &p, err
}

func (s *Store) CreatePlan(ctx context.Context, p *Plan) error {
	if err := s.SetTenantContext(ctx, p.TenantID); err != nil {
		return err
	}
	return s.Pool.QueryRow(ctx, `
		INSERT INTO plans (tenant_id, name, code, service_type, price, billing_cycle, download_mbps, upload_mbps,
		                   quota_gb, limit_uptime, shared_users, profile_name, isolir_profile, grace_days, tax_percent, is_active)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16) RETURNING id
	`, p.TenantID, p.Name, p.Code, p.ServiceType, p.Price, p.BillingCycle, p.DownloadMbps, p.UploadMbps,
		p.QuotaGB, p.LimitUptime, p.SharedUsers, p.ProfileName, p.IsolirProfile, p.GraceDays, p.TaxPercent, p.IsActive).Scan(&p.ID)
}

func (s *Store) UpdatePlan(ctx context.Context, p *Plan) error {
	if err := s.SetTenantContext(ctx, p.TenantID); err != nil {
		return err
	}
	tag, err := s.Pool.Exec(ctx, `
		UPDATE plans SET name=$3, service_type=$4, price=$5, billing_cycle=$6, download_mbps=$7, upload_mbps=$8,
		                 quota_gb=$9, limit_uptime=$10, shared_users=$11, profile_name=$12, isolir_profile=$13,
		                 grace_days=$14, tax_percent=$15, is_active=$16, updated_at=NOW()
		WHERE tenant_id=$1 AND id=$2
	`, p.TenantID, p.ID, p.Name, p.ServiceType, p.Price, p.BillingCycle, p.DownloadMbps, p.UploadMbps,
		p.QuotaGB, p.LimitUptime, p.SharedUsers, p.ProfileName, p.IsolirProfile, p.GraceDays, p.TaxPercent, p.IsActive)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeletePlan(ctx context.Context, tenantID xid.ID, id xid.ID) error {
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return err
	}
	tag, err := s.Pool.Exec(ctx, `DELETE FROM plans WHERE tenant_id=$1 AND id=$2`, tenantID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

type Subscription struct {
	ID           xid.ID     `json:"id"`
	TenantID     xid.ID     `json:"tenant_id"`
	CustomerID   xid.ID     `json:"customer_id"`
	PlanID       xid.ID     `json:"plan_id"`
	RouterID     *xid.ID    `json:"router_id,omitempty"`
	Username     string     `json:"username"`
	Password     *string    `json:"password,omitempty"`
	ServiceType  string     `json:"service_type"`
	Status       string     `json:"status"`
	StartedAt    *time.Time `json:"started_at,omitempty"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
	NextBillAt   *time.Time `json:"next_bill_at,omitempty"`
	SuspendedAt  *time.Time `json:"suspended_at,omitempty"`
	CustomerName string     `json:"customer_name,omitempty"`
	CustomerCode string     `json:"customer_code,omitempty"`
	PlanName     string     `json:"plan_name,omitempty"`
	ODPID        *xid.ID    `json:"odp_id,omitempty"`
	ODPCode      string     `json:"odp_code,omitempty"`
	ODPName      string     `json:"odp_name,omitempty"`
	PortNumber   *int       `json:"port_number,omitempty"`
}

func (s *Store) ListSubscriptions(ctx context.Context, tenantID xid.ID, status string, limit, offset int) ([]Subscription, int64, error) {
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return nil, 0, err
	}
	where := "WHERE s.tenant_id = $1"
	args := []any{tenantID}
	if status != "" {
		where += " AND s.status = $2"
		args = append(args, status)
	}
	var total int64
	if err := s.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM subscriptions s "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	if limit <= 0 {
		limit = 20
	}
	args = append(args, limit, offset)
	q := `
		SELECT s.id, s.tenant_id, s.customer_id, s.plan_id, s.router_id, s.username, s.service_type, s.status,
		       s.started_at, s.expires_at, s.next_bill_at, s.suspended_at, c.full_name, c.customer_code, p.name,
		       op.odp_id, COALESCE(o.code, ''), COALESCE(o.name, ''), op.port_number
		FROM subscriptions s
		JOIN customers c ON c.id = s.customer_id
		JOIN plans p ON p.id = s.plan_id
		LEFT JOIN odp_ports op ON op.subscription_id = s.id
		LEFT JOIN odps o ON o.id = op.odp_id
		` + where + ` ORDER BY s.id DESC LIMIT $` + itoa(len(args)-1) + ` OFFSET $` + itoa(len(args))
	rows, err := s.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var list []Subscription
	for rows.Next() {
		var sub Subscription
		var odpID *xid.ID
		var portNum *int
		if err := rows.Scan(&sub.ID, &sub.TenantID, &sub.CustomerID, &sub.PlanID, &sub.RouterID, &sub.Username,
			&sub.ServiceType, &sub.Status, &sub.StartedAt, &sub.ExpiresAt, &sub.NextBillAt, &sub.SuspendedAt,
			&sub.CustomerName, &sub.CustomerCode, &sub.PlanName, &odpID, &sub.ODPCode, &sub.ODPName, &portNum); err != nil {
			return nil, 0, err
		}
		sub.ODPID = odpID
		sub.PortNumber = portNum
		list = append(list, sub)
	}
	return list, total, rows.Err()
}

func (s *Store) GetSubscription(ctx context.Context, tenantID xid.ID, id xid.ID) (*Subscription, error) {
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return nil, err
	}
	row := s.Pool.QueryRow(ctx, `
		SELECT s.id, s.tenant_id, s.customer_id, s.plan_id, s.router_id, s.username, s.password, s.service_type, s.status,
		       s.started_at, s.expires_at, s.next_bill_at, s.suspended_at, c.full_name, c.customer_code, p.name,
		       op.odp_id, COALESCE(o.code, ''), COALESCE(o.name, ''), op.port_number
		FROM subscriptions s
		JOIN customers c ON c.id = s.customer_id
		JOIN plans p ON p.id = s.plan_id
		LEFT JOIN odp_ports op ON op.subscription_id = s.id
		LEFT JOIN odps o ON o.id = op.odp_id
		WHERE s.tenant_id = $1 AND s.id = $2
	`, tenantID, id)
	var sub Subscription
	var odpID *xid.ID
	var portNum *int
	err := row.Scan(&sub.ID, &sub.TenantID, &sub.CustomerID, &sub.PlanID, &sub.RouterID, &sub.Username, &sub.Password,
		&sub.ServiceType, &sub.Status, &sub.StartedAt, &sub.ExpiresAt, &sub.NextBillAt, &sub.SuspendedAt,
		&sub.CustomerName, &sub.CustomerCode, &sub.PlanName, &odpID, &sub.ODPCode, &sub.ODPName, &portNum)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	sub.ODPID = odpID
	sub.PortNumber = portNum
	return &sub, nil
}

// FindProvisionableSubscriptionByCustomer returns the best subscription to sync for a customer
// (active preferred, then suspended/overdue), optionally filtered by router.
func (s *Store) FindProvisionableSubscriptionByCustomer(ctx context.Context, tenantID, customerID xid.ID, routerID *xid.ID) (*Subscription, error) {
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return nil, err
	}
	args := []any{tenantID, customerID}
	routerFilter := ""
	if routerID != nil {
		routerFilter = " AND s.router_id = $3"
		args = append(args, *routerID)
	}
	row := s.Pool.QueryRow(ctx, `
		SELECT s.id, s.tenant_id, s.customer_id, s.plan_id, s.router_id, s.username, s.password, s.service_type, s.status,
		       s.started_at, s.expires_at, s.next_bill_at, s.suspended_at, c.full_name, c.customer_code, p.name,
		       op.odp_id, COALESCE(o.code, ''), COALESCE(o.name, ''), op.port_number
		FROM subscriptions s
		JOIN customers c ON c.id = s.customer_id
		JOIN plans p ON p.id = s.plan_id
		LEFT JOIN odp_ports op ON op.subscription_id = s.id
		LEFT JOIN odps o ON o.id = op.odp_id
		WHERE s.tenant_id = $1 AND s.customer_id = $2
		  AND s.router_id IS NOT NULL
		  AND s.status IN ('active','suspended','overdue')
		`+routerFilter+`
		ORDER BY CASE s.status WHEN 'active' THEN 0 WHEN 'overdue' THEN 1 ELSE 2 END, s.id DESC
		LIMIT 1
	`, args...)
	var sub Subscription
	var odpID *xid.ID
	var portNum *int
	err := row.Scan(&sub.ID, &sub.TenantID, &sub.CustomerID, &sub.PlanID, &sub.RouterID, &sub.Username, &sub.Password,
		&sub.ServiceType, &sub.Status, &sub.StartedAt, &sub.ExpiresAt, &sub.NextBillAt, &sub.SuspendedAt,
		&sub.CustomerName, &sub.CustomerCode, &sub.PlanName, &odpID, &sub.ODPCode, &sub.ODPName, &portNum)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	sub.ODPID = odpID
	sub.PortNumber = portNum
	return &sub, nil
}

func (s *Store) CreateSubscription(ctx context.Context, sub *Subscription) error {
	if err := s.SetTenantContext(ctx, sub.TenantID); err != nil {
		return err
	}
	return s.Pool.QueryRow(ctx, `
		INSERT INTO subscriptions (tenant_id, customer_id, plan_id, router_id, username, password, service_type, status, started_at, expires_at, next_bill_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING id
	`, sub.TenantID, sub.CustomerID, sub.PlanID, sub.RouterID, sub.Username, sub.Password, sub.ServiceType, sub.Status,
		sub.StartedAt, sub.ExpiresAt, sub.NextBillAt).Scan(&sub.ID)
}

func (s *Store) UpdateSubscription(ctx context.Context, tenantID xid.ID, sub *Subscription, updatePassword bool) error {
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return err
	}
	if updatePassword {
		_, err := s.Pool.Exec(ctx, `
			UPDATE subscriptions SET
				plan_id=$3, router_id=$4, username=$5, password=$6, service_type=$7, updated_at=NOW()
			WHERE tenant_id=$1 AND id=$2
		`, tenantID, sub.ID, sub.PlanID, sub.RouterID, sub.Username, sub.Password, sub.ServiceType)
		return err
	}
	_, err := s.Pool.Exec(ctx, `
		UPDATE subscriptions SET
			plan_id=$3, router_id=$4, username=$5, service_type=$6, updated_at=NOW()
		WHERE tenant_id=$1 AND id=$2
	`, tenantID, sub.ID, sub.PlanID, sub.RouterID, sub.Username, sub.ServiceType)
	return err
}

func (s *Store) DeleteSubscription(ctx context.Context, tenantID xid.ID, id xid.ID) error {
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return err
	}
	_ = s.ReleaseODPPortBySubscription(ctx, tenantID, id)
	tag, err := s.Pool.Exec(ctx, `DELETE FROM subscriptions WHERE tenant_id=$1 AND id=$2`, tenantID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) UpdateSubscriptionStatus(ctx context.Context, tenantID xid.ID, id xid.ID, status string) error {
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return err
	}
	_, err := s.Pool.Exec(ctx, `
		UPDATE subscriptions SET status=$3, updated_at=NOW(),
			suspended_at = CASE WHEN $3 = 'suspended' THEN NOW() ELSE NULL END
		WHERE tenant_id=$1 AND id=$2
	`, tenantID, id, status)
	return err
}

func (s *Store) ListDueSubscriptions(ctx context.Context, tenantID xid.ID) ([]Subscription, error) {
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return nil, err
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT s.id, s.tenant_id, s.customer_id, s.plan_id, s.router_id, s.username, s.service_type, s.status,
		       s.started_at, s.expires_at, s.next_bill_at, s.suspended_at, '', ''
		FROM subscriptions s
		WHERE s.tenant_id = $1 AND s.status = 'active' AND s.next_bill_at <= NOW()
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []Subscription
	for rows.Next() {
		var sub Subscription
		if err := rows.Scan(&sub.ID, &sub.TenantID, &sub.CustomerID, &sub.PlanID, &sub.RouterID, &sub.Username,
			&sub.ServiceType, &sub.Status, &sub.StartedAt, &sub.ExpiresAt, &sub.NextBillAt, &sub.SuspendedAt,
			&sub.CustomerName, &sub.PlanName); err != nil {
			return nil, err
		}
		list = append(list, sub)
	}
	return list, rows.Err()
}

func (s *Store) ListOverdueSubscriptions(ctx context.Context, tenantID xid.ID, graceDays int) ([]Subscription, error) {
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return nil, err
	}
	// Isolir when an unpaid invoice is past due_date + plan.grace_days.
	// (Do not use next_bill_at — billing advances that clock when creating invoices.)
	_ = graceDays // kept for API compat; grace comes from joined plan
	rows, err := s.Pool.Query(ctx, `
		SELECT s.id, s.tenant_id, s.customer_id, s.plan_id, s.router_id, s.username, s.service_type, s.status,
		       s.started_at, s.expires_at, s.next_bill_at, s.suspended_at, '', ''
		FROM subscriptions s
		JOIN plans p ON p.id = s.plan_id
		WHERE s.tenant_id = $1
		  AND s.status IN ('active','overdue')
		  AND EXISTS (
		    SELECT 1 FROM invoices i
		    WHERE i.tenant_id = s.tenant_id
		      AND i.subscription_id = s.id
		      AND i.status IN ('issued','partial','overdue')
		      AND i.total_amount > i.paid_amount
		      AND (i.due_date::timestamptz + (COALESCE(p.grace_days, 0) || ' days')::interval) < NOW()
		  )
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []Subscription
	for rows.Next() {
		var sub Subscription
		if err := rows.Scan(&sub.ID, &sub.TenantID, &sub.CustomerID, &sub.PlanID, &sub.RouterID, &sub.Username,
			&sub.ServiceType, &sub.Status, &sub.StartedAt, &sub.ExpiresAt, &sub.NextBillAt, &sub.SuspendedAt,
			&sub.CustomerName, &sub.PlanName); err != nil {
			return nil, err
		}
		list = append(list, sub)
	}
	return list, rows.Err()
}

func itoa(i int) string {
	return fmtInt(int64(i))
}

func fmtInt(n int64) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
