package routeros

import (
	"context"
	"fmt"

	"github.com/dianrp/drp-billing/internal/xid"
	"github.com/go-routeros/routeros/v3"
)

// EnsureBandwidthProfile creates or updates a PPP/hotspot profile with rate-limit on the router.
func (c *Client) EnsureBandwidthProfile(ctx context.Context, tenantID, routerID xid.ID, name string, downloadMbps, uploadMbps int, serviceType string) error {
	if name == "" {
		return fmt.Errorf("profile name required")
	}
	up := uploadMbps
	if up <= 0 {
		up = downloadMbps
	}
	down := downloadMbps
	if down <= 0 {
		down = 10
	}
	// PPP profile rate-limit: rx/tx from client view ≈ download/upload.
	rate := mbpsLimit(down) + "/" + mbpsLimit(up)

	switch serviceType {
	case "hotspot":
		return c.run(ctx, tenantID, routerID, nil, "/ip/hotspot/user/profile/add", func(cl *routeros.Client) (*routeros.Reply, error) {
			_, _ = cl.Run("/ip/hotspot/user/profile/remove", "=numbers="+name)
			return cl.Run(
				"/ip/hotspot/user/profile/add",
				"=name="+name,
				"=rate-limit="+rate,
				"=comment=drp-billing",
			)
		})
	default:
		return c.run(ctx, tenantID, routerID, nil, "/ppp/profile/add", func(cl *routeros.Client) (*routeros.Reply, error) {
			_, _ = cl.Run("/ppp/profile/remove", "=numbers="+name)
			return cl.Run(
				"/ppp/profile/add",
				"=name="+name,
				"=rate-limit="+rate,
				"=comment=drp-billing",
			)
		})
	}
}
