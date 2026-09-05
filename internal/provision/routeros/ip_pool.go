package routeros

import (
	"context"
	"fmt"
	"strings"

	"github.com/dianrp/drp-billing/internal/xid"
	"github.com/go-routeros/routeros/v3"
)

// EnsureIPPool creates or updates /ip/pool on the router from a CIDR network.
// Updates in place when the name exists so PPP profiles keep remote-address=pool.
func (c *Client) EnsureIPPool(ctx context.Context, tenantID, routerID xid.ID, name, network string, gateway *string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("nama pool wajib")
	}
	ranges, err := CIDRToPoolRanges(network, gateway)
	if err != nil {
		return err
	}
	comment := c.brandComment(ctx, tenantID)
	props := []string{
		"=ranges=" + ranges,
		"=comment=" + comment,
	}
	return c.run(ctx, tenantID, routerID, nil, "/ip/pool/set", func(cl *routeros.Client) (*routeros.Reply, error) {
		return upsertByName(cl, "/ip/pool", name, props)
	})
}

// RemoveIPPool deletes /ip/pool by name (best-effort).
func (c *Client) RemoveIPPool(ctx context.Context, tenantID, routerID xid.ID, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	return c.run(ctx, tenantID, routerID, nil, "/ip/pool/remove", func(cl *routeros.Client) (*routeros.Reply, error) {
		return cl.Run("/ip/pool/remove", "=numbers="+name)
	})
}
