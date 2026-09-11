package audit

import (
	"github.com/dianrp-space/d5net-billing/internal/xid"
	"context"
	"encoding/json"
	"net"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Logger struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Logger {
	return &Logger{pool: pool}
}

func (l *Logger) Log(ctx context.Context, tenantID *xid.ID, userID *xid.ID, action, entityType string, entityID *xid.ID, metadata map[string]any, ip net.IP) error {
	meta, _ := json.Marshal(metadata)
	_, err := l.pool.Exec(ctx, `
		INSERT INTO audit_logs (tenant_id, user_id, action, entity_type, entity_id, metadata, ip_address, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, tenantID, userID, action, entityType, entityID, meta, ip, time.Now())
	return err
}
