package db

import (
	"context"
	"fmt"

	"github.com/dianrp-space/d5net-billing/internal/xid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DB struct {
	Pool *pgxpool.Pool
}

func Connect(ctx context.Context, databaseURL string) (*DB, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("connect db: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping db: %w", err)
	}
	return &DB{Pool: pool}, nil
}

func (d *DB) Close() {
	d.Pool.Close()
}

func WithTenant(ctx context.Context, tenantID xid.ID) context.Context {
	return context.WithValue(ctx, tenantKey{}, tenantID)
}

func TenantFromContext(ctx context.Context) (xid.ID, bool) {
	v, ok := ctx.Value(tenantKey{}).(xid.ID)
	return v, ok
}

func SetTenant(ctx context.Context, conn *pgxpool.Pool, tenantID xid.ID) error {
	_, err := conn.Exec(ctx, "SELECT set_config('app.tenant_id', $1, false)", tenantID.String())
	return err
}

func SetTenantTx(ctx context.Context, tx pgx.Tx, tenantID xid.ID) error {
	_, err := tx.Exec(ctx, "SELECT set_config('app.tenant_id', $1, true)", tenantID.String())
	return err
}

type tenantKey struct{}
