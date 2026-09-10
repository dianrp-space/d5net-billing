package store

import (
	"context"
	"errors"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/dianrp/drp-billing/internal/xid"
	"github.com/jackc/pgx/v5"
)

const (
	DiscountKindPercent  = "percent"
	DiscountKindAmount   = "amount"
	DiscountAudienceAll  = "all"
	DiscountAudiencePick = "selected"
)

type PlanDiscount struct {
	ID            xid.ID    `json:"id"`
	TenantID      xid.ID    `json:"tenant_id"`
	Name          string    `json:"name"`
	PlanID        *xid.ID   `json:"plan_id,omitempty"`
	PlanName      string    `json:"plan_name,omitempty"`
	Kind          string    `json:"kind"`
	Value         int64     `json:"value"`
	StartsOn      string    `json:"starts_on"`
	EndsOn        string    `json:"ends_on"`
	Audience      string    `json:"audience"`
	IsActive      bool      `json:"is_active"`
	CustomerIDs   []xid.ID  `json:"customer_ids,omitempty"`
	CustomerCount int       `json:"customer_count"`
	CreatedAt     time.Time `json:"created_at"`
}

func NormalizePlanDiscount(d PlanDiscount) PlanDiscount {
	d.Name = strings.TrimSpace(d.Name)
	d.Kind = strings.ToLower(strings.TrimSpace(d.Kind))
	if d.Kind != DiscountKindAmount {
		d.Kind = DiscountKindPercent
	}
	d.Audience = strings.ToLower(strings.TrimSpace(d.Audience))
	if d.Audience != DiscountAudiencePick {
		d.Audience = DiscountAudienceAll
	}
	d.StartsOn = strings.TrimSpace(d.StartsOn)
	d.EndsOn = strings.TrimSpace(d.EndsOn)
	if d.Kind == DiscountKindPercent && d.Value > 100 {
		d.Value = 100
	}
	if d.Value < 0 {
		d.Value = 0
	}
	if d.Audience == DiscountAudienceAll {
		d.CustomerIDs = nil
	}
	return d
}

func ApplyPlanDiscount(base int64, d *PlanDiscount) int64 {
	if d == nil || base <= 0 || d.Value <= 0 {
		return base
	}
	off := d.Value
	if d.Kind == DiscountKindPercent {
		off = int64(math.Round(float64(base) * float64(d.Value) / 100))
	}
	if off > base {
		off = base
	}
	if off < 0 {
		off = 0
	}
	return base - off
}

func (d *PlanDiscount) Label() string {
	if d == nil {
		return ""
	}
	if d.Kind == DiscountKindPercent {
		return d.Name + " −" + strconv.FormatInt(d.Value, 10) + "%"
	}
	return d.Name
}

func discountSpecificity(d *PlanDiscount) int {
	if d == nil {
		return 0
	}
	n := 0
	if d.PlanID != nil && !xid.IsNil(*d.PlanID) {
		n += 2
	}
	if d.Audience == DiscountAudiencePick {
		n++
	}
	return n
}

func (s *Store) ListPlanDiscounts(ctx context.Context, tenantID xid.ID) ([]PlanDiscount, error) {
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return nil, err
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT d.id, d.tenant_id, d.name, d.plan_id, COALESCE(p.name, ''),
		       d.kind, d.value, d.starts_on::text, d.ends_on::text, d.audience, d.is_active,
		       (SELECT COUNT(*) FROM plan_discount_customers c WHERE c.discount_id = d.id),
		       d.created_at
		FROM plan_discounts d
		LEFT JOIN plans p ON p.id = d.plan_id
		WHERE d.tenant_id = $1
		ORDER BY d.starts_on DESC, d.created_at DESC
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []PlanDiscount
	for rows.Next() {
		var d PlanDiscount
		if err := rows.Scan(&d.ID, &d.TenantID, &d.Name, &d.PlanID, &d.PlanName,
			&d.Kind, &d.Value, &d.StartsOn, &d.EndsOn, &d.Audience, &d.IsActive,
			&d.CustomerCount, &d.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, d)
	}
	if list == nil {
		list = []PlanDiscount{}
	}
	return list, rows.Err()
}

