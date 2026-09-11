package store

import (
	"context"
	"errors"
	"fmt"
	"github.com/dianrp-space/d5net-billing/internal/xid"
	"time"

	"github.com/jackc/pgx/v5"
)

type Router struct {
	ID          xid.ID     `json:"id"`
	TenantID    xid.ID     `json:"tenant_id"`
	SiteID      *xid.ID    `json:"cluster_id,omitempty"` // DB column site_id; API name cluster_id
	Name        string     `json:"name"`
	Address     string     `json:"address"`
	Port        int        `json:"port"`
	UseTLS      bool       `json:"use_tls"`
	Provisioner string     `json:"provisioner"`
	Username    string     `json:"username"`
	PasswordEnc []byte     `json:"-"`
	IsActive    bool       `json:"is_active"`
	LastSeenAt  *time.Time `json:"last_seen_at,omitempty"`
	LastError   *string    `json:"last_error,omitempty"`
}

func (s *Store) ListRouters(ctx context.Context, tenantID xid.ID) ([]Router, error) {
	var list []Router
	err := s.withTenant(ctx, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id, tenant_id, site_id, name, address, port, use_tls, provisioner, username, password_enc,
			       is_active, last_seen_at, last_error
			FROM routers WHERE tenant_id = $1 ORDER BY name
		`, tenantID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var r Router
			if err := rows.Scan(&r.ID, &r.TenantID, &r.SiteID, &r.Name, &r.Address, &r.Port, &r.UseTLS, &r.Provisioner,
				&r.Username, &r.PasswordEnc, &r.IsActive, &r.LastSeenAt, &r.LastError); err != nil {
				return err
			}
			list = append(list, r)
		}
		return rows.Err()
	})
	return list, err
}

func (s *Store) GetRouter(ctx context.Context, tenantID xid.ID, id xid.ID) (*Router, error) {
	var r Router
	err := s.withTenant(ctx, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			SELECT id, tenant_id, site_id, name, address, port, use_tls, provisioner, username, password_enc,
			       is_active, last_seen_at, last_error
			FROM routers WHERE tenant_id = $1 AND id = $2
		`, tenantID, id)
		err := row.Scan(&r.ID, &r.TenantID, &r.SiteID, &r.Name, &r.Address, &r.Port, &r.UseTLS, &r.Provisioner,
			&r.Username, &r.PasswordEnc, &r.IsActive, &r.LastSeenAt, &r.LastError)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (s *Store) CreateRouter(ctx context.Context, r *Router) error {
	return s.withTenant(ctx, r.TenantID, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			INSERT INTO routers (tenant_id, site_id, name, address, port, use_tls, provisioner, username, password_enc, is_active)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id
		`, r.TenantID, r.SiteID, r.Name, r.Address, r.Port, r.UseTLS, r.Provisioner, r.Username, r.PasswordEnc, r.IsActive).Scan(&r.ID)
	})
}

func (s *Store) UpdateRouter(ctx context.Context, r *Router) error {
	return s.withTenant(ctx, r.TenantID, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			UPDATE routers SET site_id=$3, name=$4, address=$5, port=$6, use_tls=$7, provisioner=$8,
			                   username=$9, password_enc=$10, is_active=$11, updated_at=NOW()
			WHERE tenant_id=$1 AND id=$2
		`, r.TenantID, r.ID, r.SiteID, r.Name, r.Address, r.Port, r.UseTLS, r.Provisioner, r.Username, r.PasswordEnc, r.IsActive)
		return err
	})
}

func (s *Store) UpdateRouterStatus(ctx context.Context, tenantID xid.ID, id xid.ID, lastSeen *time.Time, lastError *string) error {
	_, err := s.Pool.Exec(ctx, `
		UPDATE routers SET last_seen_at=$3, last_error=$4, updated_at=NOW() WHERE tenant_id=$1 AND id=$2
	`, tenantID, id, lastSeen, lastError)
	return err
}

func (s *Store) DeleteRouter(ctx context.Context, tenantID xid.ID, id xid.ID) error {
	return s.withTenant(ctx, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `DELETE FROM routers WHERE tenant_id=$1 AND id=$2`, tenantID, id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		return nil
	})
}

