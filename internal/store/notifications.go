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

// NotificationLog adalah satu baris riwayat pengiriman (antrean notifikasi).
type NotificationLog struct {
	ID          xid.ID     `json:"id"`
	Channel     string     `json:"channel"`
	Event       string     `json:"event"`
	Recipient   string     `json:"recipient"`
	Subject     *string    `json:"subject,omitempty"`
	Body        string     `json:"body"`
	Status      string     `json:"status"`
	Attempts    int        `json:"attempts"`
	Error       *string    `json:"error,omitempty"`
	BatchID     *xid.ID    `json:"batch_id,omitempty"`
	ScheduledAt time.Time  `json:"scheduled_at"`
	SentAt      *time.Time `json:"sent_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

// ListNotificationHistory memfilter antrean notifikasi untuk halaman Riwayat.
func (s *Store) ListNotificationHistory(ctx context.Context, tenantID xid.ID, status, channel, search string, limit, offset int) ([]NotificationLog, int64, error) {
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return nil, 0, err
	}
	where := "WHERE tenant_id = $1"
	args := []any{tenantID}
	if v := strings.TrimSpace(status); v != "" {
		args = append(args, v)
		where += fmt.Sprintf(" AND status = $%d", len(args))
	}
	if v := strings.TrimSpace(channel); v != "" {
		args = append(args, v)
		where += fmt.Sprintf(" AND channel = $%d", len(args))
	}
	if v := strings.TrimSpace(search); v != "" {
		args = append(args, "%"+v+"%")
		n := len(args)
		where += fmt.Sprintf(" AND (recipient ILIKE $%d OR body ILIKE $%d OR COALESCE(subject,'') ILIKE $%d)", n, n, n)
	}
	var total int64
	if err := s.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM notification_queue `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}
	args = append(args, limit, offset)
	q := `
		SELECT id, channel, event, recipient, subject, body, status, attempts, error,
		       batch_id, scheduled_at, sent_at, created_at
		FROM notification_queue ` + where +
		fmt.Sprintf(" ORDER BY created_at DESC, id DESC LIMIT $%d OFFSET $%d", len(args)-1, len(args))
	rows, err := s.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var list []NotificationLog
	for rows.Next() {
		var n NotificationLog
		if err := rows.Scan(&n.ID, &n.Channel, &n.Event, &n.Recipient, &n.Subject, &n.Body, &n.Status,
			&n.Attempts, &n.Error, &n.BatchID, &n.ScheduledAt, &n.SentAt, &n.CreatedAt); err != nil {
			return nil, 0, err
		}
		list = append(list, n)
	}
	return list, total, rows.Err()
}

// NotificationHistoryStats menghitung status antrean untuk ringkasan riwayat.
func (s *Store) NotificationHistoryStats(ctx context.Context, tenantID xid.ID) (pending, sent, failed int64, err error) {
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return 0, 0, 0, err
	}
	err = s.Pool.QueryRow(ctx, `
		SELECT COUNT(*) FILTER (WHERE status = 'pending'),
		       COUNT(*) FILTER (WHERE status = 'sent'),
		       COUNT(*) FILTER (WHERE status = 'failed')
		FROM notification_queue WHERE tenant_id = $1
	`, tenantID).Scan(&pending, &sent, &failed)
	return pending, sent, failed, err
}

// CountPurgeableNotificationHistory menghitung log final (sent/failed) yang lebih
// tua dari retentionDays hari — dipakai untuk konfirmasi sebelum hapus.
func (s *Store) CountPurgeableNotificationHistory(ctx context.Context, tenantID xid.ID, retentionDays int) (int64, error) {
	if retentionDays < 1 {
		return 0, fmt.Errorf("retensi minimal 1 hari")
	}
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return 0, err
	}
	var n int64
	err := s.Pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM notification_queue
		WHERE tenant_id = $1
		  AND status IN ('sent','failed')
		  AND created_at < NOW() - make_interval(days => $2)
	`, tenantID, retentionDays).Scan(&n)
	return n, err
}

// PurgeNotificationHistory menghapus log notifikasi final (sent/failed) yang lebih
// tua dari retentionDays hari. Antrean pending tidak pernah dihapus agar
// pengiriman yang belum diproses worker tetap aman.
func (s *Store) PurgeNotificationHistory(ctx context.Context, tenantID xid.ID, retentionDays int) (int64, error) {
	if retentionDays < 1 {
		return 0, fmt.Errorf("retensi minimal 1 hari")
	}
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return 0, err
	}
	tag, err := s.Pool.Exec(ctx, `
		DELETE FROM notification_queue
		WHERE tenant_id = $1
		  AND status IN ('sent','failed')
		  AND created_at < NOW() - make_interval(days => $2)
	`, tenantID, retentionDays)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
