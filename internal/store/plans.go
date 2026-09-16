package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dianrp-space/d5net-billing/internal/xid"

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
	// DueDay overrides the tenant invoice due day for this plan (nil = tenant default).
	DueDay        *int    `json:"due_day,omitempty"`
	TaxPercent    float64 `json:"tax_percent"`
	IsActive      bool    `json:"is_active"`
	PortalVisible bool    `json:"portal_visible"`
}

func (s *Store) ListPlans(ctx context.Context, tenantID xid.ID) ([]Plan, error) {
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return nil, err
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT id, tenant_id, name, code, service_type, price, billing_cycle, download_mbps, upload_mbps,
		       quota_gb, limit_uptime, shared_users, profile_name, isolir_profile, due_day, tax_percent, is_active, portal_visible
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
			&p.ProfileName, &p.IsolirProfile, &p.DueDay, &p.TaxPercent, &p.IsActive, &p.PortalVisible); err != nil {
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
		       quota_gb, limit_uptime, shared_users, profile_name, isolir_profile, due_day, tax_percent, is_active, portal_visible
		FROM plans WHERE tenant_id = $1 AND id = $2
	`, tenantID, id)
	var p Plan
	err := row.Scan(&p.ID, &p.TenantID, &p.Name, &p.Code, &p.ServiceType, &p.Price, &p.BillingCycle,
		&p.DownloadMbps, &p.UploadMbps, &p.QuotaGB, &p.LimitUptime, &p.SharedUsers,
		&p.ProfileName, &p.IsolirProfile, &p.DueDay, &p.TaxPercent, &p.IsActive, &p.PortalVisible)
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
		                   quota_gb, limit_uptime, shared_users, profile_name, isolir_profile, due_day, tax_percent, is_active, portal_visible)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17) RETURNING id
	`, p.TenantID, p.Name, p.Code, p.ServiceType, p.Price, p.BillingCycle, p.DownloadMbps, p.UploadMbps,
		p.QuotaGB, p.LimitUptime, p.SharedUsers, p.ProfileName, p.IsolirProfile, p.DueDay, p.TaxPercent, p.IsActive, p.PortalVisible).Scan(&p.ID)
}