// RouterBlockers lists reasons a router cannot be deleted because other
// records still depend on it (returned as human-readable labels).
func (s *Store) RouterBlockers(ctx context.Context, tenantID, id xid.ID) ([]string, error) {
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return nil, err
	}
	var subs, pools, vouchers int64
	if err := s.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM subscriptions WHERE tenant_id=$1 AND router_id=$2`, tenantID, id).Scan(&subs); err != nil {
		return nil, err
	}
	if err := s.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM ip_pools WHERE tenant_id=$1 AND router_id=$2`, tenantID, id).Scan(&pools); err != nil {
		return nil, err
	}
	if err := s.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM voucher_batches WHERE tenant_id=$1 AND router_id=$2`, tenantID, id).Scan(&vouchers); err != nil {
		return nil, err
	}
	var blockers []string
	if subs > 0 {
		blockers = append(blockers, fmt.Sprintf("%d langganan", subs))
	}
	if pools > 0 {
		blockers = append(blockers, fmt.Sprintf("%d IP pool", pools))
	}
	if vouchers > 0 {
		blockers = append(blockers, fmt.Sprintf("%d batch voucher", vouchers))
	}
	var isolir IsolirNetworkSettings
	if err := s.GetSettingJSON(ctx, tenantID, IsolirNetworkSettingKey, &isolir); err == nil &&
		isolir.RouterID != nil && !xid.IsNil(*isolir.RouterID) && *isolir.RouterID == id {
		blockers = append(blockers, "router isolir (Template Isolir)")
	}
	return blockers, nil
}

func (s *Store) LogRouterCommand(ctx context.Context, tenantID xid.ID, routerID xid.ID, userID *xid.ID, command, result string, success bool) error {
	_, err := s.Pool.Exec(ctx, `
		INSERT INTO router_command_logs (tenant_id, router_id, user_id, command, result, success)
		VALUES ($1,$2,$3,$4,$5,$6)
	`, tenantID, routerID, userID, command, result, success)
	return err
}

type IPPool struct {
	ID          xid.ID   `json:"id"`
	TenantID    xid.ID   `json:"tenant_id"`
	ClusterID   *xid.ID  `json:"cluster_id,omitempty"`
	ClusterName string   `json:"cluster_name,omitempty"`
	RouterID    *xid.ID  `json:"router_id,omitempty"`
	RouterName  *string  `json:"router_name,omitempty"`
	Name        string   `json:"name"`
	Network     string   `json:"network"`
	Gateway     *string  `json:"gateway,omitempty"`
	DNSServers  []string `json:"dns_servers,omitempty"`
	UsedCount   int      `json:"used_count"`
}

const ipPoolSelect = `
			SELECT p.id, p.tenant_id, p.cluster_id, COALESCE(s.name, ''), p.router_id, r.name,
			       p.name, p.network::text, host(p.gateway), p.dns_servers,
			       COALESCE((SELECT COUNT(*) FROM ip_assignments a WHERE a.pool_id = p.id), 0)
			FROM ip_pools p
			LEFT JOIN routers r ON r.id = p.router_id
			LEFT JOIN sites s ON s.id = p.cluster_id
`

func scanIPPool(scan func(dest ...any) error) (IPPool, error) {
	var p IPPool
	err := scan(&p.ID, &p.TenantID, &p.ClusterID, &p.ClusterName, &p.RouterID, &p.RouterName,
		&p.Name, &p.Network, &p.Gateway, &p.DNSServers, &p.UsedCount)
	return p, err
}

func (s *Store) ListIPPools(ctx context.Context, tenantID xid.ID) ([]IPPool, error) {
	var list []IPPool
	err := s.withTenant(ctx, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, ipPoolSelect+`
			WHERE p.tenant_id = $1
			ORDER BY p.name
		`, tenantID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			p, err := scanIPPool(rows.Scan)
			if err != nil {
				return err
			}
			list = append(list, p)
		}
		return rows.Err()
	})
	return list, err
}

func (s *Store) GetIPPool(ctx context.Context, tenantID, id xid.ID) (*IPPool, error) {
	var p IPPool
	err := s.withTenant(ctx, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		row := tx.QueryRow(ctx, ipPoolSelect+`
			WHERE p.tenant_id = $1 AND p.id = $2
		`, tenantID, id)
		got, err := scanIPPool(row.Scan)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		p = got
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *Store) GetFirstIPPoolForRouter(ctx context.Context, tenantID, routerID xid.ID) (*IPPool, error) {
	var p IPPool
	err := s.withTenant(ctx, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		row := tx.QueryRow(ctx, ipPoolSelect+`
			WHERE p.tenant_id = $1 AND p.router_id = $2
			ORDER BY p.name
			LIMIT 1
		`, tenantID, routerID)
		got, err := scanIPPool(row.Scan)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		p = got
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *Store) CreateIPPool(ctx context.Context, p *IPPool) error {
	err := s.withTenant(ctx, p.TenantID, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			INSERT INTO ip_pools (tenant_id, cluster_id, router_id, name, network, gateway, dns_servers)
			VALUES ($1,$2,$3,$4,$5::cidr,$6::inet,$7) RETURNING id
		`, p.TenantID, p.ClusterID, p.RouterID, p.Name, p.Network, p.Gateway, p.DNSServers).Scan(&p.ID)
	})
	if IsUniqueViolation(err) {
		return ErrConflict
	}
	return err
}

