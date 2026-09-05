package routeros

import (
	"context"
	"fmt"

	"github.com/dianrp/drp-billing/internal/provision"
	"github.com/dianrp/drp-billing/internal/xid"
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
		app = "drp-billing"
	}
	comment := provision.ProfileComment(app, price)

	switch serviceType {
	case "hotspot":
		props := []string{
			"=rate-limit=" + rate,
			"=comment=" + comment,
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

// upsertByName sets properties on an existing named row, or adds it if missing.
func upsertByName(cl *routeros.Client, basePath, name string, props []string) (*routeros.Reply, error) {
	reply, err := cl.Run(basePath+"/print", "?name="+name, "=.proplist=.id")
	if err == nil && reply != nil {
		for _, re := range reply.Re {
			id := re.Map[".id"]
			if id == "" {
				continue
			}
			args := append([]string{basePath + "/set", "=numbers=" + id}, props...)
			return cl.Run(args...)
		}
	}
	args := append([]string{basePath + "/add", "=name=" + name}, props...)
	return cl.Run(args...)
}
