package routeros

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/dianrp-space/d5net-billing/internal/store"
	"github.com/dianrp-space/d5net-billing/internal/xid"
	"github.com/go-routeros/routeros/v3"
)

const (
	isolirRuleCommentDNS       = "d5n-isolir:dns"
	isolirRuleCommentPortal    = "d5n-isolir:portal"
	isolirRuleCommentNATProxy  = "d5n-isolir:nat-to-proxy"
	isolirProxyAllowComment    = "d5n-isolir:proxy-allow-portal"
	isolirProxyRedirectComment = "d5n-isolir:proxy-redirect"
	isolirPortalAddressList    = "d5n-isolir-portal"
	isolirDefaultProxyPort     = "8080"
)

// isolirLegacyComments maps current infra comments to their drp- era names.
// Sync matches either and renames legacy rows in place, so existing routers
// migrate without duplicate rules.
var isolirLegacyComments = map[string]string{
	isolirRuleCommentDNS:          "drp-isolir:dns",
	isolirRuleCommentDNS + "-tcp": "drp-isolir:dns-tcp",
	isolirRuleCommentPortal:       "drp-isolir:portal",
	isolirRuleCommentNATProxy:     "drp-isolir:nat-to-proxy",
	isolirProxyAllowComment:       "drp-isolir:proxy-allow-portal",
	isolirProxyRedirectComment:    "drp-isolir:proxy-redirect",
}

const isolirLegacyPortalAddressList = "drp-isolir-portal"

func isolirLegacyComment(comment string) string {
	return isolirLegacyComments[comment]
}

