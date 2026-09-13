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

// BroadcastFilter mempersempit audience broadcast ke area terdampak
// (mis. pemberitahuan gangguan): cluster = POP/area pelanggan,
// odp = ODP tempat langganan pelanggan menancap.
type BroadcastFilter struct {
	ClusterID *xid.ID
	ODPID     *xid.ID
}

// broadcastExtraConditions menyusun kondisi tambahan cluster/ODP untuk query
// audience. Kedua query dasar memakai alias "c" untuk customers dan $1 untuk
// tenant_id; kondisi ODP dicek lewat langganan (odp_ports.subscription_id)
// dengan fallback ke odp_ports.customer_id langsung.
func broadcastExtraConditions(filter BroadcastFilter, args []any) (string, []any) {
	cond := ""
	if filter.ClusterID != nil && !xid.IsNil(*filter.ClusterID) {
		args = append(args, *filter.ClusterID)
		cond += fmt.Sprintf(" AND c.cluster_id = $%d", len(args))
	}
	if filter.ODPID != nil && !xid.IsNil(*filter.ODPID) {
		args = append(args, *filter.ODPID)
		n := len(args)
		cond += fmt.Sprintf(` AND (EXISTS (
			SELECT 1 FROM subscriptions s
			JOIN odp_ports op ON op.subscription_id = s.id AND op.tenant_id = s.tenant_id
			WHERE s.customer_id = c.id AND s.tenant_id = $1 AND op.odp_id = $%d
		) OR EXISTS (
			SELECT 1 FROM odp_ports op2
			WHERE op2.customer_id = c.id AND op2.tenant_id = $1 AND op2.odp_id = $%d
		))`, n, n)
	}
	return cond, args
}

func broadcastBaseQuery(audience string) string {
	if audience == "overdue" {
		return `
			SELECT c.phone, MIN(c.full_name) AS customer_name FROM customers c
			JOIN invoices i ON i.customer_id = c.id AND i.tenant_id = c.tenant_id
			WHERE c.tenant_id=$1 AND c.phone <> ''
			  AND i.deleted_at IS NULL
			  AND i.status IN ('issued','partial','overdue')
			  AND i.total_amount > i.paid_amount
			  AND i.due_date < CURRENT_DATE`
	}
	return `
		SELECT c.phone, MIN(c.full_name) AS customer_name FROM customers c
		JOIN subscriptions s ON s.customer_id = c.id AND s.tenant_id = c.tenant_id
		WHERE c.tenant_id=$1 AND c.phone <> '' AND s.status IN ('active','suspended','overdue')`
}

const broadcastGroupBy = "\n\t\t\tGROUP BY c.phone"

// BroadcastRecipient adalah satu penerima broadcast beserta namanya (untuk
// personalisasi variabel seperti {{customer_name}}).
type BroadcastRecipient struct {
	Phone        string `json:"phone"`
	CustomerName string `json:"customer_name"`
}

// ListBroadcastPhones returns distinct customer phones for an audience filter.
func (s *Store) ListBroadcastPhones(ctx context.Context, tenantID xid.ID, audience string) ([]string, error) {
	recips, err := s.ListBroadcastRecipientsFiltered(ctx, tenantID, audience, BroadcastFilter{})
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(recips))
	for _, r := range recips {
		out = append(out, r.Phone)
	}
	return out, nil
}

// ListBroadcastRecipientsFiltered mengembalikan penerima (nomor + nama) untuk
// audience dengan filter opsional cluster dan ODP.
func (s *Store) ListBroadcastRecipientsFiltered(ctx context.Context, tenantID xid.ID, audience string, filter BroadcastFilter) ([]BroadcastRecipient, error) {
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return nil, err
	}
	args := []any{tenantID}
	cond, args := broadcastExtraConditions(filter, args)
	q := broadcastBaseQuery(audience) + cond + broadcastGroupBy
	rows, err := s.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BroadcastRecipient
	for rows.Next() {
		var r BroadcastRecipient
		if err := rows.Scan(&r.Phone, &r.CustomerName); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ListBroadcastPhonesFiltered sama seperti ListBroadcastPhones dengan filter
// opsional cluster dan ODP.
func (s *Store) ListBroadcastPhonesFiltered(ctx context.Context, tenantID xid.ID, audience string, filter BroadcastFilter) ([]string, error) {
	recips, err := s.ListBroadcastRecipientsFiltered(ctx, tenantID, audience, filter)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(recips))
	for _, r := range recips {
		out = append(out, r.Phone)
	}
	return out, nil
}

// CountBroadcastPhonesFiltered menghitung penerima tanpa memuat semuanya —
// dipakai preview audience sebelum broadcast dikirim.
func (s *Store) CountBroadcastPhonesFiltered(ctx context.Context, tenantID xid.ID, audience string, filter BroadcastFilter) (int64, error) {
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return 0, err
	}
	args := []any{tenantID}
	cond, args := broadcastExtraConditions(filter, args)
	base := broadcastBaseQuery(audience)
	// Bungkus sebagai subquery agar DISTINCT tetap dihitung tepat.
	var n int64
	if err := s.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM (`+base+cond+`) AS aud`, args...).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
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
