package store

import (
	"context"
	"errors"
	"github.com/dianrp-space/d5net-billing/internal/xid"

	"github.com/jackc/pgx/v5"
)

func (s *Store) ListRecentAlerts(ctx context.Context, tenantID xid.ID, limit int) ([]Alert, error) {
	return s.ListAlerts(ctx, tenantID, limit)
}

func (s *Store) GetPaymentIntentByExternalID(ctx context.Context, externalID string) (*PaymentIntent, error) {
	row := s.Pool.QueryRow(ctx, `
		SELECT `+paymentIntentSelectCols()+`
		FROM payment_intents
		WHERE external_id = $1 OR metadata->>'transaction_id' = $1
		ORDER BY created_at DESC LIMIT 1
	`, externalID)
	pi, err := scanPaymentIntent(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return pi, err
}

func (s *Store) UpdatePaymentIntentStatus(ctx context.Context, externalID, status string) error {
	_, err := s.Pool.Exec(ctx, `
		UPDATE payment_intents SET status=$2, updated_at=NOW()
		WHERE external_id=$1 AND status <> 'paid'
	`, externalID, status)
	return err
}

// CancelPendingPaymentIntentsExcept cancels all pending intents for an invoice
// except the one with keepExternalID. Used when the customer switches payment
// gateway so only a single active checkout exists per invoice.
func (s *Store) CancelPendingPaymentIntentsExcept(ctx context.Context, tenantID, invoiceID xid.ID, keepExternalID string) error {
	_, err := s.Pool.Exec(ctx, `
		UPDATE payment_intents SET status='cancelled', updated_at=NOW()
		WHERE tenant_id=$1 AND invoice_id=$2 AND status='pending' AND external_id <> $3
	`, tenantID, invoiceID, keepExternalID)
	return err
}

func (s *Store) CancelPendingPaymentIntentsForInvoice(ctx context.Context, tenantID, invoiceID xid.ID) error {
	_, err := s.Pool.Exec(ctx, `
		UPDATE payment_intents SET status='cancelled', updated_at=NOW()
		WHERE tenant_id=$1 AND invoice_id=$2 AND LOWER(status) IN ('pending','created','unpaid')
	`, tenantID, invoiceID)
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
