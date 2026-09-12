package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/dianrp-space/d5net-billing/internal/xid"
)

// AuditLog adalah satu baris audit untuk halaman Audit Log.
type AuditLog struct {
	ID         xid.ID         `json:"id"`
	Action     string         `json:"action"`
	EntityType *string        `json:"entity_type,omitempty"`
	EntityID   *xid.ID        `json:"entity_id,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
	IPAddress  *string        `json:"ip_address,omitempty"`
	CreatedAt  time.Time      `json:"created_at"`
	UserID     *xid.ID        `json:"user_id,omitempty"`
	UserEmail  *string        `json:"user_email,omitempty"`
}

// ListAuditLogs memfilter audit_logs untuk halaman Audit Log.
func (s *Store) ListAuditLogs(ctx context.Context, tenantID xid.ID, action, search string, limit, offset int) ([]AuditLog, int64, error) {
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return nil, 0, err
	}
	where := "WHERE a.tenant_id = $1"
	args := []any{tenantID}
	if v := strings.TrimSpace(action); v != "" {
		args = append(args, v)
		where += fmt.Sprintf(" AND a.action = $%d", len(args))
	}
	if v := strings.TrimSpace(search); v != "" {
		args = append(args, "%"+v+"%")
		n := len(args)
		where += fmt.Sprintf(" AND (a.action ILIKE $%d OR COALESCE(a.entity_type,'') ILIKE $%d OR COALESCE(u.email,'') ILIKE $%d OR COALESCE(a.metadata::text,'') ILIKE $%d)", n, n, n, n)
	}
	var total int64
	if err := s.Pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM audit_logs a LEFT JOIN users u ON u.id = a.user_id
		`+where, args...).Scan(&total); err != nil {
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
		SELECT a.id, a.action, a.entity_type, a.entity_id, a.metadata,
		       NULLIF(a.ip_address::text, ''), a.created_at, a.user_id, u.email
		FROM audit_logs a LEFT JOIN users u ON u.id = a.user_id
		` + where + fmt.Sprintf(" ORDER BY a.created_at DESC, a.id DESC LIMIT $%d OFFSET $%d", len(args)-1, len(args))
	rows, err := s.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var list []AuditLog
	for rows.Next() {
		var l AuditLog
		var meta []byte
		if err := rows.Scan(&l.ID, &l.Action, &l.EntityType, &l.EntityID, &meta,
			&l.IPAddress, &l.CreatedAt, &l.UserID, &l.UserEmail); err != nil {
			return nil, 0, err
		}
		if len(meta) > 0 {
			m := map[string]any{}
			if jerr := json.Unmarshal(meta, &m); jerr == nil {
				l.Metadata = m
			}
		}
		list = append(list, l)
	}
	return list, total, rows.Err()
}
