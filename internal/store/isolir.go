package store

import (
	"context"
	"errors"
	"strings"

	"github.com/dianrp-space/d5net-billing/internal/xid"
)

const (
	IsolirNetworkSettingKey = "isolir.network"
	IsolirHTMLSettingKey    = "isolir.captive_html"
)

// IsolirNetworkSettings controls RouterOS isolir pool/profile/web-proxy redirect.
// RouterID + IPPoolID bind the config to a specific router via IPAM.
type IsolirNetworkSettings struct {
	ProfileName string  `json:"profile_name"`
	RouterID    *xid.ID `json:"router_id,omitempty"`
	IPPoolID    *xid.ID `json:"ip_pool_id,omitempty"`
	// Derived / legacy fields kept for RouterOS sync & older saved settings.
	PoolName      string `json:"pool_name,omitempty"`
	PoolRanges    string `json:"pool_ranges,omitempty"`
	PoolGateway   string `json:"pool_gateway,omitempty"`
	PortalBaseURL string `json:"portal_base_url"`
	RedirectMode  string `json:"redirect_mode,omitempty"` // always web-proxy; kept for compat
}

func DefaultIsolirNetworkSettings() IsolirNetworkSettings {
	return IsolirNetworkSettings{
		ProfileName:  "isolir",
		RedirectMode: "web-proxy",
	}
}

// IsolirProfileName is the PPP/hotspot profile used when suspending a subscription.
func IsolirProfileName(cfg IsolirNetworkSettings, plan *Plan) string {
	if plan != nil && plan.IsolirProfile != nil {
		if n := strings.TrimSpace(*plan.IsolirProfile); n != "" {
			return n
		}
	}
	if n := strings.TrimSpace(cfg.ProfileName); n != "" {
		return n
	}
	return "isolir"
}

// IsolirPoolName is the RouterOS /ip/pool bound to the isolir profile.
func IsolirPoolName(cfg IsolirNetworkSettings) string {
	if n := strings.TrimSpace(cfg.PoolName); n != "" {
		return n
	}
	return "isolir"
}

// IsolirClientPath is the public isolir portal path used for captive redirect.
// It must be the dedicated isolir page (/isolir), not the regular client login.
func IsolirClientPath(_ string) string {
	return "/isolir"
}

// IsolirPortalURL builds {base}/isolir for Web Proxy redirect.
func IsolirPortalURL(base, slug string) string {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	path := IsolirClientPath(slug)
	if base == "" {
		return path
	}
	return base + path
}

func (s *Store) GetIsolirNetworkSettings(ctx context.Context, tenantID xid.ID) (IsolirNetworkSettings, error) {
	cfg := DefaultIsolirNetworkSettings()
	err := s.GetSettingJSON(ctx, tenantID, IsolirNetworkSettingKey, &cfg)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return cfg, err
	}
	if cfg.ProfileName == "" {
		cfg.ProfileName = "isolir"
	}
	cfg.RedirectMode = "web-proxy"
	cfg.PortalBaseURL = strings.TrimRight(strings.TrimSpace(cfg.PortalBaseURL), "/")
	return cfg, nil
}

func (s *Store) UpsertIsolirNetworkSettings(ctx context.Context, tenantID xid.ID, cfg IsolirNetworkSettings) error {
	if cfg.ProfileName == "" {
		cfg.ProfileName = "isolir"
	}
	cfg.RedirectMode = "web-proxy"
	cfg.PortalBaseURL = strings.TrimRight(strings.TrimSpace(cfg.PortalBaseURL), "/")
	return s.UpsertSettingJSON(ctx, tenantID, IsolirNetworkSettingKey, cfg)
}

// ResolveIsolirPool fills PoolName/PoolRanges from the linked IPAM pool (and validates router).
func (s *Store) ResolveIsolirPool(ctx context.Context, tenantID xid.ID, cfg *IsolirNetworkSettings) error {
	if cfg == nil {
		return nil
	}
	if cfg.IPPoolID == nil || xid.IsNil(*cfg.IPPoolID) {
		return nil
	}
	pool, err := s.GetIPPool(ctx, tenantID, *cfg.IPPoolID)
	if err != nil {
		return err
	}
	cfg.PoolName = pool.Name
	if pool.RouterID != nil && !xid.IsNil(*pool.RouterID) {
		cfg.RouterID = pool.RouterID
	}
	// Prefer stored network CIDR; EnsureIsolirInfra converts CIDR → ranges when needed.
	if pool.Network != "" {
		cfg.PoolRanges = pool.Network
	}
	if pool.Gateway != nil {
		if g := strings.TrimSpace(*pool.Gateway); g != "" {
			cfg.PoolGateway = g
		}
	}
	return nil
}

func (s *Store) GetIsolirHTML(ctx context.Context, tenantID xid.ID) (string, error) {
	var raw struct {
		HTML string `json:"html"`
	}
	err := s.GetSettingJSON(ctx, tenantID, IsolirHTMLSettingKey, &raw)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return "", err
	}
	return raw.HTML, nil
}

func (s *Store) UpsertIsolirHTML(ctx context.Context, tenantID xid.ID, html string) error {
	return s.UpsertSettingJSON(ctx, tenantID, IsolirHTMLSettingKey, map[string]string{"html": html})
}

// DefaultIsolirHTML is the built-in captive landing (login CTA).
func DefaultIsolirHTML(appName, logoURL, loginURL string) string {
	if appName == "" {
		appName = "Isolir"
	}
	logo := ""
	if logoURL != "" {
		logo = `<img src="` + logoURL + `" alt="` + appName + `" style="max-height:48px;margin-bottom:1rem"/>`
	}
	return `<!DOCTYPE html>
<html lang="id"><head><meta charset="utf-8"/><meta name="viewport" content="width=device-width,initial-scale=1"/>
<title>` + appName + ` — Isolir</title>
<style>
body{margin:0;font-family:system-ui,sans-serif;background:#F7F6F2;color:#1a1a14;min-height:100vh;display:flex;align-items:center;justify-content:center;padding:1.5rem}
.card{background:#fff;border:1px solid #e5e2d9;border-radius:12px;padding:2rem;max-width:420px;width:100%;box-shadow:0 8px 24px rgba(0,0,0,.06);text-align:center}
h1{font-size:1.35rem;margin:0 0 .5rem;color:#5A5A40}
p{color:#5c584c;line-height:1.5;margin:0 0 1.25rem}
a.btn{display:inline-block;background:#5A5A40;color:#fff;text-decoration:none;padding:.7rem 1.25rem;border-radius:8px;font-weight:600}
</style></head><body><div class="card">` + logo + `
<h1>Layanan diisolir</h1>
<p>Internet Anda dibatasi karena ada tagihan yang belum lunas. Silakan masuk untuk melihat tagihan dan membayar.</p>
<a class="btn" href="` + loginURL + `">Login &amp; bayar tagihan</a>
</div></body></html>`
}
