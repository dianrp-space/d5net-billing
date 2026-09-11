package routeros

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dianrp-space/d5net-billing/internal/auth"
	"github.com/dianrp-space/d5net-billing/internal/provision"
	"github.com/dianrp-space/d5net-billing/internal/store"
	"github.com/dianrp-space/d5net-billing/internal/xid"
	"github.com/go-routeros/routeros/v3"
)

const connCacheTTL = 30 * time.Second

type cachedConn struct {
	client  *routeros.Client
	expires time.Time
}

type Client struct {
	store     *store.Store
	encryptor *auth.Encryptor
	connPool  sync.Map // legacy; unused for dial
	connCache sync.Map // routerID -> *cachedConn
	breakers  sync.Map
}

func New(st *store.Store, enc *auth.Encryptor) *Client {
	return &Client{store: st, encryptor: enc}
}

func (c *Client) Capabilities() provision.Caps {
	return provision.Caps{PPPoE: true, Hotspot: true, DHCP: true, Queue: true, Backup: true}
}

func (c *Client) dial(ctx context.Context, tenantID xid.ID, routerID xid.ID) (*routeros.Client, *store.Router, error) {
	if !c.allow(routerID) {
		return nil, nil, fmt.Errorf("circuit open for router %d", routerID)
	}
	r, err := c.store.GetRouter(ctx, tenantID, routerID)
	if err != nil {
		return nil, nil, err
	}
	if !r.IsActive {
		return nil, nil, fmt.Errorf("router %s is inactive", r.Name)
	}

	if v, ok := c.connCache.Load(routerID); ok {
		cc := v.(*cachedConn)
		if time.Now().Before(cc.expires) && cc.client != nil {
			return cc.client, r, nil
		}
		if cc.client != nil {
			cc.client.Close()
		}
		c.connCache.Delete(routerID)
	}

	password, err := c.encryptor.Decrypt(r.PasswordEnc)
	if err != nil {
		return nil, nil, fmt.Errorf("decrypt password: %w", err)
	}
	addr := net.JoinHostPort(r.Address, strconv.Itoa(r.Port))
	var client *routeros.Client
	if r.UseTLS {
		client, err = routeros.DialTLSContext(ctx, addr, r.Username, password, &tls.Config{InsecureSkipVerify: true})
	} else {
		client, err = routeros.DialContext(ctx, addr, r.Username, password)
	}
	if err != nil {
		c.fail(routerID)
		msg := err.Error()
		_ = c.store.UpdateRouterStatus(ctx, tenantID, routerID, nil, &msg)
		return nil, nil, fmt.Errorf("dial router %s (%s): %w", r.Name, addr, err)
	}
	c.success(routerID)
	now := time.Now()
	_ = c.store.UpdateRouterStatus(ctx, tenantID, routerID, &now, nil)
	c.connCache.Store(routerID, &cachedConn{client: client, expires: now.Add(connCacheTTL)})
	return client, r, nil
}

func (c *Client) run(ctx context.Context, tenantID xid.ID, routerID xid.ID, userID *xid.ID, cmd string, fn func(*routeros.Client) (*routeros.Reply, error)) error {
	client, r, err := c.dial(ctx, tenantID, routerID)
	if err != nil {
		return err
	}
	// Connection may be cached (30s TTL); do not Close here.

	reply, err := fn(client)
	result := "ok"
	success := true
	if err != nil {
		result = err.Error()
		success = false
	} else if reply != nil && len(reply.Re) > 0 {
		result = fmt.Sprintf("%v", reply.Re)
	}
	_ = c.store.LogRouterCommand(ctx, tenantID, routerID, userID, cmd, result, success)
	if err != nil {
		return fmt.Errorf("router %s: %w", r.Name, err)
	}
	return nil
}

func commentTag(spec *provision.ServiceSpec) string {
	if spec.Comment != "" {
		return spec.Comment
	}
	if !xid.IsNil(spec.SubscriptionID) {
		return "d5n:" + spec.SubscriptionID.String()
	}
	return "d5n"
}