func (s *Store) UpdatePlan(ctx context.Context, p *Plan) error {
	if err := s.SetTenantContext(ctx, p.TenantID); err != nil {
		return err
	}
	tag, err := s.Pool.Exec(ctx, `
		UPDATE plans SET name=$3, service_type=$4, price=$5, billing_cycle=$6, download_mbps=$7, upload_mbps=$8,
		                 quota_gb=$9, limit_uptime=$10, shared_users=$11, profile_name=$12, isolir_profile=$13,
		                 due_day=$14, tax_percent=$15, is_active=$16, portal_visible=$17, updated_at=NOW()
		WHERE tenant_id=$1 AND id=$2
	`, p.TenantID, p.ID, p.Name, p.ServiceType, p.Price, p.BillingCycle, p.DownloadMbps, p.UploadMbps,
		p.QuotaGB, p.LimitUptime, p.SharedUsers, p.ProfileName, p.IsolirProfile, p.DueDay, p.TaxPercent, p.IsActive, p.PortalVisible)
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

// PortalPlanOption is a customer-facing plan card (price already resolved for cluster).
type PortalPlanOption struct {
	ID            xid.ID `json:"id"`
	Name          string `json:"name"`
	Code          string `json:"code"`
	Price         int64  `json:"price"`
	OriginalPrice int64  `json:"original_price,omitempty"`
	DiscountLabel string `json:"discount_label,omitempty"`
	DownloadMbps  int    `json:"download_mbps"`
	UploadMbps    int    `json:"upload_mbps"`
	ServiceType   string `json:"service_type"`
	BillingCycle  string `json:"billing_cycle"`
}

// ListPortalPlans returns active, portal-visible plans the customer may pick.
// With a cluster, only active cluster offers are listed (offer price).
func (s *Store) ListPortalPlans(ctx context.Context, tenantID xid.ID, clusterID *xid.ID, serviceType string, excludePlanID xid.ID) ([]PortalPlanOption, error) {
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return nil, err
	}
	svc := strings.TrimSpace(strings.ToLower(serviceType))
	if clusterID == nil || xid.IsNil(*clusterID) {
		// Portal shows only plans offered in the customer's cluster; a customer
		// without a cluster has no offers to show.
		return []PortalPlanOption{}, nil
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT p.id, p.name, p.code, o.price, p.download_mbps, p.upload_mbps, p.service_type, p.billing_cycle
		FROM plans p
		JOIN plan_cluster_offers o ON o.plan_id = p.id AND o.cluster_id = $2 AND o.tenant_id = p.tenant_id AND o.is_active
		WHERE p.tenant_id = $1 AND p.is_active AND p.portal_visible
		  AND ($3 = '' OR p.service_type = $3)
		  AND p.id <> $4
		ORDER BY o.price, p.name
	`, tenantID, *clusterID, svc, excludePlanID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []PortalPlanOption
	for rows.Next() {
		var p PortalPlanOption
		if err := rows.Scan(&p.ID, &p.Name, &p.Code, &p.Price, &p.DownloadMbps, &p.UploadMbps, &p.ServiceType, &p.BillingCycle); err != nil {
			return nil, err
		}
		list = append(list, p)
	}
	if list == nil {
		list = []PortalPlanOption{}
	}
	return list, rows.Err()
}

// PublicPlan is a plan shown on the public catalog (no cluster context): the
// base plan price, or the cheapest active cluster-offer price when the base is 0.
type PublicPlan struct {
	ID           xid.ID `json:"id"`
	Name         string `json:"name"`
	Price        int64  `json:"price"`
	BillingCycle string `json:"billing_cycle"`
	ServiceType  string `json:"service_type"`
	DownloadMbps int    `json:"download_mbps"`
	UploadMbps   int    `json:"upload_mbps"`
	QuotaGB      *int   `json:"quota_gb,omitempty"`
}

// ListPublicPlans lists active, portal-visible plans for the public landing page.
func (s *Store) ListPublicPlans(ctx context.Context, tenantID xid.ID) ([]PublicPlan, error) {
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return nil, err
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT p.id, p.name, p.price, p.billing_cycle, p.service_type, p.download_mbps, p.upload_mbps, p.quota_gb,
		       COALESCE(MIN(o.price) FILTER (WHERE o.is_active), 0) AS min_offer
		FROM plans p
		LEFT JOIN plan_cluster_offers o ON o.plan_id = p.id AND o.tenant_id = p.tenant_id
		WHERE p.tenant_id = $1 AND p.is_active AND p.portal_visible
		GROUP BY p.id
		ORDER BY p.price, p.name
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []PublicPlan
	for rows.Next() {
		var p PublicPlan
		var minOffer int64
		if err := rows.Scan(&p.ID, &p.Name, &p.Price, &p.BillingCycle, &p.ServiceType,
			&p.DownloadMbps, &p.UploadMbps, &p.QuotaGB, &minOffer); err != nil {
			return nil, err
		}
		if p.Price <= 0 && minOffer > 0 {
			p.Price = minOffer
		}
		list = append(list, p)
	}
	if list == nil {
		list = []PublicPlan{}
	}
	return list, rows.Err()
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
	// IsFree marks a subscription on a 0-price plan/offer (no billing/auto-isolir).
	IsFree bool `json:"is_free,omitempty"`
}

func (s *Store) ListSubscriptions(ctx context.Context, tenantID xid.ID, status string, customerID *xid.ID, limit, offset int) ([]Subscription, int64, error) {
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return nil, 0, err
	}
	where := "WHERE s.tenant_id = $1"
	args := []any{tenantID}
	if status != "" {
		args = append(args, status)
		where += fmt.Sprintf(" AND s.status = $%d", len(args))
	}
	if customerID != nil {
		args = append(args, *customerID)
		where += fmt.Sprintf(" AND s.customer_id = $%d", len(args))
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
		SELECT s.id, s.tenant_id, s.customer_id, s.plan_id, s.router_id, s.username, s.password, s.service_type, s.status,
		       s.started_at, s.expires_at, s.next_bill_at, s.suspended_at, c.full_name, c.customer_code, p.name,
		       op.odp_id, COALESCE(o.code, ''), COALESCE(o.name, ''), op.port_number,
		       COALESCE((
		           SELECT po.price FROM plan_cluster_offers po
		           WHERE po.tenant_id = s.tenant_id AND po.plan_id = s.plan_id
		             AND po.cluster_id = c.cluster_id AND po.is_active = true
		           LIMIT 1
		       ), p.price) = 0 AS is_free
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
		if err := rows.Scan(&sub.ID, &sub.TenantID, &sub.CustomerID, &sub.PlanID, &sub.RouterID, &sub.Username, &sub.Password,
			&sub.ServiceType, &sub.Status, &sub.StartedAt, &sub.ExpiresAt, &sub.NextBillAt, &sub.SuspendedAt,
			&sub.CustomerName, &sub.CustomerCode, &sub.PlanName, &odpID, &sub.ODPCode, &sub.ODPName, &portNum, &sub.IsFree); err != nil {
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

// PlanNameForSubscription returns the plan name for a subscription id, or "".
func (s *Store) PlanNameForSubscription(ctx context.Context, tenantID xid.ID, subID *xid.ID) string {
	if subID == nil || xid.IsNil(*subID) {
		return ""
	}
	var name string
	if err := s.Pool.QueryRow(ctx, `
		SELECT COALESCE(p.name, '') FROM subscriptions s
		JOIN plans p ON p.id = s.plan_id
		WHERE s.tenant_id = $1 AND s.id = $2
	`, tenantID, *subID).Scan(&name); err != nil {
		return ""
	}
	return name
}

// IsFreeSubscription reports whether a subscription is billed at base price 0
// (free plan, or a 0-price cluster offer). Free subscriptions skip billing,
// auto-isolir, auto-resume, and dunning; manual admin actions still apply.
func (s *Store) IsFreeSubscription(ctx context.Context, tenantID, subscriptionID xid.ID) (bool, error) {
	var free bool
	err := s.Pool.QueryRow(ctx, `
		SELECT COALESCE((
			SELECT o.price FROM plan_cluster_offers o
			WHERE o.tenant_id = s.tenant_id AND o.plan_id = s.plan_id
			  AND o.cluster_id = c.cluster_id AND o.is_active = true
			LIMIT 1
		), p.price) = 0
		FROM subscriptions s
		JOIN customers c ON c.id = s.customer_id
		JOIN plans p ON p.id = s.plan_id
		WHERE s.tenant_id = $1 AND s.id = $2
	`, tenantID, subscriptionID).Scan(&free)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrNotFound
	}
	return free, err
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
			suspended_at = CASE
				WHEN $3 = 'suspended' THEN COALESCE(suspended_at, NOW())
				WHEN $3 IN ('active', 'overdue') THEN suspended_at
				ELSE NULL
			END
		WHERE tenant_id=$1 AND id=$2
	`, tenantID, id, status)
	return err
}

