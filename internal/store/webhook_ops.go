package store

import (
	"context"
	"errors"
	"github.com/dianrp/drp-billing/internal/xid"

	"github.com/jackc/pgx/v5"
)

func (s *Store) ListRecentAlerts(ctx context.Context, tenantID xid.ID, limit int) ([]Alert, error) {
	return s.ListAlerts(ctx, tenantID, limit)
}

func (s *Store) GetPaymentIntentByExternalID(ctx context.Context, externalID string) (*PaymentIntent, error) {
	row := s.Pool.QueryRow(ctx, `
		SELECT id, tenant_id, customer_id, invoice_id, provider, COALESCE(external_id,''), amount, status,
		       COALESCE(checkout_url,''), created_at, updated_at
		FROM payment_intents WHERE external_id = $1
		ORDER BY id DESC LIMIT 1
	`, externalID)
	var pi PaymentIntent
	err := row.Scan(&pi.ID, &pi.TenantID, &pi.CustomerID, &pi.InvoiceID, &pi.Provider, &pi.ExternalID,
		&pi.Amount, &pi.Status, &pi.CheckoutURL, &pi.CreatedAt, &pi.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &pi, nil
}

func (s *Store) UpdatePaymentIntentStatus(ctx context.Context, externalID, status string) error {
	_, err := s.Pool.Exec(ctx, `
		UPDATE payment_intents SET status=$2, updated_at=NOW()
		WHERE external_id=$1 AND status <> 'paid'
	`, externalID, status)
	return err
}

// DispatchOutboundEvent queues deliveries for active webhooks subscribed to event.
func (s *Store) DispatchOutboundEvent(ctx context.Context, tenantID xid.ID, event string, payload map[string]any) error {
	hooks, err := s.ListOutboundWebhooks(ctx, tenantID)
	if err != nil {
		return err
	}
	for _, w := range hooks {
		if !w.IsActive {
			continue
		}
		if len(w.Events) > 0 && !containsString(w.Events, event) {
			continue
		}
		if err := s.EnqueueOutboundDelivery(ctx, tenantID, w.ID, event, payload); err != nil {
			return err
		}
	}
	return nil
}

func containsString(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}
