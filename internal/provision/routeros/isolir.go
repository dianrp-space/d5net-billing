package routeros

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/dianrp-space/d5net-billing/internal/store"
	"github.com/dianrp-space/d5net-billing/internal/xid"
	"github.com/go-routeros/routeros/v3"
)

const (
	isolirRuleCommentDNS    = "d5n-isolir:dns"
	isolirRuleCommentPortal = "d5n-isolir:portal"
	isolirRuleCommentBlock  = "d5n-isolir:block"
	// isolirRuleCommentNATDstnat is the current dst-nat redirect rule comment.
	// isolirRuleCommentNATProxy is the legacy web-proxy redirect NAT comment,
	// matched during upsert so migrating routers convert the rule in place.
	isolirRuleCommentNATDstnat = "d5n-isolir:nat-dstnat"
	isolirRuleCommentNATProxy  = "d5n-isolir:nat-to-proxy"
	isolirProxyAllowComment    = "d5n-isolir:proxy-allow-portal"
	isolirProxyRedirectComment = "d5n-isolir:proxy-redirect"
	isolirPortalAddressList    = "d5n-isolir-portal"
)

// isolirLegacyComments maps current infra comments to their drp- era names.
// Sync matches either and renames legacy rows in place, so existing routers
// migrate without duplicate rules.
var isolirLegacyComments = map[string]string{
	isolirRuleCommentDNS:          "drp-isolir:dns",
	isolirRuleCommentDNS + "-tcp": "drp-isolir:dns-tcp",
	isolirRuleCommentPortal:       "drp-isolir:portal",
	isolirRuleCommentBlock:        "drp-isolir:block",
	isolirRuleCommentNATProxy:     "drp-isolir:nat-to-proxy",
	isolirProxyAllowComment:       "drp-isolir:proxy-allow-portal",
	isolirProxyRedirectComment:    "drp-isolir:proxy-redirect",
}

const isolirLegacyPortalAddressList = "drp-isolir-portal"

func isolirLegacyComment(comment string) string {
	return isolirLegacyComments[comment]
}

// EnsureIsolirInfra creates pool + PPP/hotspot isolir profile + DST-NAT redirect
// (to the app captive listener) + firewall allow DNS/portal + drop the rest.
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
	captivePort := strings.TrimSpace(cfg.IsolirHostPort)
	if captivePort == "" {
		captivePort = store.DefaultIsolirHostPort
	}
	// Optional manual override (public IP). Prefer this over DNS on the billing
	// host — that often returns a LAN address via split-horizon /hosts.
	overrideIP := strings.TrimSpace(cfg.IsolirHostIP)

	return c.run(ctx, tenantID, routerID, nil, "/ip/firewall/nat/set", func(cl *routeros.Client) (*routeros.Reply, error) {
		// Accept rules first: if any of them fails we abort WITHOUT adding the
		// drop, so a broken accept rule can never lock every client out. The
		// portal allow also permits the captive port so the dst-nat'd traffic
		// (portal IP : captivePort) is not swallowed by the drop rule below.
		acceptErr := errors.Join(
			upsertFirewallFilterByComment(cl, isolirRuleCommentDNS, []string{
				"=chain=forward",
				"=src-address=" + src,
				"=protocol=udp",
				"=dst-port=53",
				"=action=accept",
				"=comment=" + isolirRuleCommentDNS,
			}),
			upsertFirewallFilterByComment(cl, isolirRuleCommentDNS+"-tcp", []string{
				"=chain=forward",
				"=src-address=" + src,
				"=protocol=tcp",
				"=dst-port=53",
				"=action=accept",
				"=comment=" + isolirRuleCommentDNS + "-tcp",
			}),
			upsertIsolirPortalAllow(cl, src, portalHost, captivePort),
		)
		if acceptErr != nil {
			return nil, acceptErr
		}

		// DST-NAT target IP: same view as Winbox address-list (RouterOS DNS),
		// not the billing server's LookupIP (which often returns LAN).
		hostIP := overrideIP
		if hostIP == "" {
			var err error
			hostIP, err = resolveAddressListIPv4(cl, isolirPortalAddressList, portalHost)
			if err != nil {
				return nil, fmt.Errorf("resolve IP portal dari address-list %q: %w", isolirPortalAddressList, err)
			}
		} else if ip := net.ParseIP(hostIP); ip == nil || ip.To4() == nil {
			return nil, fmt.Errorf("isolir_host_ip %q bukan IPv4 valid", hostIP)
		}

		// DST-NAT: redirect isolir-pool HTTP straight to the app captive
		// listener that serves the isolir page for any host/path. No web-proxy
		// package required (works on every RouterOS 6/7). Reuses the legacy
		// nat-to-proxy comments so routers migrating from web-proxy convert the
		// existing rule in place instead of duplicating it.
		var natErr error
		if err := upsertFirewallNATByComments(cl, isolirRuleCommentNATDstnat,
			[]string{isolirRuleCommentNATProxy, "drp-isolir:nat-to-proxy"},
			[]string{
				"=chain=dstnat",
				"=src-address=" + src,
				"=protocol=tcp",
				"=dst-port=80",
				"=action=dst-nat",
				"=to-addresses=" + hostIP,
				"=to-ports=" + captivePort,
				"=comment=" + isolirRuleCommentNATDstnat,
			}); err != nil {
			natErr = fmt.Errorf("dst-nat isolir: %w", err)
		}

		// Clean up leftover web-proxy rules from the old redirect mode so the
		// router does not keep an orphan proxy redirect. Best-effort.
		_ = deleteByComment(cl, "/ip/proxy/access",
			isolirProxyAllowComment, isolirLegacyComment(isolirProxyAllowComment),
			isolirProxyRedirectComment, isolirLegacyComment(isolirProxyRedirectComment))

		// Drop everything else. Added last and ordered after every accept rule,
		// so DNS and portal access always win.
		if err := ensureFirewallBlock(cl, []string{
			"=chain=forward",
			"=src-address=" + src,
			"=action=drop",
			"=comment=" + isolirRuleCommentBlock,
		}, isolirRuleCommentDNS, isolirRuleCommentDNS+"-tcp", isolirRuleCommentPortal); err != nil {
			return nil, fmt.Errorf("block isolir: %w", err)
		}
		return nil, natErr
	})
}

