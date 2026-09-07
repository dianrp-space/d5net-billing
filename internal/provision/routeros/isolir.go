package routeros

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/dianrp/drp-billing/internal/store"
	"github.com/dianrp/drp-billing/internal/xid"
	"github.com/go-routeros/routeros/v3"
)

const (
	isolirRuleCommentDNS       = "drp-isolir:dns"
	isolirRuleCommentPortal    = "drp-isolir:portal"
	isolirRuleCommentNATProxy  = "drp-isolir:nat-to-proxy"
	isolirProxyAllowComment    = "drp-isolir:proxy-allow-portal"
	isolirProxyRedirectComment = "drp-isolir:proxy-redirect"
	isolirDefaultProxyPort     = "8080"
)

// EnsureIsolirInfra creates pool + PPP/hotspot isolir profile + Web Proxy redirect + NAT/filter.
func (c *Client) EnsureIsolirInfra(ctx context.Context, tenantID, routerID xid.ID, cfg store.IsolirNetworkSettings, tenantSlug string) error {
	poolName := cfg.PoolName
	if poolName == "" {
		poolName = "isolir"
	}
	profileName := cfg.ProfileName
	if profileName == "" {
		profileName = "isolir"
	}
	rawRanges := strings.TrimSpace(cfg.PoolRanges)
	if rawRanges == "" {
		return fmt.Errorf("isolir pool (IPAM) wajib dipilih")
	}
	ranges := rawRanges
	if strings.Contains(rawRanges, "/") {
		converted, err := CIDRToPoolRanges(rawRanges, nil)
		if err != nil {
			return fmt.Errorf("isolir pool network: %w", err)
		}
		ranges = converted
	}

	comment := c.brandComment(ctx, tenantID)
	if err := c.run(ctx, tenantID, routerID, nil, "/ip/pool/set", func(cl *routeros.Client) (*routeros.Reply, error) {
		return upsertByName(cl, "/ip/pool", poolName, []string{"=ranges=" + ranges, "=comment=" + comment})
	}); err != nil {
		return fmt.Errorf("isolir pool: %w", err)
	}

	if err := c.run(ctx, tenantID, routerID, nil, "/ppp/profile/set", func(cl *routeros.Client) (*routeros.Reply, error) {
		return upsertByName(cl, "/ppp/profile", profileName, []string{
			"=remote-address=" + poolName,
			"=rate-limit=1M/1M",
			"=comment=" + comment + " isolir",
		})
	}); err != nil {
		return fmt.Errorf("isolir ppp profile: %w", err)
	}

	_ = c.run(ctx, tenantID, routerID, nil, "/ip/hotspot/user/profile/set", func(cl *routeros.Client) (*routeros.Reply, error) {
		return upsertByName(cl, "/ip/hotspot/user/profile", profileName, []string{
			"=address-pool=" + poolName,
			"=rate-limit=1M/1M",
		})
	})

	portalHost, portalPath := isolirPortalHostPath(cfg.PortalBaseURL, tenantSlug)
	if portalHost == "" {
		return fmt.Errorf("portal_base_url wajib diisi untuk redirect isolir")
	}
	isolirURL := strings.TrimRight(strings.TrimSpace(cfg.PortalBaseURL), "/") + portalPath
	src := rangesToSrcMatch(ranges)

	return c.run(ctx, tenantID, routerID, nil, "/ip/proxy/set", func(cl *routeros.Client) (*routeros.Reply, error) {
		if err := upsertFirewallFilterByComment(cl, isolirRuleCommentDNS, []string{
			"=chain=forward",
			"=src-address=" + src,
			"=protocol=udp",
			"=dst-port=53",
			"=action=accept",
			"=comment=" + isolirRuleCommentDNS,
		}); err != nil {
			return nil, err
		}
		_ = upsertFirewallFilterByComment(cl, isolirRuleCommentDNS+"-tcp", []string{
			"=chain=forward",
			"=src-address=" + src,
			"=protocol=tcp",
			"=dst-port=53",
			"=action=accept",
			"=comment=" + isolirRuleCommentDNS + "-tcp",
		})
		_ = upsertFirewallFilterByComment(cl, isolirRuleCommentPortal, []string{
			"=chain=forward",
			"=src-address=" + src,
			"=dst-address=" + portalHost,
			"=action=accept",
			"=comment=" + isolirRuleCommentPortal,
		})

		if _, err := cl.Run("/ip/proxy/set", "=enabled=yes", "=port="+isolirDefaultProxyPort); err != nil {
			return nil, fmt.Errorf("enable web proxy: %w", err)
		}
		if err := upsertProxyAccessByComment(cl, isolirProxyAllowComment, []string{
			"=src-address=" + src,
			"=dst-host=" + portalHost,
			"=action=allow",
			"=comment=" + isolirProxyAllowComment,
		}); err != nil {
			return nil, fmt.Errorf("proxy allow portal: %w", err)
		}
		if err := upsertProxyAccessByComment(cl, isolirProxyRedirectComment, []string{
			"=src-address=" + src,
			"=action=deny",
			"=redirect-to=" + isolirURL,
			"=comment=" + isolirProxyRedirectComment,
		}); err != nil {
			return nil, fmt.Errorf("proxy redirect: %w", err)
		}
		if err := upsertFirewallNATByComment(cl, isolirRuleCommentNATProxy, []string{
			"=chain=dstnat",
			"=src-address=" + src,
			"=protocol=tcp",
			"=dst-port=80",
			"=action=redirect",
			"=to-ports=" + isolirDefaultProxyPort,
			"=comment=" + isolirRuleCommentNATProxy,
		}); err != nil {
			return nil, fmt.Errorf("nat to proxy: %w", err)
		}
		return nil, nil
	})
}

func isolirPortalHostPath(baseURL, tenantSlug string) (host, path string) {
	path = "/isolir/" + strings.TrimSpace(tenantSlug)
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return "", path
	}
	u, err := url.Parse(baseURL)
	if err != nil || u.Host == "" {
		host = strings.TrimPrefix(strings.TrimPrefix(baseURL, "https://"), "http://")
		host = strings.Split(host, "/")[0]
		return host, path
	}
	return u.Hostname(), path
}

func rangesToSrcMatch(ranges string) string {
	ranges = strings.TrimSpace(ranges)
	if i := strings.IndexAny(ranges, ",;"); i >= 0 {
		ranges = strings.TrimSpace(ranges[:i])
	}
	return ranges
}

func upsertFirewallFilterByComment(cl *routeros.Client, comment string, props []string) error {
	return upsertByComment(cl, "/ip/firewall/filter", comment, props)
}

func upsertFirewallNATByComment(cl *routeros.Client, comment string, props []string) error {
	return upsertByComment(cl, "/ip/firewall/nat", comment, props)
}

func upsertProxyAccessByComment(cl *routeros.Client, comment string, props []string) error {
	return upsertByComment(cl, "/ip/proxy/access", comment, props)
}

func upsertByComment(cl *routeros.Client, basePath, comment string, props []string) error {
	reply, err := cl.Run(basePath+"/print", "=.proplist=.id,comment")
	if err == nil && reply != nil {
		for _, re := range reply.Re {
			id := re.Map[".id"]
			cmt := re.Map["comment"]
			if id == "" || !strings.Contains(cmt, comment) {
				continue
			}
			args := append([]string{basePath + "/set", "=numbers=" + id}, props...)
			_, err = cl.Run(args...)
			return err
		}
	}
	args := append([]string{basePath + "/add"}, props...)
	_, err = cl.Run(args...)
	return err
}
