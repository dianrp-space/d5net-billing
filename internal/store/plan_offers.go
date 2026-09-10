package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/dianrp/drp-billing/internal/xid"
	"github.com/jackc/pgx/v5"
)

// PlanClusterOffer is a plan offered in a cluster at a specific price.
type PlanClusterOffer struct {
	ID         xid.ID  `json:"id"`
	TenantID   xid.ID  `json:"tenant_id"`
	PlanID     xid.ID  `json:"plan_id"`
	ClusterID  xid.ID  `json:"cluster_id"`
	IPPoolID   *xid.ID `json:"ip_pool_id,omitempty"`
	IPPoolName string  `json:"ip_pool_name,omitempty"`
	Price      int64   `json:"price"`
	IsActive   bool    `json:"is_active"`
	// DueDay overrides plans.due_day for this cluster offer (nil = use plan).
	DueDay      *int   `json:"due_day,omitempty"`
	PlanName    string `json:"plan_name,omitempty"`
	PlanCode    string `json:"plan_code,omitempty"`
	ClusterName string `json:"cluster_name,omitempty"`
	ClusterCode string `json:"cluster_code,omitempty"`
	// Embedded plan specs for subscription UI / sync.
	ServiceType  string  `json:"service_type,omitempty"`
	DownloadMbps int     `json:"download_mbps,omitempty"`
	UploadMbps   int     `json:"upload_mbps,omitempty"`
	ProfileName  *string `json:"profile_name,omitempty"`
	BillingCycle string  `json:"billing_cycle,omitempty"`
	BasePrice    int64   `json:"base_price,omitempty"`
}

const planOfferSelect = `
		SELECT o.id, o.tenant_id, o.plan_id, o.cluster_id, o.ip_pool_id, COALESCE(ip.name,''),
		       o.price, o.is_active, o.due_day,
		       p.name, p.code, p.service_type, p.download_mbps, p.upload_mbps, p.profile_name, p.billing_cycle, p.price,
		       s.name, s.code
		FROM plan_cluster_offers o
		JOIN plans p ON p.id = o.plan_id
		JOIN sites s ON s.id = o.cluster_id
		LEFT JOIN ip_pools ip ON ip.id = o.ip_pool_id
`

func scanPlanOffer(row pgx.Row) (*PlanClusterOffer, error) {
	var o PlanClusterOffer
	err := row.Scan(
		&o.ID, &o.TenantID, &o.PlanID, &o.ClusterID, &o.IPPoolID, &o.IPPoolName,
		&o.Price, &o.IsActive, &o.DueDay,
		&o.PlanName, &o.PlanCode, &o.ServiceType, &o.DownloadMbps, &o.UploadMbps, &o.ProfileName, &o.BillingCycle, &o.BasePrice,
		&o.ClusterName, &o.ClusterCode,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &o, nil
}

func (s *Store) ListPlanOffers(ctx context.Context, tenantID xid.ID, clusterID *xid.ID, planID *xid.ID) ([]PlanClusterOffer, error) {
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return nil, err
	}
	where := "WHERE o.tenant_id = $1"
	args := []any{tenantID}
	n := 2
	if clusterID != nil {
		where += fmt.Sprintf(" AND o.cluster_id = $%d", n)
		args = append(args, *clusterID)
		n++
	}
	if planID != nil {
		where += fmt.Sprintf(" AND o.plan_id = $%d", n)
		args = append(args, *planID)
	}
	q := planOfferSelect + where + ` ORDER BY s.name, p.name`
	rows, err := s.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []PlanClusterOffer
	for rows.Next() {
		var o PlanClusterOffer
		if err := rows.Scan(
			&o.ID, &o.TenantID, &o.PlanID, &o.ClusterID, &o.IPPoolID, &o.IPPoolName,
			&o.Price, &o.IsActive, &o.DueDay,
			&o.PlanName, &o.PlanCode, &o.ServiceType, &o.DownloadMbps, &o.UploadMbps, &o.ProfileName, &o.BillingCycle, &o.BasePrice,
			&o.ClusterName, &o.ClusterCode,
		); err != nil {
			return nil, err
		}
		list = append(list, o)
	}
	return list, rows.Err()
}

func (s *Store) GetPlanOffer(ctx context.Context, tenantID, id xid.ID) (*PlanClusterOffer, error) {
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return nil, err
	}
	return scanPlanOffer(s.Pool.QueryRow(ctx, planOfferSelect+`
		WHERE o.tenant_id = $1 AND o.id = $2
	`, tenantID, id))
}