// brandComment is the RouterOS comment for tenant-owned objects (profiles, pools).
func (c *Client) brandComment(ctx context.Context, tenantID xid.ID) string {
	app, err := c.store.EffectiveAppName(ctx, tenantID)
	if err != nil || strings.TrimSpace(app) == "" {
		app = "D5Net"
	}
	return provision.SanitizeBrandPrefix(app)
}

func mbpsLimit(n int) string {
	if n <= 0 {
		return "0"
	}
	return strconv.Itoa(n) + "M"
}

func (c *Client) Apply(ctx context.Context, spec *provision.ServiceSpec) error {
	var err error
	switch spec.ServiceType {
	case "pppoe":
		err = c.applyPPPoE(ctx, spec)
	case "hotspot":
		err = c.applyHotspot(ctx, spec)
	case "dhcp":
		err = c.applyDHCP(ctx, spec)
	default:
		err = c.applyPPPoE(ctx, spec)
	}
	if err != nil {
		return err
	}
	if spec.DownloadMbps > 0 {
		_ = c.applySimpleQueue(ctx, spec) // best-effort; ignore errors
	}
	return nil
}

func (c *Client) applyPPPoE(ctx context.Context, spec *provision.ServiceSpec) error {
	comment := commentTag(spec)
	return c.run(ctx, spec.TenantID, spec.RouterID, nil, "/ppp/secret/add", func(cl *routeros.Client) (*routeros.Reply, error) {
		_, _ = cl.Run("/ppp/secret/remove", "=numbers="+spec.Username)
		args := []string{
			"/ppp/secret/add",
			"=name=" + spec.Username,
			"=password=" + spec.Password,
			"=service=pppoe",
			"=comment=" + comment,
		}
		if spec.ProfileName != "" {
			args = append(args, "=profile="+spec.ProfileName)
		}
		if ip := HostIP(spec.IPAddress); ip != "" {
			args = append(args, "=remote-address="+ip)
		}
		if local := HostIP(spec.LocalAddress); local != "" {
			args = append(args, "=local-address="+local)
		}
		return cl.Run(args...)
	})
}

func (c *Client) applyHotspot(ctx context.Context, spec *provision.ServiceSpec) error {
	comment := commentTag(spec)
	return c.run(ctx, spec.TenantID, spec.RouterID, nil, "/ip/hotspot/user/add", func(cl *routeros.Client) (*routeros.Reply, error) {
		_, _ = cl.Run("/ip/hotspot/user/remove", "=numbers="+spec.Username)
		args := []string{
			"/ip/hotspot/user/add",
			"=name=" + spec.Username,
			"=password=" + spec.Password,
			"=comment=" + comment,
		}
		if spec.ProfileName != "" {
			args = append(args, "=profile="+spec.ProfileName)
		}
		if spec.LimitBytesTotal > 0 {
			args = append(args, "=limit-bytes-total="+fmt.Sprintf("%d", spec.LimitBytesTotal))
		}
		if u := strings.TrimSpace(spec.LimitUptime); u != "" {
			args = append(args, "=limit-uptime="+u)
		}
		if spec.SharedUsers > 0 {
			args = append(args, "=shared-users="+fmt.Sprintf("%d", spec.SharedUsers))
		}
		return cl.Run(args...)
	})
}

