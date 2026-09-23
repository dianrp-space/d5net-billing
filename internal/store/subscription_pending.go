package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/dianrp-space/d5net-billing/internal/xid"
)

// PendingPlanChange is an Upgrade (speed naik) whose delta invoice must be
// paid in full before the subscription plan (and router profile) is switched.
// Stored in subscriptions.metadata under key "pending_plan_change" so no
// schema migration is needed.
type PendingPlanChange struct {
	OldPlanID  xid.ID   `json:"old_plan_id"`
	NewPlanID  xid.ID   `json:"new_plan_id"`
	InvoiceID  xid.ID   `json:"invoice_id"`
	Direction  string   `json:"direction"`
	ServiceType string  `json:"service_type,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

const pendingPlanChangeMetaKey = "pending_plan_change"

// GetSubscriptionPendingPlan returns the pending upgrade for a subscription, or nil.
func (s *Store) GetSubscriptionPendingPlan(ctx context.Context, tenantID, subscriptionID xid.ID) (*PendingPlanChange, error) {
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return nil, err
	}
	var raw string
	err := s.Pool.QueryRow(ctx, `
		SELECT COALESCE((metadata->'pending_plan_change')::text, 'null')
		FROM subscriptions WHERE tenant_id=$1 AND id=$2
	`, tenantID, subscriptionID).Scan(&raw)
	if err != nil {
		return nil, err
	}
	if raw == "" || raw == "null" {
		return nil, nil
	}
	var p PendingPlanChange
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return nil, err
	}
	if xid.IsNil(p.NewPlanID) {
		return nil, nil
	}
	return &p, nil
}

// SetSubscriptionPendingPlan stores/overwrites the pending upgrade.
func (s *Store) SetSubscriptionPendingPlan(ctx context.Context, tenantID, subscriptionID xid.ID, pending *PendingPlanChange) error {
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return err
	}
	if pending.CreatedAt.IsZero() {
		pending.CreatedAt = time.Now()
	}
	blob, err := json.Marshal(pending)
	if err != nil {
		return err
	}
	_, err = s.Pool.Exec(ctx, `
		UPDATE subscriptions
		SET metadata = COALESCE(metadata, '{}'::jsonb) || jsonb_build_object('pending_plan_change', $3::jsonb),
		    updated_at = NOW()
		WHERE tenant_id=$1 AND id=$2
	`, tenantID, subscriptionID, string(blob))
	return err
}

// ClearSubscriptionPendingPlan removes the pending upgrade marker.
func (s *Store) ClearSubscriptionPendingPlan(ctx context.Context, tenantID, subscriptionID xid.ID) error {
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return err
	}
	_, err := s.Pool.Exec(ctx, `
		UPDATE subscriptions
		SET metadata = COALESCE(metadata, '{}'::jsonb) - 'pending_plan_change',
		    updated_at = NOW()
		WHERE tenant_id=$1 AND id=$2
	`, tenantID, subscriptionID)
	return err
}

// PendingPlanInvoicePaid reports whether the pending upgrade invoice is fully paid.
func (s *Store) PendingPlanInvoicePaid(ctx context.Context, tenantID xid.ID, pending *PendingPlanChange) (bool, error) {
	if pending == nil || xid.IsNil(pending.InvoiceID) {
		return false, nil
	}
	inv, _, err := s.GetInvoice(ctx, tenantID, pending.InvoiceID)
	if err != nil {
		return false, err
	}
	if inv == nil {
		return false, nil
	}
	if inv.PaidAmount >= inv.TotalAmount && inv.TotalAmount > 0 {
		return true, nil
	}
	return inv.Status == "paid", nil
}