func (s *Store) GetPlanOfferByPlanCluster(ctx context.Context, tenantID, planID, clusterID xid.ID) (*PlanClusterOffer, error) {
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return nil, err
	}
	return scanPlanOffer(s.Pool.QueryRow(ctx, planOfferSelect+`
		WHERE o.tenant_id = $1 AND o.plan_id = $2 AND o.cluster_id = $3
	`, tenantID, planID, clusterID))
}

// ResolvePlanDueDay returns the effective invoice due day (1–28) for a plan in a
// cluster: active cluster offer override, then plan override, then tenant default.
func (s *Store) ResolvePlanDueDay(ctx context.Context, tenantID, planID xid.ID, clusterID *xid.ID, planDueDay *int) int {
	def := s.InvoiceDueDay(ctx, tenantID)
	if clusterID != nil && !xid.IsNil(*clusterID) {
		offer, err := s.GetPlanOfferByPlanCluster(ctx, tenantID, planID, *clusterID)
		if err == nil && offer != nil && offer.IsActive && offer.DueDay != nil {
			return ClampDueDay(*offer.DueDay, def)
		}
	}
	if planDueDay != nil {
		return ClampDueDay(*planDueDay, def)
	}
	return def
}

func (s *Store) UpsertPlanOffer(ctx context.Context, o *PlanClusterOffer) error {
	if err := s.SetTenantContext(ctx, o.TenantID); err != nil {
		return err
	}
	return s.Pool.QueryRow(ctx, `
		INSERT INTO plan_cluster_offers (tenant_id, plan_id, cluster_id, price, is_active, ip_pool_id, due_day)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		ON CONFLICT (plan_id, cluster_id) DO UPDATE
		SET price = EXCLUDED.price, is_active = EXCLUDED.is_active, ip_pool_id = EXCLUDED.ip_pool_id,
		    due_day = EXCLUDED.due_day, updated_at = NOW()
		RETURNING id
	`, o.TenantID, o.PlanID, o.ClusterID, o.Price, o.IsActive, o.IPPoolID, o.DueDay).Scan(&o.ID)
}

func (s *Store) UpdatePlanOffer(ctx context.Context, o *PlanClusterOffer) error {
	if err := s.SetTenantContext(ctx, o.TenantID); err != nil {
		return err
	}
	tag, err := s.Pool.Exec(ctx, `
		UPDATE plan_cluster_offers SET price=$3, is_active=$4, ip_pool_id=$5, due_day=$6, updated_at=NOW()
		WHERE tenant_id=$1 AND id=$2
	`, o.TenantID, o.ID, o.Price, o.IsActive, o.IPPoolID, o.DueDay)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeletePlanOffer(ctx context.Context, tenantID, id xid.ID) error {
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return err
	}
	tag, err := s.Pool.Exec(ctx, `DELETE FROM plan_cluster_offers WHERE tenant_id=$1 AND id=$2`, tenantID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ResolvePlanPrice returns cluster offer price when available, otherwise plan base price.
func (s *Store) ResolvePlanPrice(ctx context.Context, tenantID, planID xid.ID, clusterID *xid.ID) (int64, error) {
	plan, err := s.GetPlan(ctx, tenantID, planID)
	if err != nil {
		return 0, err
	}
	if clusterID == nil {
		return plan.Price, nil
	}
	offer, err := s.GetPlanOfferByPlanCluster(ctx, tenantID, planID, *clusterID)
	if errors.Is(err, ErrNotFound) {
		return plan.Price, nil
	}
	if err != nil {
		return 0, err
	}
	if !offer.IsActive {
		return plan.Price, nil
	}
	return offer.Price, nil
}

// ListRoutersByCluster returns routers linked to a cluster (site_id).
func (s *Store) ListRoutersByCluster(ctx context.Context, tenantID, clusterID xid.ID) ([]Router, error) {
	var list []Router
	err := s.withTenant(ctx, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id, tenant_id, site_id, name, address, port, use_tls, provisioner, username, password_enc,
			       is_active, last_seen_at, last_error
			FROM routers WHERE tenant_id = $1 AND site_id = $2 ORDER BY name
		`, tenantID, clusterID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var r Router
			if err := rows.Scan(&r.ID, &r.TenantID, &r.SiteID, &r.Name, &r.Address, &r.Port, &r.UseTLS, &r.Provisioner,
				&r.Username, &r.PasswordEnc, &r.IsActive, &r.LastSeenAt, &r.LastError); err != nil {
				return err
			}
			list = append(list, r)
		}
		return rows.Err()
	})
	return list, err
}