func (c *Client) applyDHCP(ctx context.Context, spec *provision.ServiceSpec) error {
	comment := commentTag(spec)
	return c.run(ctx, spec.TenantID, spec.RouterID, nil, "/ip/dhcp-server/lease/add", func(cl *routeros.Client) (*routeros.Reply, error) {
		if spec.IPAddress != "" {
			args := []string{
				"/ip/dhcp-server/lease/add",
				"=address=" + spec.IPAddress,
				"=comment=" + comment,
			}
			if spec.MACAddress != "" {
				args = append(args, "=mac-address="+spec.MACAddress)
			}
			reply, err := cl.Run(args...)
			if err == nil {
				return reply, nil
			}
			// Fallback: address-list with ownership comment.
			// Pre-remove by address so legacy drp-dhcp entries migrate without dupes.
			_, _ = cl.Run("/ip/firewall/address-list/remove", "=numbers="+spec.IPAddress)
			listArgs := []string{
				"/ip/firewall/address-list/add",
				"=list=d5n-dhcp",
				"=address=" + spec.IPAddress,
				"=comment=" + comment,
			}
			return cl.Run(listArgs...)
		}
		if spec.MACAddress != "" {
			_, _ = cl.Run("/ip/firewall/address-list/remove", "=numbers="+spec.MACAddress)
			return cl.Run(
				"/ip/firewall/address-list/add",
				"=list=d5n-dhcp",
				"=address="+spec.MACAddress,
				"=comment="+comment,
			)
		}
		return nil, fmt.Errorf("dhcp apply requires IPAddress or MACAddress")
	})
}

func (c *Client) applySimpleQueue(ctx context.Context, spec *provision.ServiceSpec) error {
	name := "d5n-" + spec.Username
	up := spec.UploadMbps
	if up <= 0 {
		up = spec.DownloadMbps
	}
	maxLimit := mbpsLimit(up) + "/" + mbpsLimit(spec.DownloadMbps)
	return c.run(ctx, spec.TenantID, spec.RouterID, nil, "/queue/simple/add", func(cl *routeros.Client) (*routeros.Reply, error) {
		// Drop the legacy drp- queue first so a rename never leaves duplicates.
		_, _ = cl.Run("/queue/simple/remove", "=numbers=drp-"+spec.Username)
		_, _ = cl.Run("/queue/simple/remove", "=numbers="+name)
		args := []string{
			"/queue/simple/add",
			"=name=" + name,
			"=max-limit=" + maxLimit,
			"=comment=" + commentTag(spec),
		}
		if spec.IPAddress != "" {
			args = append(args, "=target="+spec.IPAddress)
		}
		return cl.Run(args...)
	})
}

func setNamedRow(cl *routeros.Client, printPath, setPath, name string, props []string) (*routeros.Reply, bool, error) {
	reply, err := cl.Run(printPath, "?name="+name)
	if err != nil {
		return nil, false, err
	}
	id := ""
	var row map[string]string
	if reply != nil {
		for _, re := range reply.Re {
			if v := re.Map[".id"]; v != "" {
				id = v
				row = re.Map
				break
			}
		}
	}
	if id == "" {
		return nil, false, fmt.Errorf("%s %q tidak ditemukan", printPath, name)
	}
	if rowMatchesProps(row, props) {
		return reply, false, nil
	}
	args := append([]string{setPath, "=numbers=" + id}, props...)
	reply, err = cl.Run(args...)
	return reply, true, err
}

func lookupNamedID(cl *routeros.Client, printPath, name string) (string, error) {
	reply, err := cl.Run(printPath, "?name="+name, "=.proplist=.id")
	if err != nil {
		return "", err
	}
	if reply != nil {
		for _, re := range reply.Re {
			if v := re.Map[".id"]; v != "" {
				return v, nil
			}
		}
	}
	return "", fmt.Errorf("%s %q tidak ditemukan", printPath, name)
}

func unsetPPPSecretFields(cl *routeros.Client, id string, fields ...string) error {
	var last error
	for _, f := range fields {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		_, err := cl.Run("/ppp/secret/unset", "=numbers="+id, "=value-name="+f)
		if err != nil {
			_, err2 := cl.Run("/ppp/secret/unset", "=.id="+id, "=value-name="+f)
			if err2 != nil {
				last = err
			}
		}
	}
	return last
}