// resolveAddressListIPv4 reads IPv4 addresses that RouterOS resolved for the
// portal FQDN address-list entry (same IPs shown in Winbox). Prefers public
// IPv4 over RFC1918/link-local so dst-nat does not target a LAN address that
// only the billing host itself would resolve via split-horizon DNS.
//
// RouterOS keeps the static FQDN row and adds dynamic rows with the resolved
// IP; both are scanned. Retries briefly because DNS resolution after add is
// not always instant.
func resolveAddressListIPv4(cl *routeros.Client, list, portalHost string) (string, error) {
	list = strings.TrimSpace(list)
	portalHost = strings.TrimSpace(portalHost)
	if list == "" {
		return "", fmt.Errorf("address-list kosong")
	}
	var last error
	for attempt := 0; attempt < 5; attempt++ {
		if attempt > 0 {
			time.Sleep(400 * time.Millisecond)
		}
		reply, err := cl.Run("/ip/firewall/address-list/print", "?list="+list)
		if err != nil {
			last = err
			continue
		}
		var ips []string
		for _, re := range reply.Re {
			addr := strings.TrimSpace(re.Map["address"])
			if addr == "" {
				continue
			}
			// Strip /32 (or any prefix) RouterOS sometimes prints.
			if i := strings.IndexByte(addr, '/'); i >= 0 {
				addr = addr[:i]
			}
			ip := net.ParseIP(addr)
			if ip == nil || ip.To4() == nil {
				continue // still an FQDN row, or IPv6 — skip
			}
			ips = append(ips, ip.To4().String())
		}
		if picked := pickPreferredIPv4(ips); picked != "" {
			return picked, nil
		}
		last = fmt.Errorf("belum ada IPv4 di address-list %q untuk %q (tunggu DNS RouterOS)", list, portalHost)
	}
	if last == nil {
		last = fmt.Errorf("tidak ada IPv4 di address-list %q", list)
	}
	return "", last
}

// pickPreferredIPv4 returns the first public IPv4, else the first private IPv4.
func pickPreferredIPv4(ips []string) string {
	var private string
	for _, s := range ips {
		ip := net.ParseIP(s)
		if ip == nil || ip.To4() == nil {
			continue
		}
		if !ip.IsPrivate() && !ip.IsLoopback() && !ip.IsLinkLocalUnicast() {
			return ip.To4().String()
		}
		if private == "" {
			private = ip.To4().String()
		}
	}
	return private
}

// resolveHostIPv4 resolves host to an IPv4 address on the billing host.
// Prefer resolveAddressListIPv4 for dst-nat; this remains for tests / fallbacks.
func resolveHostIPv4(host string) (string, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		return "", fmt.Errorf("host kosong")
	}
	if ip := net.ParseIP(host); ip != nil {
		return host, nil
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return "", err
	}
	var strs []string
	for _, ip := range ips {
		if v4 := ip.To4(); v4 != nil {
			strs = append(strs, v4.String())
		}
	}
	if picked := pickPreferredIPv4(strs); picked != "" {
		return picked, nil
	}
	if len(ips) > 0 {
		return ips[0].String(), nil
	}
	return "", fmt.Errorf("tidak ada IP untuk %q", host)
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