func (s *Store) UpdateIPPool(ctx context.Context, p *IPPool) error {
	err := s.withTenant(ctx, p.TenantID, func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE ip_pools SET cluster_id=$3, router_id=$4, name=$5, network=$6::cidr, gateway=$7::inet, dns_servers=$8
			WHERE tenant_id=$1 AND id=$2
		`, p.TenantID, p.ID, p.ClusterID, p.RouterID, p.Name, p.Network, p.Gateway, p.DNSServers)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		return nil
	})
	if IsUniqueViolation(err) {
		return ErrConflict
	}
	return err
}

func (s *Store) DeleteIPPool(ctx context.Context, tenantID, id xid.ID) error {
	return s.withTenant(ctx, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `DELETE FROM ip_pools WHERE tenant_id=$1 AND id=$2`, tenantID, id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		return nil
	})
}

type IPAssignment struct {
	ID           xid.ID  `json:"id"`
	TenantID     xid.ID  `json:"tenant_id"`
	PoolID       xid.ID  `json:"pool_id"`
	CustomerID   *xid.ID `json:"customer_id,omitempty"`
	CustomerName *string `json:"customer_name,omitempty"`
	IPAddress    string  `json:"ip_address"`
	MACAddress   *string `json:"mac_address,omitempty"`
	Status       string  `json:"status"`
}

func (s *Store) ListIPAssignments(ctx context.Context, tenantID xid.ID, poolID xid.ID) ([]IPAssignment, error) {
	var list []IPAssignment
	err := s.withTenant(ctx, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT a.id, a.tenant_id, a.pool_id, a.customer_id, c.full_name, host(a.ip_address), a.mac_address::text, a.status
			FROM ip_assignments a
			LEFT JOIN customers c ON c.id = a.customer_id
			WHERE a.tenant_id = $1 AND a.pool_id = $2
			ORDER BY a.ip_address
		`, tenantID, poolID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var a IPAssignment
			if err := rows.Scan(&a.ID, &a.TenantID, &a.PoolID, &a.CustomerID, &a.CustomerName, &a.IPAddress, &a.MACAddress, &a.Status); err != nil {
				return err
			}
			list = append(list, a)
		}
		return rows.Err()
	})
	return list, err
}

func (s *Store) ListIPAssignmentsByCustomer(ctx context.Context, tenantID, customerID xid.ID) ([]IPAssignment, error) {
	var list []IPAssignment
	err := s.withTenant(ctx, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT a.id, a.tenant_id, a.pool_id, a.customer_id, c.full_name, host(a.ip_address), a.mac_address::text, a.status
			FROM ip_assignments a
			LEFT JOIN customers c ON c.id = a.customer_id
			WHERE a.tenant_id = $1 AND a.customer_id = $2
			ORDER BY a.ip_address
		`, tenantID, customerID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var a IPAssignment
			if err := rows.Scan(&a.ID, &a.TenantID, &a.PoolID, &a.CustomerID, &a.CustomerName, &a.IPAddress, &a.MACAddress, &a.Status); err != nil {
				return err
			}
			list = append(list, a)
		}
		return rows.Err()
	})
	return list, err
}

func (s *Store) AssignIP(ctx context.Context, a *IPAssignment) error {
	return s.withTenant(ctx, a.TenantID, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			INSERT INTO ip_assignments (tenant_id, pool_id, customer_id, ip_address, mac_address, status)
			VALUES ($1,$2,$3,$4::inet,$5::macaddr,$6) RETURNING id
		`, a.TenantID, a.PoolID, a.CustomerID, a.IPAddress, a.MACAddress, a.Status).Scan(&a.ID)
	})
}