// EnsureIsolirInfra creates pool + PPP/hotspot isolir profile + Web Proxy redirect + NAT/filter.
func (c *Client) EnsureIsolirInfra(ctx context.Context, tenantID, routerID xid.ID, cfg store.IsolirNetworkSettings, tenantSlug string) error {
	poolName := store.IsolirPoolName(cfg)
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
	var gw *string
	if g := strings.TrimSpace(cfg.PoolGateway); g != "" {
		gw = &g
	}
	ranges := rawRanges
	src := rawRanges
	if strings.Contains(rawRanges, "/") {
		converted, err := CIDRToPoolRanges(rawRanges, gw)
		if err != nil {
			return fmt.Errorf("isolir pool network: %w", err)
		}
		ranges = converted
	} else {
		src = rangesToSrcMatch(ranges)
	}
	localAddr := CIDRLocalAddress(rawRanges, gw)

	comment := c.brandComment(ctx, tenantID)
	if err := c.run(ctx, tenantID, routerID, nil, "/ip/pool/set", func(cl *routeros.Client) (*routeros.Reply, error) {
		return upsertByName(cl, "/ip/pool", poolName, []string{"=ranges=" + ranges, "=comment=" + comment})
	}); err != nil {
		return fmt.Errorf("isolir pool: %w", err)
	}

	pppProps := []string{
		"=remote-address=" + poolName,
		"=rate-limit=1M/1M",
		"=comment=" + comment + " isolir",
	}
	if localAddr != "" {
		pppProps = append(pppProps, "=local-address="+localAddr)
	}
	if err := c.run(ctx, tenantID, routerID, nil, "/ppp/profile/set", func(cl *routeros.Client) (*routeros.Reply, error) {
		return upsertByName(cl, "/ppp/profile", profileName, pppProps)
	}); err != nil {
		return fmt.Errorf("isolir ppp profile: %w", err)
	}

	_ = c.run(ctx, tenantID, routerID, nil, "/ip/hotspot/user/profile/set", func(cl *routeros.Client) (*routeros.Reply, error) {
		return upsertByName(cl, "/ip/hotspot/user/profile", profileName, []string{
			"=address-pool=" + poolName,
			"=rate-limit=1M/1M",
		})
	})

	portalHost, _ := isolirPortalHostPath(cfg.PortalBaseURL, tenantSlug)
	if portalHost == "" {
		return fmt.Errorf("portal_base_url wajib diisi untuk redirect isolir")
	}
	isolirURL := store.IsolirPortalURL(cfg.PortalBaseURL, tenantSlug)

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
		if err := upsertIsolirPortalAllow(cl, src, portalHost); err != nil {
			return nil, fmt.Errorf("allow portal: %w", err)
		}

		if err := ensureProxyEnabled(cl, isolirDefaultProxyPort); err != nil {
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
		if err := upsertProxyIsolirRedirect(cl, src, isolirURL); err != nil {
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
	path = store.IsolirClientPath(tenantSlug)
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
	return upsertByCommentWithLegacy(cl, "/ip/firewall/filter", comment, isolirLegacyComment(comment), props)
}

// upsertIsolirPortalAllow lets isolir clients reach the billing site over HTTP/HTTPS.
// dst-address on a filter is an IP prefix; putting a hostname there makes RouterOS
// resolve once and freeze a public IP (breaks Cloudflare/CDN). Address-list keeps the FQDN
// and refreshes A/AAAA records via DNS.
func upsertIsolirPortalAllow(cl *routeros.Client, src, portalHost string) error {
	if err := upsertAddressListFQDN(cl, isolirPortalAddressList, portalHost, isolirRuleCommentPortal); err != nil {
		return err
	}
	props := []string{
		"=chain=forward",
		"=src-address=" + src,
		"=dst-address-list=" + isolirPortalAddressList,
		"=protocol=tcp",
		"=dst-port=80,443",
		"=action=accept",
		"=comment=" + isolirRuleCommentPortal,
	}
	reply, err := cl.Run("/ip/firewall/filter/print")
	if err == nil && reply != nil {
		for _, re := range reply.Re {
			id := re.Map[".id"]
			cmt := strings.TrimSpace(re.Map["comment"])
			if id == "" || (cmt != isolirRuleCommentPortal && cmt != isolirLegacyComment(isolirRuleCommentPortal)) {
				continue
			}
			if rowMatchesProps(re.Map, props) && !fieldFilled(re.Map, "dst-address") {
				return nil
			}
			args := append([]string{"/ip/firewall/filter/set", "=numbers=" + id}, props...)
			if _, err = cl.Run(args...); err != nil {
				return err
			}
			if fieldFilled(re.Map, "dst-address") {
				_, _ = cl.Run("/ip/firewall/filter/unset", "=numbers="+id, "=value-name=dst-address")
			}
			return nil
		}
	}
	args := append([]string{"/ip/firewall/filter/add"}, props...)
	_, err = cl.Run(args...)
	return err
}

func upsertAddressListFQDN(cl *routeros.Client, list, address, comment string) error {
	list = strings.TrimSpace(list)
	address = strings.TrimSpace(address)
	comment = strings.TrimSpace(comment)
	if list == "" || address == "" {
		return fmt.Errorf("address-list portal wajib")
	}
	props := []string{
		"=list=" + list,
		"=address=" + address,
		"=comment=" + comment,
	}
	reply, err := cl.Run("/ip/firewall/address-list/print")
	if err == nil && reply != nil {
		for _, re := range reply.Re {
			id := re.Map[".id"]
			if id == "" || isROSTrue(re.Map["dynamic"]) {
				continue
			}
			cmt := strings.TrimSpace(re.Map["comment"])
			if cmt != comment && cmt != isolirLegacyComment(comment) {
				continue
			}
			if rowMatchesProps(re.Map, props) {
				return nil
			}
			args := append([]string{"/ip/firewall/address-list/set", "=numbers=" + id}, props...)
			_, err = cl.Run(args...)
			return err
		}
	}
	args := append([]string{"/ip/firewall/address-list/add"}, props...)
	_, err = cl.Run(args...)
	return err
}

func upsertFirewallNATByComment(cl *routeros.Client, comment string, props []string) error {
	return upsertByCommentWithLegacy(cl, "/ip/firewall/nat", comment, isolirLegacyComment(comment), props)
}

func upsertProxyAccessByComment(cl *routeros.Client, comment string, props []string) error {
	return upsertByCommentWithLegacy(cl, "/ip/proxy/access", comment, isolirLegacyComment(comment), props)
}

// proxyRedirectPropSets: RouterOS 7 uses action=redirect + action-data;
// v6 uses action=deny + redirect-to (removed in v7).
func proxyRedirectPropSets(src, isolirURL string) [][]string {
	return [][]string{
		{
			"=src-address=" + src,
			"=action=redirect",
			"=action-data=" + isolirURL,
			"=comment=" + isolirProxyRedirectComment,
		},
		{
			"=src-address=" + src,
			"=action=deny",
			"=redirect-to=" + isolirURL,
			"=comment=" + isolirProxyRedirectComment,
		},
	}
}

func upsertProxyIsolirRedirect(cl *routeros.Client, src, isolirURL string) error {
	var last error
	for _, props := range proxyRedirectPropSets(src, isolirURL) {
		if err := upsertProxyAccessByComment(cl, isolirProxyRedirectComment, props); err != nil {
			last = err
			continue
		}
		return nil
	}
	if last == nil {
		return fmt.Errorf("tidak ada aturan proxy redirect")
	}
	return last
}