// ClearSubscriptionSuspendedAt marks router resume as done (billing already active).
func (s *Store) ClearSubscriptionSuspendedAt(ctx context.Context, tenantID, id xid.ID) error {
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return err
	}
	_, err := s.Pool.Exec(ctx, `
		UPDATE subscriptions SET suspended_at = NULL, updated_at = NOW()
		WHERE tenant_id=$1 AND id=$2
	`, tenantID, id)
	return err
}

// ListSubscriptionsNeedingResume is paid (no past-due unpaid) but still flagged isolir
// because Resume ke router belum sukses (router down, timeout, dll).
func (s *Store) ListSubscriptionsNeedingResume(ctx context.Context, tenantID xid.ID) ([]xid.ID, error) {
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return nil, err
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT s.id
		FROM subscriptions s
		WHERE s.tenant_id = $1
		  AND s.router_id IS NOT NULL
		  AND s.status IN ('active','overdue','suspended')
		  AND s.suspended_at IS NOT NULL
		  AND NOT EXISTS (
		    SELECT 1 FROM invoices i
		    WHERE i.tenant_id = s.tenant_id
		      AND i.isolir = TRUE
		      AND (i.subscription_id = s.id OR i.isolir_subscription_id = s.id)
		      AND i.deleted_at IS NULL
		      AND i.status IN ('issued','partial','overdue')
		      AND i.total_amount > i.paid_amount
		      AND (i.due_date::timestamptz + make_interval(days => $2)) < NOW()
		  )
	`, tenantID, s.IsolirGraceDays(ctx, tenantID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []xid.ID
	for rows.Next() {
		var id xid.ID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// ListProvisionableSubscriptionsByRouter lists live services on a router for profile sync.
func (s *Store) ListProvisionableSubscriptionsByRouter(ctx context.Context, tenantID, routerID xid.ID) ([]Subscription, error) {
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return nil, err
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT s.id, s.tenant_id, s.customer_id, s.plan_id, s.router_id, s.username, s.service_type, s.status,
		       s.started_at, s.expires_at, s.next_bill_at, s.suspended_at, c.full_name, c.customer_code, p.name
		FROM subscriptions s
		JOIN customers c ON c.id = s.customer_id
		JOIN plans p ON p.id = s.plan_id
		WHERE s.tenant_id = $1 AND s.router_id = $2
		  AND s.status IN ('active','suspended','overdue')
	`, tenantID, routerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []Subscription
	for rows.Next() {
		var sub Subscription
		if err := rows.Scan(&sub.ID, &sub.TenantID, &sub.CustomerID, &sub.PlanID, &sub.RouterID, &sub.Username,
			&sub.ServiceType, &sub.Status, &sub.StartedAt, &sub.ExpiresAt, &sub.NextBillAt, &sub.SuspendedAt,
			&sub.CustomerName, &sub.CustomerCode, &sub.PlanName); err != nil {
			return nil, err
		}
		list = append(list, sub)
	}
	return list, rows.Err()
}

