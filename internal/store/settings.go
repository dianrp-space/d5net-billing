package store

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/dianrp/drp-billing/internal/xid"
	"github.com/jackc/pgx/v5"
)

func (s *Store) GetSettingJSON(ctx context.Context, tenantID xid.ID, key string, dest any) error {
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return err
	}
	var raw []byte
	err := s.Pool.QueryRow(ctx, `
		SELECT value FROM settings WHERE tenant_id=$1 AND key=$2
	`, tenantID, key).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if len(raw) == 0 {
		raw = []byte("{}")
	}
	return json.Unmarshal(raw, dest)
}

func (s *Store) UpsertSettingJSON(ctx context.Context, tenantID xid.ID, key string, value any) error {
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return err
	}
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = s.Pool.Exec(ctx, `
		INSERT INTO settings (tenant_id, key, value, updated_at)
		VALUES ($1,$2,$3::jsonb,NOW())
		ON CONFLICT (tenant_id, key) DO UPDATE SET value=EXCLUDED.value, updated_at=NOW()
	`, tenantID, key, b)
	return err
}