func (s *Store) GetPlanDiscount(ctx context.Context, tenantID, id xid.ID) (*PlanDiscount, error) {
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return nil, err
	}
	row := s.Pool.QueryRow(ctx, `
		SELECT d.id, d.tenant_id, d.name, d.plan_id, COALESCE(p.name, ''),
		       d.kind, d.value, d.starts_on::text, d.ends_on::text, d.audience, d.is_active,
		       (SELECT COUNT(*) FROM plan_discount_customers c WHERE c.discount_id = d.id),
		       d.created_at
		FROM plan_discounts d
		LEFT JOIN plans p ON p.id = d.plan_id
		WHERE d.tenant_id = $1 AND d.id = $2
	`, tenantID, id)
	var d PlanDiscount
	err := row.Scan(&d.ID, &d.TenantID, &d.Name, &d.PlanID, &d.PlanName,
		&d.Kind, &d.Value, &d.StartsOn, &d.EndsOn, &d.Audience, &d.IsActive,
		&d.CustomerCount, &d.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	ids, err := s.listPlanDiscountCustomers(ctx, d.ID)
	if err != nil {
		return nil, err
	}
	d.CustomerIDs = ids
	return &d, nil
}

func (s *Store) listPlanDiscountCustomers(ctx context.Context, discountID xid.ID) ([]xid.ID, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT customer_id FROM plan_discount_customers WHERE discount_id = $1
	`, discountID)
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
	if ids == nil {
		ids = []xid.ID{}
	}
	return ids, rows.Err()
}

func (s *Store) CreatePlanDiscount(ctx context.Context, d *PlanDiscount) (*PlanDiscount, error) {
	if err := s.SetTenantContext(ctx, d.TenantID); err != nil {
		return nil, err
	}
	d.ID = xid.New()
	_, err := s.Pool.Exec(ctx, `
		INSERT INTO plan_discounts (id, tenant_id, name, plan_id, kind, value, starts_on, ends_on, audience, is_active)
		VALUES ($1,$2,$3,$4,$5,$6,$7::date,$8::date,$9,$10)
	`, d.ID, d.TenantID, d.Name, d.PlanID, d.Kind, d.Value, d.StartsOn, d.EndsOn, d.Audience, d.IsActive)
	if err != nil {
		return nil, err
	}
	if err := s.replacePlanDiscountCustomers(ctx, d.ID, d.CustomerIDs); err != nil {
		return nil, err
	}
	return s.GetPlanDiscount(ctx, d.TenantID, d.ID)
}

func (s *Store) UpdatePlanDiscount(ctx context.Context, d *PlanDiscount) (*PlanDiscount, error) {
	if err := s.SetTenantContext(ctx, d.TenantID); err != nil {
		return nil, err
	}
	tag, err := s.Pool.Exec(ctx, `
		UPDATE plan_discounts
		SET name=$3, plan_id=$4, kind=$5, value=$6, starts_on=$7::date, ends_on=$8::date,
		    audience=$9, is_active=$10, updated_at=NOW()
		WHERE tenant_id=$1 AND id=$2
	`, d.TenantID, d.ID, d.Name, d.PlanID, d.Kind, d.Value, d.StartsOn, d.EndsOn, d.Audience, d.IsActive)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrNotFound
	}
	if err := s.replacePlanDiscountCustomers(ctx, d.ID, d.CustomerIDs); err != nil {
		return nil, err
	}
	return s.GetPlanDiscount(ctx, d.TenantID, d.ID)
}

func (s *Store) DeletePlanDiscount(ctx context.Context, tenantID, id xid.ID) error {
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return err
	}
	tag, err := s.Pool.Exec(ctx, `DELETE FROM plan_discounts WHERE tenant_id=$1 AND id=$2`, tenantID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) replacePlanDiscountCustomers(ctx context.Context, discountID xid.ID, ids []xid.ID) error {
	if _, err := s.Pool.Exec(ctx, `DELETE FROM plan_discount_customers WHERE discount_id=$1`, discountID); err != nil {
		return err
	}
	for _, id := range ids {
		if xid.IsNil(id) {
			continue
		}
		if _, err := s.Pool.Exec(ctx, `
			INSERT INTO plan_discount_customers (discount_id, customer_id)
			VALUES ($1,$2) ON CONFLICT DO NOTHING
		`, discountID, id); err != nil {
			return err
		}
	}
	return nil
}

// FindBestPlanDiscount returns the active discount that yields the lowest price
// for this customer + plan on the given calendar day (tenant-local date string YYYY-MM-DD).
func (s *Store) FindBestPlanDiscount(ctx context.Context, tenantID, planID, customerID xid.ID, on string, base int64) (*PlanDiscount, error) {
	if xid.IsNil(customerID) || base <= 0 {
		return nil, nil
	}
	on = strings.TrimSpace(on)
	if on == "" {
		on = time.Now().Format("2006-01-02")
	}
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return nil, err
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT d.id, d.tenant_id, d.name, d.plan_id, COALESCE(p.name, ''),
		       d.kind, d.value, d.starts_on::text, d.ends_on::text, d.audience, d.is_active,
		       0, d.created_at
		FROM plan_discounts d
		LEFT JOIN plans p ON p.id = d.plan_id
		WHERE d.tenant_id = $1
		  AND d.is_active
		  AND d.starts_on <= $4::date
		  AND d.ends_on >= $4::date
		  AND (d.plan_id IS NULL OR d.plan_id = $2)
		  AND (
		    d.audience = 'all'
		    OR EXISTS (
		      SELECT 1 FROM plan_discount_customers c
		      WHERE c.discount_id = d.id AND c.customer_id = $3
		    )
		  )
	`, tenantID, planID, customerID, on)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var best *PlanDiscount
	bestPrice := base
	for rows.Next() {
		var d PlanDiscount
		if err := rows.Scan(&d.ID, &d.TenantID, &d.Name, &d.PlanID, &d.PlanName,
			&d.Kind, &d.Value, &d.StartsOn, &d.EndsOn, &d.Audience, &d.IsActive,
			&d.CustomerCount, &d.CreatedAt); err != nil {
			return nil, err
		}
		price := ApplyPlanDiscount(base, &d)
		better := best == nil || price < bestPrice
		if !better && price == bestPrice && best != nil && discountSpecificity(&d) > discountSpecificity(best) {
			better = true
		}
		if better {
			cp := d
			best = &cp
			bestPrice = price
		}
	}
	return best, rows.Err()
}

