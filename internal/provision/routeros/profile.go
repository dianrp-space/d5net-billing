package routeros

import (
	"context"
	"fmt"

	"github.com/dianrp-space/d5net-billing/internal/provision"
	"github.com/dianrp-space/d5net-billing/internal/xid"
	"github.com/go-routeros/routeros/v3"
)

// EnsureBandwidthProfile creates or updates a PPP/hotspot profile with rate-limit on the router.
// Updates in place when the name already exists so PPP secrets keep their profile reference
// (remove+add would leave secrets with unknown/empty profile).
func (c *Client) EnsureBandwidthProfile(ctx context.Context, tenantID, routerID xid.ID, name string, downloadMbps, uploadMbps int, serviceType, addressPool string, price int64) error {
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
	rate := mbpsLimit(down) + "/" + mbpsLimit(up)
	app, err := c.store.EffectiveAppName(ctx, tenantID)
	if err != nil || app == "" {
		app = "D5Net"
	}
	comment := provision.ProfileComment(app, price)

	switch serviceType {
	case "hotspot":
		// Note: /ip/hotspot/user/profile has no "comment" property (unlike /ppp/profile).
		props := []string{
			"=rate-limit=" + rate,
		}
		if addressPool != "" {
			props = append(props, "=address-pool="+addressPool)
		}
		return c.run(ctx, tenantID, routerID, nil, "/ip/hotspot/user/profile/set", func(cl *routeros.Client) (*routeros.Reply, error) {
			return upsertByName(cl, "/ip/hotspot/user/profile", name, props)
		})
	default:
		props := []string{
			"=rate-limit=" + rate,
			"=comment=" + comment,
		}
		if addressPool != "" {
			props = append(props, "=remote-address="+addressPool)
		}
		return c.run(ctx, tenantID, routerID, nil, "/ppp/profile/set", func(cl *routeros.Client) (*routeros.Reply, error) {
			return upsertByName(cl, "/ppp/profile", name, props)
		})
	}
}