func (s *Store) GetIPAssignment(ctx context.Context, tenantID, poolID, id xid.ID) (*IPAssignment, error) {
	var a IPAssignment
	err := s.withTenant(ctx, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			SELECT a.id, a.tenant_id, a.pool_id, a.customer_id, c.full_name, host(a.ip_address), a.mac_address::text, a.status
			FROM ip_assignments a
			LEFT JOIN customers c ON c.id = a.customer_id
			WHERE a.tenant_id = $1 AND a.pool_id = $2 AND a.id = $3
		`, tenantID, poolID, id)
		err := row.Scan(&a.ID, &a.TenantID, &a.PoolID, &a.CustomerID, &a.CustomerName, &a.IPAddress, &a.MACAddress, &a.Status)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (s *Store) DeleteIPAssignment(ctx context.Context, tenantID, poolID, id xid.ID) error {
	return s.withTenant(ctx, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			DELETE FROM ip_assignments WHERE tenant_id=$1 AND pool_id=$2 AND id=$3
		`, tenantID, poolID, id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		return nil
	})
}

// CustomerIPAssignment is a static IP bound to a customer via IPAM.
type CustomerIPAssignment struct {
	IPAddress string
	Pool      IPPool
}

// GetAssignedIPForCustomer returns the latest assigned IP (+ pool) for a customer.
func (s *Store) GetAssignedIPForCustomer(ctx context.Context, tenantID, customerID xid.ID) (*CustomerIPAssignment, error) {
	var out CustomerIPAssignment
	err := s.withTenant(ctx, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			SELECT host(a.ip_address),
			       p.id, p.tenant_id, p.cluster_id, COALESCE(s.name, ''), p.router_id, r.name, p.name, p.network::text, host(p.gateway), p.dns_servers,
			       COALESCE((SELECT COUNT(*) FROM ip_assignments x WHERE x.pool_id = p.id), 0)
			FROM ip_assignments a
			JOIN ip_pools p ON p.id = a.pool_id AND p.tenant_id = a.tenant_id
			LEFT JOIN routers r ON r.id = p.router_id
			LEFT JOIN sites s ON s.id = p.cluster_id
			WHERE a.tenant_id = $1 AND a.customer_id = $2 AND a.status = 'assigned'
			ORDER BY a.id DESC
			LIMIT 1
		`, tenantID, customerID)
		err := row.Scan(
			&out.IPAddress,
			&out.Pool.ID, &out.Pool.TenantID, &out.Pool.ClusterID, &out.Pool.ClusterName,
			&out.Pool.RouterID, &out.Pool.RouterName,
			&out.Pool.Name, &out.Pool.Network, &out.Pool.Gateway, &out.Pool.DNSServers, &out.Pool.UsedCount,
		)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (s *Store) SaveRouterBackup(ctx context.Context, tenantID xid.ID, routerID xid.ID, filename, content, checksum string) error {
	_, err := s.Pool.Exec(ctx, `
		INSERT INTO router_backups (tenant_id, router_id, filename, content, checksum)
		VALUES ($1,$2,$3,$4,$5)
	`, tenantID, routerID, filename, content, checksum)
	return err
}

func (s *Store) ListRouterBackups(ctx context.Context, tenantID xid.ID, routerID xid.ID) ([]map[string]any, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT id, filename, checksum, created_at FROM router_backups
		WHERE tenant_id = $1 AND router_id = $2 ORDER BY created_at DESC LIMIT 50
	`, tenantID, routerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []map[string]any
	for rows.Next() {
		var id xid.ID
		var filename, checksum string
		var createdAt time.Time
		if err := rows.Scan(&id, &filename, &checksum, &createdAt); err != nil {
			return nil, err
		}
		list = append(list, map[string]any{
			"id": id, "filename": filename, "checksum": checksum, "created_at": createdAt,
		})
	}
	return list, rows.Err()
}

func (s *Store) RouterAddress(r *Router) string {
	return fmt.Sprintf("%s:%d", r.Address, r.Port)
}