func setPPPSecret(cl *routeros.Client, username string, props []string, unsetFields []string) (*routeros.Reply, bool, error) {
	reply, err := cl.Run("/ppp/secret/print", "?name="+username)
	if err != nil {
		return nil, false, err
	}
	id := ""
	row := map[string]string{}
	if reply != nil {
		for _, re := range reply.Re {
			if v := re.Map[".id"]; v != "" {
				id = v
				row = re.Map
				break
			}
		}
	}
	if id == "" {
		return nil, false, fmt.Errorf("%s %q tidak ditemukan", "/ppp/secret/print", username)
	}
	needSet := !rowMatchesProps(row, props)
	var pendingUnset []string
	for _, f := range unsetFields {
		if fieldFilled(row, f) {
			pendingUnset = append(pendingUnset, f)
		}
	}
	if !needSet && len(pendingUnset) == 0 {
		return reply, false, nil
	}
	if needSet {
		args := append([]string{"/ppp/secret/set", "=numbers=" + id}, props...)
		reply, err = cl.Run(args...)
		if err != nil {
			return nil, false, err
		}
	}
	if err := unsetPPPSecretFields(cl, id, pendingUnset...); err != nil {
		return reply, true, fmt.Errorf("hapus %s secret: %w", strings.Join(pendingUnset, ","), err)
	}
	return reply, true, nil
}

func (c *Client) Suspend(ctx context.Context, spec *provision.ServiceSpec) error {
	profile := spec.IsolirProfile
	if profile == "" {
		profile = "isolir"
	}
	comment := provision.WithIsolirComment(commentTag(spec))
	switch spec.ServiceType {
	case "hotspot":
		changed := false
		err := c.run(ctx, spec.TenantID, spec.RouterID, nil, "/ip/hotspot/user/set", func(cl *routeros.Client) (*routeros.Reply, error) {
			reply, did, err := setNamedRow(cl, "/ip/hotspot/user/print", "/ip/hotspot/user/set", spec.Username, []string{
				"=profile=" + profile, "=disabled=no", "=comment=" + comment,
			})
			changed = did
			return reply, err
		})
		if err != nil {
			return err
		}
		if !changed {
			return nil
		}
		// Best-effort kick hotspot active session
		_ = c.run(ctx, spec.TenantID, spec.RouterID, nil, "/ip/hotspot/active/remove", func(cl *routeros.Client) (*routeros.Reply, error) {
			reply, err := cl.Run("/ip/hotspot/active/print", "?user="+spec.Username)
			if err != nil {
				return nil, err
			}
			for _, re := range reply.Re {
				if id := re.Map[".id"]; id != "" {
					_, _ = cl.Run("/ip/hotspot/active/remove", "=numbers="+id)
				}
			}
			return reply, nil
		})
		return nil
	default:
		changed := false
		err := c.run(ctx, spec.TenantID, spec.RouterID, nil, "/ppp/secret/set", func(cl *routeros.Client) (*routeros.Reply, error) {
			// Secret remote-address must be an IP (or empty). Pool names belong on the PPP profile.
			props := []string{"=profile=" + profile, "=disabled=no", "=comment=" + comment}
			unset := []string{"remote-address"}
			if local := HostIP(spec.LocalAddress); local != "" {
				props = append(props, "=local-address="+local)
			} else {
				unset = append(unset, "local-address")
			}
			reply, did, err := setPPPSecret(cl, spec.Username, props, unset)
			changed = did
			return reply, err
		})
		if err != nil {
			return err
		}
		if changed {
			_ = c.Disconnect(ctx, spec)
		}
		return nil
	}
}