// ResolveBilledPlanPrice is cluster offer (or base) minus the best customer discount.
func (s *Store) ResolveBilledPlanPrice(ctx context.Context, tenantID, planID, customerID xid.ID, clusterID *xid.ID, at time.Time) (int64, *PlanDiscount, error) {
	base, err := s.ResolvePlanPrice(ctx, tenantID, planID, clusterID)
	if err != nil {
		return 0, nil, err
	}
	if at.IsZero() {
		at = time.Now()
	}
	disc, err := s.FindBestPlanDiscount(ctx, tenantID, planID, customerID, at.Format("2006-01-02"), base)
	if err != nil {
		return base, nil, nil
	}
	return ApplyPlanDiscount(base, disc), disc, nil
}

func (s *Store) DecoratePortalPlanPrice(ctx context.Context, tenantID, customerID xid.ID, p *PortalPlanOption) {
	if p == nil || xid.IsNil(customerID) {
		return
	}
	disc, err := s.FindBestPlanDiscount(ctx, tenantID, p.ID, customerID, time.Now().Format("2006-01-02"), p.Price)
	if err != nil || disc == nil {
		return
	}
	next := ApplyPlanDiscount(p.Price, disc)
	if next >= p.Price {
		return
	}
	p.OriginalPrice = p.Price
	p.Price = next
	p.DiscountLabel = disc.Label()
}
