package routeros

import (
	"fmt"
	"net"
	"strings"
)

// HostIP strips an optional CIDR/prefix from a PostgreSQL inet text value
// (e.g. "10.10.0.5/32" → "10.10.0.5"). MikroTik rejects /prefix on remote-address.
func HostIP(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if host, _, err := net.ParseCIDR(s); err == nil {
		if v4 := host.To4(); v4 != nil {
			return v4.String()
		}
		return host.String()
	}
	if ip := net.ParseIP(s); ip != nil {
		if v4 := ip.To4(); v4 != nil {
			return v4.String()
		}
		return ip.String()
	}
	// fallback: strip after /
	if i := strings.IndexByte(s, '/'); i > 0 {
		return strings.TrimSpace(s[:i])
	}
	return s
}

// CIDRToPoolRanges converts a CIDR (e.g. 10.10.0.0/24) into a MikroTik pool ranges
// string (e.g. 10.10.0.2-10.10.0.254), excluding network, broadcast, and optional gateway.
func CIDRToPoolRanges(network string, gateway *string) (string, error) {
	network = strings.TrimSpace(network)
	_, ipNet, err := net.ParseCIDR(network)
	if err != nil {
		// allow bare IP/mask already as range?
		if strings.Contains(network, "-") {
			return network, nil
		}
		return "", fmt.Errorf("network CIDR tidak valid: %w", err)
	}
	ones, bits := ipNet.Mask.Size()
	if bits != 32 {
		return "", fmt.Errorf("hanya IPv4 yang didukung untuk IP pool")
	}
	if ones >= 31 {
		// /31 /32 — use the single usable address(es) as-is
		ip := ipNet.IP.To4()
		if ip == nil {
			return "", fmt.Errorf("bukan IPv4")
		}
		return ip.String(), nil
	}

	ip := ipNet.IP.To4()
	if ip == nil {
		return "", fmt.Errorf("bukan IPv4")
	}
	mask := ipNet.Mask
	networkIP := make(net.IP, 4)
	broadcast := make(net.IP, 4)
	for i := 0; i < 4; i++ {
		networkIP[i] = ip[i] & mask[i]
		broadcast[i] = ip[i] | ^mask[i]
	}

	start := dupIP(networkIP)
	incIP(start) // skip network (.0 → .1)
	end := dupIP(broadcast)
	decIP(end) // skip broadcast

	var gw net.IP
	if gateway != nil {
		if h := HostIP(*gateway); h != "" {
			if g := net.ParseIP(h); g != nil {
				gw = g.To4()
			}
		}
	}

	// If gateway == first host (.1), start after gateway (.2); same for last host.
	if gw != nil && ipNet.Contains(gw) {
		if gw.Equal(start) {
			incIP(start)
		}
		if gw.Equal(end) {
			decIP(end)
		}
	}

	if compareIP(start, end) > 0 {
		return "", fmt.Errorf("rentang pool kosong setelah exclude network/broadcast/gateway")
	}
	if start.Equal(end) {
		return start.String(), nil
	}
	return start.String() + "-" + end.String(), nil
}

func dupIP(ip net.IP) net.IP {
	out := make(net.IP, len(ip))
	copy(out, ip)
	return out
}

func incIP(ip net.IP) {
	for i := len(ip) - 1; i >= 0; i-- {
		ip[i]++
		if ip[i] != 0 {
			return
		}
	}
}

func decIP(ip net.IP) {
	for i := len(ip) - 1; i >= 0; i-- {
		if ip[i] > 0 {
			ip[i]--
			return
		}
		ip[i] = 0xff
	}
}

func compareIP(a, b net.IP) int {
	a4, b4 := a.To4(), b.To4()
	for i := 0; i < 4; i++ {
		if a4[i] < b4[i] {
			return -1
		}
		if a4[i] > b4[i] {
			return 1
		}
	}
	return 0
}