func (c *Client) Resume(ctx context.Context, spec *provision.ServiceSpec) error {
	comment := provision.WithoutIsolirComment(commentTag(spec))
	switch spec.ServiceType {
	case "hotspot":
		changed := false
		err := c.run(ctx, spec.TenantID, spec.RouterID, nil, "/ip/hotspot/user/set", func(cl *routeros.Client) (*routeros.Reply, error) {
			props := []string{"=disabled=no", "=comment=" + comment}
			if spec.ProfileName != "" {
				props = append(props, "=profile="+spec.ProfileName)
			}
			reply, did, err := setNamedRow(cl, "/ip/hotspot/user/print", "/ip/hotspot/user/set", spec.Username, props)
			changed = did
			return reply, err
		})
		if err != nil {
			return err
		}
		if !changed {
			return nil
		}
		_ = c.run(ctx, spec.TenantID, spec.RouterID, nil, "/ip/hotspot/active/remove", func(cl *routeros.Client) (*routeros.Reply, error) {
			reply, err := cl.Run("/ip/hotspot/active/print", "?user="+spec.Username)
			if err != nil {
				return nil, err
			}
			for _, re := range reply.Re {
				if id := re.Map[".id"]; id != "" {
					_, _ = cl.Run("/ip/hotspot/active/remove", "=numbers="+id)
				}
			}
			return reply, nil
		})
		return nil
	default:
		changed := false
		err := c.run(ctx, spec.TenantID, spec.RouterID, nil, "/ppp/secret/set", func(cl *routeros.Client) (*routeros.Reply, error) {
			props := []string{"=disabled=no", "=comment=" + comment}
			unset := []string{}
			if spec.ProfileName != "" {
				props = append(props, "=profile="+spec.ProfileName)
			}
			if ip := HostIP(spec.IPAddress); ip != "" {
				props = append(props, "=remote-address="+ip)
			} else {
				unset = append(unset, "remote-address")
			}
			if local := HostIP(spec.LocalAddress); local != "" {
				props = append(props, "=local-address="+local)
			} else {
				unset = append(unset, "local-address")
			}
			reply, did, err := setPPPSecret(cl, spec.Username, props, unset)
			changed = did
			return reply, err
		})
		if err != nil {
			return err
		}
		if changed {
			_ = c.Disconnect(ctx, spec)
		}
		return nil
	}
}

func (c *Client) Remove(ctx context.Context, spec *provision.ServiceSpec) error {
	path := "/ppp/secret/remove"
	if spec.ServiceType == "hotspot" {
		path = "/ip/hotspot/user/remove"
	} else if spec.ServiceType == "dhcp" {
		return c.run(ctx, spec.TenantID, spec.RouterID, nil, "/ip/dhcp-server/lease/remove", func(cl *routeros.Client) (*routeros.Reply, error) {
			if spec.IPAddress != "" {
				return cl.Run("/ip/firewall/address-list/remove", "=numbers="+spec.IPAddress)
			}
			return nil, nil
		})
	}
	return c.run(ctx, spec.TenantID, spec.RouterID, nil, path, func(cl *routeros.Client) (*routeros.Reply, error) {
		return cl.Run(path, "=numbers="+spec.Username)
	})
}

func (c *Client) GetServiceSecret(ctx context.Context, tenantID, routerID xid.ID, username, serviceType string) (*provision.SecretState, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return nil, fmt.Errorf("username wajib")
	}
	var found *provision.SecretState
	path := "/ppp/secret/print"
	if strings.EqualFold(strings.TrimSpace(serviceType), "hotspot") {
		path = "/ip/hotspot/user/print"
	}
	err := c.run(ctx, tenantID, routerID, nil, path, func(cl *routeros.Client) (*routeros.Reply, error) {
		reply, err := cl.Run(path, "?name="+username)
		if err != nil {
			return nil, err
		}
		for _, re := range reply.Re {
			name := strings.TrimSpace(re.Map["name"])
			if name == "" {
				continue
			}
			st := "pppoe"
			if path == "/ip/hotspot/user/print" {
				st = "hotspot"
			}
			sec := provision.SecretState{
				Username:    name,
				Profile:     re.Map["profile"],
				Comment:     re.Map["comment"],
				ServiceType: st,
			}
			found = &sec
			break
		}
		return reply, nil
	})
	if err != nil {
		return nil, err
	}
	return found, nil
}