// upsertIsolirPortalAllow lets isolir clients reach the billing site over HTTP/HTTPS
// plus the captive port that dst-nat redirects HTTP to. dst-address on a filter is an
// IP prefix; putting a hostname there makes RouterOS resolve once and freeze a public
// IP (breaks Cloudflare/CDN). Address-list keeps the FQDN and refreshes A/AAAA records
// via DNS.
func upsertIsolirPortalAllow(cl *routeros.Client, src, portalHost, captivePort string) error {
	if err := upsertAddressListFQDN(cl, isolirPortalAddressList, portalHost, isolirRuleCommentPortal); err != nil {
		return err
	}
	dstPorts := "80,443"
	if p := strings.TrimSpace(captivePort); p != "" && p != "80" && p != "443" {
		dstPorts += "," + p
	}
	props := []string{
		"=chain=forward",
		"=src-address=" + src,
		"=dst-address-list=" + isolirPortalAddressList,
		"=protocol=tcp",
		"=dst-port=" + dstPorts,
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

// rosRule is a minimal view of a firewall row used to reason about ordering.
type rosRule struct {
	ID      string
	Comment string
}

// isolirBlockPosition returns where the isolir drop rule must sit so every
// isolir accept rule (DNS + portal) always wins. destination is the .id the drop
// must be moved before ("" = move to the end). needMove is false when the drop
// already sits after the last accept rule.
//
// The drop must come after the LAST accept rule, not merely after the portal
// rule: the accept rules are not guaranteed to be contiguous, and a drop placed
// between them would swallow DNS and lock every client out of the portal.
func isolirBlockPosition(rules []rosRule, blockComment string, afterComments ...string) (blockID, destination string, needMove bool) {
	isAccept := func(c string) bool {
		for _, a := range afterComments {
			if c == a || c == isolirLegacyComment(a) {
				return true
			}
		}
		return false
	}
	blockIdx, lastIdx := -1, -1
	for i, r := range rules {
		c := strings.TrimSpace(r.Comment)
		if c == blockComment || c == isolirLegacyComment(blockComment) {
			blockIdx = i
			blockID = r.ID
		}
		if isAccept(c) {
			lastIdx = i
		}
	}
	if blockIdx < 0 || lastIdx < 0 || blockIdx > lastIdx {
		return blockID, "", false
	}
	if lastIdx+1 < len(rules) {
		return blockID, rules[lastIdx+1].ID, true
	}
	return blockID, "", true
}

// ensureFirewallBlock upserts the isolir drop rule and, if needed, moves it so it
// sits after every accept comment, so DNS and portal access keep precedence.
func ensureFirewallBlock(cl *routeros.Client, props []string, afterComments ...string) error {
	if err := upsertFirewallFilterByComment(cl, isolirRuleCommentBlock, props); err != nil {
		return err
	}
	reply, err := cl.Run("/ip/firewall/filter/print")
	if err != nil {
		return err
	}
	rules := make([]rosRule, 0, len(reply.Re))
	for _, re := range reply.Re {
		rules = append(rules, rosRule{ID: re.Map[".id"], Comment: re.Map["comment"]})
	}
	blockID, dest, needMove := isolirBlockPosition(rules, isolirRuleCommentBlock, afterComments...)
	if !needMove || blockID == "" {
		return nil
	}
	args := []string{"/ip/firewall/filter/move", "=numbers=" + blockID}
	if dest != "" {
		args = append(args, "=destination="+dest)
	}
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

// upsertFirewallNATByComments upserts a NAT rule matched by its current comment
// or any of the legacy comments (updating a legacy row in place migrates it).
func upsertFirewallNATByComments(cl *routeros.Client, comment string, legacies, props []string) error {
	return upsertByCommentWithLegacies(cl, "/ip/firewall/nat", comment, legacies, props)
}

// deleteByComment removes every row under basePath whose comment matches any of
// the given comments. Blank comments are ignored. Best-effort cleanup helper.
func deleteByComment(cl *routeros.Client, basePath string, comments ...string) error {
	want := make(map[string]struct{}, len(comments))
	for _, c := range comments {
		if c = strings.TrimSpace(c); c != "" {
			want[c] = struct{}{}
		}
	}
	if len(want) == 0 {
		return nil
	}
	reply, err := cl.Run(basePath + "/print")
	if err != nil || reply == nil {
		return err
	}
	var last error
	for _, re := range reply.Re {
		id := re.Map[".id"]
		if id == "" {
			continue
		}
		if _, ok := want[strings.TrimSpace(re.Map["comment"])]; !ok {
			continue
		}
		if _, err := cl.Run(basePath+"/remove", "=numbers="+id); err != nil {
			last = err
		}
	}
	return last
}
