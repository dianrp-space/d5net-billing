package store

import (
	"context"
	"errors"

	"github.com/dianrp-space/d5net-billing/internal/xid"
	"github.com/jackc/pgx/v5"
)

type NotificationTemplate struct {
	ID       xid.ID  `json:"id"`
	TenantID xid.ID  `json:"tenant_id"`
	Channel  string  `json:"channel"`
	Event    string  `json:"event"`
	Subject  *string `json:"subject,omitempty"`
	Body     string  `json:"body"`
}

func (s *Store) ListNotificationTemplates(ctx context.Context, tenantID xid.ID) ([]NotificationTemplate, error) {
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return nil, err
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT id, tenant_id, channel, event, subject, body
		FROM notification_templates WHERE tenant_id=$1
		ORDER BY channel, event
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []NotificationTemplate
	for rows.Next() {
		var t NotificationTemplate
		if err := rows.Scan(&t.ID, &t.TenantID, &t.Channel, &t.Event, &t.Subject, &t.Body); err != nil {
			return nil, err
		}
		list = append(list, t)
	}
	return list, rows.Err()
}

func (s *Store) UpsertNotificationTemplate(ctx context.Context, t *NotificationTemplate) error {
	if err := s.SetTenantContext(ctx, t.TenantID); err != nil {
		return err
	}
	return s.Pool.QueryRow(ctx, `
		INSERT INTO notification_templates (tenant_id, channel, event, subject, body)
		VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (tenant_id, channel, event) DO UPDATE
		  SET subject=EXCLUDED.subject, body=EXCLUDED.body
		RETURNING id
	`, t.TenantID, t.Channel, t.Event, t.Subject, t.Body).Scan(&t.ID)
}

func (s *Store) DeleteNotificationTemplate(ctx context.Context, tenantID, id xid.ID) error {
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return err
	}
	tag, err := s.Pool.Exec(ctx, `DELETE FROM notification_templates WHERE tenant_id=$1 AND id=$2`, tenantID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) GetNotificationTemplate(ctx context.Context, tenantID xid.ID, channel, event string) (*NotificationTemplate, error) {
	var t NotificationTemplate
	err := s.Pool.QueryRow(ctx, `
		SELECT id, tenant_id, channel, event, subject, body
		FROM notification_templates WHERE tenant_id=$1 AND channel=$2 AND event=$3
	`, tenantID, channel, event).Scan(&t.ID, &t.TenantID, &t.Channel, &t.Event, &t.Subject, &t.Body)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// ListBroadcastPhones returns distinct customer phones for an audience filter.
func (s *Store) ListBroadcastPhones(ctx context.Context, tenantID xid.ID, audience string) ([]string, error) {
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return nil, err
	}
	var q string
	switch audience {
	case "overdue":
		q = `
			SELECT DISTINCT c.phone FROM customers c
			JOIN invoices i ON i.customer_id = c.id AND i.tenant_id = c.tenant_id
			WHERE c.tenant_id=$1 AND c.phone <> ''
			  AND i.deleted_at IS NULL
			  AND i.status IN ('issued','partial','overdue')
			  AND i.total_amount > i.paid_amount
			  AND i.due_date < CURRENT_DATE`
	default: // active
		q = `
			SELECT DISTINCT c.phone FROM customers c
			JOIN subscriptions s ON s.customer_id = c.id AND s.tenant_id = c.tenant_id
			WHERE c.tenant_id=$1 AND c.phone <> '' AND s.status IN ('active','suspended','overdue')`
	}
	rows, err := s.Pool.Query(ctx, q, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var phone string
		if err := rows.Scan(&phone); err != nil {
			return nil, err
		}
		out = append(out, phone)
	}
	return out, rows.Err()
}