func (s *Store) CancelCustomerSubscriptions(ctx context.Context, tenantID, customerID xid.ID) error {
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return err
	}
	_, err := s.Pool.Exec(ctx, `
		UPDATE subscriptions
		SET status = 'cancelled', updated_at = NOW(), suspended_at = NULL
		WHERE tenant_id = $1 AND customer_id = $2 AND status NOT IN ('cancelled', 'canceled')
	`, tenantID, customerID)
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
	// Isolir when an unpaid invoice is past due_date + tenant isolir_grace_days.
	_ = graceDays
	grace := s.IsolirGraceDays(ctx, tenantID)
	rows, err := s.Pool.Query(ctx, `
		SELECT s.id, s.tenant_id, s.customer_id, s.plan_id, s.router_id, s.username, s.service_type, s.status,
		       s.started_at, s.expires_at, s.next_bill_at, s.suspended_at, '', ''
		FROM subscriptions s
		WHERE s.tenant_id = $1
		  AND s.status IN ('active','overdue','suspended')
		  AND EXISTS (
		    SELECT 1 FROM invoices i
		    WHERE i.tenant_id = s.tenant_id
		      AND i.isolir = TRUE
		      AND (i.subscription_id = s.id OR i.isolir_subscription_id = s.id)
		      AND i.deleted_at IS NULL
		      AND i.status IN ('issued','partial','overdue')
		      AND i.total_amount > i.paid_amount
		      AND (i.due_date::timestamptz + make_interval(days => $2)) < NOW()
		  )
	`, tenantID, grace)
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
