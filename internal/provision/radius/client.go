package radius

import (
	"context"
	"fmt"

	"github.com/dianrp-space/d5net-billing/internal/provision"
	"github.com/dianrp-space/d5net-billing/internal/store"
	"github.com/dianrp-space/d5net-billing/internal/xid"
)

// Client implements RADIUS provisioning via radcheck/radreply tables in PostgreSQL.
// Requires FreeRADIUS with rlm_sql pointing to the same database.
type Client struct {
	store *store.Store
}

func New(st *store.Store) *Client {
	return &Client{store: st}
}

func (c *Client) Capabilities() provision.Caps {
	return provision.Caps{PPPoE: true, Hotspot: true, CoA: true}
}

func (c *Client) Apply(ctx context.Context, spec *provision.ServiceSpec) error {
	_, err := c.store.Pool.Exec(ctx, `
		DELETE FROM radcheck WHERE username = $1;
		INSERT INTO radcheck (username, attribute, op, value) VALUES ($1, 'Cleartext-Password', ':=', $2)
		ON CONFLICT DO NOTHING
	`, spec.Username, spec.Password)
	if err != nil {
		return fmt.Errorf("radius apply: %w", err)
	}
	if spec.ProfileName != "" {
		_, err = c.store.Pool.Exec(ctx, `
			DELETE FROM radreply WHERE username = $1 AND attribute = 'Mikrotik-Rate-Limit';
			INSERT INTO radreply (username, attribute, op, value) VALUES ($1, 'Mikrotik-Rate-Limit', ':=', $2)
		`, spec.Username, spec.ProfileName)
	}
	return err
}

func (c *Client) Suspend(ctx context.Context, spec *provision.ServiceSpec) error {
	_, err := c.store.Pool.Exec(ctx, `
		UPDATE radcheck SET value = 'REJECT' WHERE username = $1 AND attribute = 'Auth-Type'
	`, spec.Username)
	if err != nil {
		_, err = c.store.Pool.Exec(ctx, `
			INSERT INTO radcheck (username, attribute, op, value) VALUES ($1, 'Auth-Type', ':=', 'Reject')
		`, spec.Username)
	}
	return err
}

func (c *Client) Resume(ctx context.Context, spec *provision.ServiceSpec) error {
	_, err := c.store.Pool.Exec(ctx, `DELETE FROM radcheck WHERE username = $1 AND attribute = 'Auth-Type'`, spec.Username)
	if err != nil {
		return err
	}
	return c.Apply(ctx, spec)
}

func (c *Client) Remove(ctx context.Context, spec *provision.ServiceSpec) error {
	_, err := c.store.Pool.Exec(ctx, `
		DELETE FROM radcheck WHERE username = $1;
		DELETE FROM radreply WHERE username = $1
	`, spec.Username)
	return err
}

func (c *Client) ActiveSessions(ctx context.Context, routerID xid.ID) ([]provision.Session, error) {
	rows, err := c.store.Pool.Query(ctx, `
		SELECT username, framedipaddress::text, acctstarttime, acctinputoctets, acctoutputoctets
		FROM radacct WHERE acctstoptime IS NULL
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var sessions []provision.Session
	for rows.Next() {
		var s provision.Session
		if err := rows.Scan(&s.Username, &s.IPAddress, &s.StartedAt, &s.RxBytes, &s.TxBytes); err != nil {
			return nil, err
		}
		sessions = append(sessions, s)
	}
	return sessions, rows.Err()
}

func (c *Client) TestConnection(ctx context.Context, routerID xid.ID) (string, error) {
	var exists bool
	err := c.store.Pool.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name = 'radcheck')
	`).Scan(&exists)
	if err != nil || !exists {
		return "", fmt.Errorf("radcheck table not found - configure FreeRADIUS rlm_sql first")
	}
	return "RADIUS SQL (radcheck) tersedia", nil
}

func (c *Client) BackupConfig(ctx context.Context, routerID xid.ID) (string, error) {
	return "", fmt.Errorf("backup not supported for RADIUS provisioner")
}