func (c *Client) ListServiceSecrets(ctx context.Context, tenantID, routerID xid.ID) ([]provision.SecretState, error) {
	var out []provision.SecretState
	err := c.run(ctx, tenantID, routerID, nil, "/ppp/secret/print", func(cl *routeros.Client) (*routeros.Reply, error) {
		reply, err := cl.Run("/ppp/secret/print")
		if err != nil {
			return nil, err
		}
		for _, re := range reply.Re {
			name := strings.TrimSpace(re.Map["name"])
			if name == "" {
				continue
			}
			out = append(out, provision.SecretState{
				Username:    name,
				Profile:     re.Map["profile"],
				Comment:     re.Map["comment"],
				ServiceType: "pppoe",
			})
		}
		hs, hsErr := cl.Run("/ip/hotspot/user/print")
		if hsErr == nil {
			for _, re := range hs.Re {
				name := strings.TrimSpace(re.Map["name"])
				if name == "" {
					continue
				}
				out = append(out, provision.SecretState{
					Username:    name,
					Profile:     re.Map["profile"],
					Comment:     re.Map["comment"],
					ServiceType: "hotspot",
				})
			}
		}
		return reply, nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) Disconnect(ctx context.Context, spec *provision.ServiceSpec) error {
	return c.run(ctx, spec.TenantID, spec.RouterID, nil, "/ppp/active/remove", func(cl *routeros.Client) (*routeros.Reply, error) {
		reply, err := cl.Run("/ppp/active/print", "?name="+spec.Username)
		if err != nil {
			return nil, err
		}
		for _, re := range reply.Re {
			if id, ok := re.Map[".id"]; ok {
				if _, err := cl.Run("/ppp/active/remove", "=numbers="+id); err != nil {
					return nil, err
				}
			}
		}
		return reply, nil
	})
}

func (c *Client) ActiveSessions(ctx context.Context, routerID xid.ID) ([]provision.Session, error) {
	var tenantID xid.ID
	err := c.store.Pool.QueryRow(ctx, `SELECT tenant_id FROM routers WHERE id = $1`, routerID).Scan(&tenantID)
	if err != nil {
		return nil, err
	}
	client, _, err := c.dial(ctx, tenantID, routerID)
	if err != nil {
		return nil, err
	}

	reply, err := client.Run("/ppp/active/print")
	if err != nil {
		return nil, err
	}
	var sessions []provision.Session
	for _, re := range reply.Re {
		s := provision.Session{
			Username:  re.Map["name"],
			IPAddress: re.Map["address"],
			StartedAt: time.Now(),
		}
		sessions = append(sessions, s)
	}
	return sessions, nil
}

type ResourceMetrics struct {
	CPULoad      float64
	FreeMemory   int64
	TotalMemory  int64
	MemoryUsed   int64
	UptimeSecs   int64
	UptimeString string
}

// CollectResourceMetrics dials the router and runs /system/resource/print.
func (c *Client) CollectResourceMetrics(ctx context.Context, routerID xid.ID) (*ResourceMetrics, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	var tenantID xid.ID
	if err := c.store.Pool.QueryRow(ctx, `SELECT tenant_id FROM routers WHERE id = $1`, routerID).Scan(&tenantID); err != nil {
		return nil, err
	}
	client, _, err := c.dial(ctx, tenantID, routerID)
	if err != nil {
		return nil, err
	}

	reply, err := client.Run("/system/resource/print")
	if err != nil {
		return nil, err
	}
	if len(reply.Re) == 0 {
		return nil, fmt.Errorf("empty resource reply")
	}
	m := reply.Re[0].Map
	cpu := parseFloatLoose(m["cpu-load"])
	freeMem := parseIntLoose(m["free-memory"])
	totalMem := parseIntLoose(m["total-memory"])
	uptimeStr := m["uptime"]
	metrics := &ResourceMetrics{
		CPULoad:      cpu,
		FreeMemory:   freeMem,
		TotalMemory:  totalMem,
		MemoryUsed:   totalMem - freeMem,
		UptimeSecs:   parseUptimeSecs(uptimeStr),
		UptimeString: uptimeStr,
	}
	if metrics.MemoryUsed < 0 {
		metrics.MemoryUsed = 0
	}
	_ = c.store.LogRouterCommand(ctx, tenantID, routerID, nil, "/system/resource/print",
		fmt.Sprintf("cpu=%.1f mem_used=%d uptime=%s", metrics.CPULoad, metrics.MemoryUsed, uptimeStr), true)
	return metrics, nil
}

func parseFloatLoose(s string) float64 {
	if s == "" {
		return 0
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return f
}

func parseIntLoose(s string) int64 {
	if s == "" {
		return 0
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0
	}
	return n
}

// parseUptimeSecs loosely parses RouterOS uptime like "1w2d3h4m5s" or "5d3h20m15s".
func parseUptimeSecs(s string) int64 {
	if s == "" {
		return 0
	}
	var total int64
	var num strings.Builder
	flush := func(unit byte) {
		if num.Len() == 0 {
			return
		}
		n, _ := strconv.ParseInt(num.String(), 10, 64)
		num.Reset()
		switch unit {
		case 'w':
			total += n * 7 * 24 * 3600
		case 'd':
			total += n * 24 * 3600
		case 'h':
			total += n * 3600
		case 'm':
			total += n * 60
		case 's':
			total += n
		}
	}
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch >= '0' && ch <= '9' {
			num.WriteByte(ch)
			continue
		}
		flush(ch)
	}
	return total
}

func (c *Client) TestConnection(ctx context.Context, routerID xid.ID) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	var tenantID xid.ID
	err := c.store.Pool.QueryRow(ctx, `SELECT tenant_id FROM routers WHERE id = $1`, routerID).Scan(&tenantID)
	if err != nil {
		return "", err
	}

	client, r, err := c.dial(ctx, tenantID, routerID)
	if err != nil {
		return "", err
	}

	identity := r.Name
	board, version := "", ""
	if reply, err := client.Run("/system/identity/print"); err == nil && len(reply.Re) > 0 {
		if n := reply.Re[0].Map["name"]; n != "" {
			identity = n
		}
	}
	if reply, err := client.Run("/system/resource/print"); err == nil && len(reply.Re) > 0 {
		board = reply.Re[0].Map["board-name"]
		version = reply.Re[0].Map["version"]
	}
	_ = c.store.LogRouterCommand(ctx, tenantID, routerID, nil, "/system/identity+resource/print",
		fmt.Sprintf("identity=%s board=%s version=%s", identity, board, version), true)

	msg := fmt.Sprintf("terhubung: %s", identity)
	if board != "" {
		msg += " (" + board
		if version != "" {
			msg += ", " + version
		}
		msg += ")"
	}
	return msg, nil
}

func (c *Client) BackupConfig(ctx context.Context, routerID xid.ID) (string, error) {
	var tenantID xid.ID
	err := c.store.Pool.QueryRow(ctx, `SELECT tenant_id FROM routers WHERE id = $1`, routerID).Scan(&tenantID)
	if err != nil {
		return "", err
	}
	client, _, err := c.dial(ctx, tenantID, routerID)
	if err != nil {
		return "", err
	}

	reply, err := client.Run("/export", "=show-sensitive")
	if err != nil {
		return "", err
	}
	var content string
	for _, re := range reply.Re {
		for k, v := range re.Map {
			if k != ".id" {
				content += fmt.Sprintf("%s=%s\n", k, v)
			}
		}
	}
	slog.Info("router backup exported", "router_id", routerID, "size", len(content))
	return content, nil
}
