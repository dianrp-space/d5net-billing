package store

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/dianrp/drp-billing/internal/xid"
	"github.com/jackc/pgx/v5"
)

// CableRoute is a drawable fiber path (POP → ODP → … → customer).
type CableRoute struct {
	ID        xid.ID          `json:"id"`
	TenantID  xid.ID          `json:"tenant_id"`
	ClusterID *xid.ID         `json:"cluster_id,omitempty"`
	Name      string          `json:"name"`
	FromKind  string          `json:"from_kind"`
	FromID    *xid.ID         `json:"from_id,omitempty"`
	ToKind    string          `json:"to_kind"`
	ToID      *xid.ID         `json:"to_id,omitempty"`
	Path      json.RawMessage `json:"path"` // [[lng,lat],...]
	Color     string          `json:"color"`
	Notes     *string         `json:"notes,omitempty"`
	IsActive  bool            `json:"is_active"`
}

func (s *Store) ListCableRoutes(ctx context.Context, tenantID xid.ID, clusterID *xid.ID) ([]CableRoute, error) {
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return nil, err
	}
	q := `
		SELECT id, tenant_id, cluster_id, name, from_kind, from_id, to_kind, to_id, path, color, notes, is_active
		FROM cable_routes WHERE tenant_id = $1`
	args := []any{tenantID}
	if clusterID != nil {
		q += ` AND cluster_id = $2`
		args = append(args, *clusterID)
	}
	q += ` ORDER BY name`
	rows, err := s.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []CableRoute
	for rows.Next() {
		var r CableRoute
		if err := rows.Scan(&r.ID, &r.TenantID, &r.ClusterID, &r.Name, &r.FromKind, &r.FromID, &r.ToKind, &r.ToID,
			&r.Path, &r.Color, &r.Notes, &r.IsActive); err != nil {
			return nil, err
		}
		list = append(list, r)
	}
	return list, rows.Err()
}

func (s *Store) GetCableRoute(ctx context.Context, tenantID, id xid.ID) (*CableRoute, error) {
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return nil, err
	}
	row := s.Pool.QueryRow(ctx, `
		SELECT id, tenant_id, cluster_id, name, from_kind, from_id, to_kind, to_id, path, color, notes, is_active
		FROM cable_routes WHERE tenant_id = $1 AND id = $2
	`, tenantID, id)
	var r CableRoute
	err := row.Scan(&r.ID, &r.TenantID, &r.ClusterID, &r.Name, &r.FromKind, &r.FromID, &r.ToKind, &r.ToID,
		&r.Path, &r.Color, &r.Notes, &r.IsActive)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (s *Store) GetCableRouteByName(ctx context.Context, tenantID xid.ID, clusterID *xid.ID, name string) (*CableRoute, error) {
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return nil, err
	}
	var row pgx.Row
	if clusterID != nil {
		row = s.Pool.QueryRow(ctx, `
			SELECT id, tenant_id, cluster_id, name, from_kind, from_id, to_kind, to_id, path, color, notes, is_active
			FROM cable_routes WHERE tenant_id = $1 AND cluster_id = $2 AND name = $3
			LIMIT 1
		`, tenantID, *clusterID, name)
	} else {
		row = s.Pool.QueryRow(ctx, `
			SELECT id, tenant_id, cluster_id, name, from_kind, from_id, to_kind, to_id, path, color, notes, is_active
			FROM cable_routes WHERE tenant_id = $1 AND cluster_id IS NULL AND name = $2
			LIMIT 1
		`, tenantID, name)
	}
	var r CableRoute
	err := row.Scan(&r.ID, &r.TenantID, &r.ClusterID, &r.Name, &r.FromKind, &r.FromID, &r.ToKind, &r.ToID,
		&r.Path, &r.Color, &r.Notes, &r.IsActive)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (s *Store) CreateCableRoute(ctx context.Context, r *CableRoute) error {
	if err := s.SetTenantContext(ctx, r.TenantID); err != nil {
		return err
	}
	if len(r.Path) == 0 {
		r.Path = json.RawMessage("[]")
	}
	if r.Color == "" {
		r.Color = "#5A5A40"
	}
	if r.FromKind == "" {
		r.FromKind = "manual"
	}
	if r.ToKind == "" {
		r.ToKind = "manual"
	}
	return s.Pool.QueryRow(ctx, `
		INSERT INTO cable_routes (tenant_id, cluster_id, name, from_kind, from_id, to_kind, to_id, path, color, notes, is_active)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING id
	`, r.TenantID, r.ClusterID, r.Name, r.FromKind, r.FromID, r.ToKind, r.ToID, r.Path, r.Color, r.Notes, r.IsActive).Scan(&r.ID)
}

func (s *Store) UpdateCableRoute(ctx context.Context, r *CableRoute) error {
	if err := s.SetTenantContext(ctx, r.TenantID); err != nil {
		return err
	}
	if len(r.Path) == 0 {
		r.Path = json.RawMessage("[]")
	}
	tag, err := s.Pool.Exec(ctx, `
		UPDATE cable_routes SET cluster_id=$3, name=$4, from_kind=$5, from_id=$6, to_kind=$7, to_id=$8,
		                       path=$9, color=$10, notes=$11, is_active=$12, updated_at=NOW()
		WHERE tenant_id=$1 AND id=$2
	`, r.TenantID, r.ID, r.ClusterID, r.Name, r.FromKind, r.FromID, r.ToKind, r.ToID, r.Path, r.Color, r.Notes, r.IsActive)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteCableRoute(ctx context.Context, tenantID, id xid.ID) error {
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return err
	}
	tag, err := s.Pool.Exec(ctx, `DELETE FROM cable_routes WHERE tenant_id=$1 AND id=$2`, tenantID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
