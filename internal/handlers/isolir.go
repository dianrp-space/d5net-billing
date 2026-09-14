package handlers

import (
	"context"
	"html"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/dianrp-space/d5net-billing/internal/httpx"
	"github.com/dianrp-space/d5net-billing/internal/provision"
	"github.com/dianrp-space/d5net-billing/internal/store"
	"github.com/dianrp-space/d5net-billing/internal/xid"
)

func registerIsolirSettings(api huma.API, d *Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "get-isolir-settings", Method: http.MethodGet, Path: "/api/settings/isolir",
		Tags: []string{"Settings"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, _ *struct{}) (*struct {
		Body struct {
			Network   store.IsolirNetworkSettings `json:"network"`
			HTML      string                      `json:"html"`
			IsolirURL string                      `json:"isolir_url"`
			DocsHint  string                      `json:"docs_hint"`
		}
	}, error) {
		tid, err := requireSettings(ctx, d)
		if err != nil {
			return nil, err
		}
		net, err := d.Store.GetIsolirNetworkSettings(ctx, tid)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		_ = d.Store.ResolveIsolirPool(ctx, tid, &net)
		htmlBody, _ := d.Store.GetIsolirHTML(ctx, tid)
		isolirURL := store.IsolirLandingURL(net.PortalBaseURL)
		out := &struct {
			Body struct {
				Network   store.IsolirNetworkSettings `json:"network"`
				HTML      string                      `json:"html"`
				IsolirURL string                      `json:"isolir_url"`
				DocsHint  string                      `json:"docs_hint"`
			}
		}{}
		out.Body.Network = net
		out.Body.HTML = htmlBody
		out.Body.IsolirURL = isolirURL
		out.Body.DocsHint = isolirDocsHint(isolirURL, net)
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "put-isolir-settings", Method: http.MethodPut, Path: "/api/settings/isolir",
		Tags: []string{"Settings"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Body struct {
			Network store.IsolirNetworkSettings `json:"network"`
			HTML    *string                     `json:"html,omitempty"`
		}
	}) (*struct {
		Body struct {
			Network   store.IsolirNetworkSettings `json:"network"`
			HTML      string                      `json:"html"`
			IsolirURL string                      `json:"isolir_url"`
			DocsHint  string                      `json:"docs_hint"`
		}
	}, error) {
		tid, err := requireSettings(ctx, d)
		if err != nil {
			return nil, err
		}
		net := input.Body.Network
		net.RedirectMode = store.IsolirRedirectDSTNAT
		if strings.TrimSpace(net.IsolirHostPort) == "" {
			if d.Config != nil {
				if p := d.Config.IsolirHTTPPort(); p != "" {
					net.IsolirHostPort = p
				}
			}
			if strings.TrimSpace(net.IsolirHostPort) == "" {
				net.IsolirHostPort = store.DefaultIsolirHostPort
			}
		}
		if err := d.Store.ResolveIsolirPool(ctx, tid, &net); err != nil {
			return nil, httpx.BadRequest("IP pool isolir tidak valid: " + err.Error())
		}
		if net.RouterID == nil || xid.IsNil(*net.RouterID) {
			return nil, httpx.BadRequest("pilih router isolir")
		}
		if net.IPPoolID == nil || xid.IsNil(*net.IPPoolID) {
			return nil, httpx.BadRequest("pilih IP pool isolir")
		}
		if strings.TrimSpace(net.PortalBaseURL) == "" {
			return nil, httpx.BadRequest("portal_base_url wajib")
		}
		if err := d.Store.UpsertIsolirNetworkSettings(ctx, tid, net); err != nil {
			return nil, httpx.Internal(err)
		}
		if input.Body.HTML != nil {
			if err := d.Store.UpsertIsolirHTML(ctx, tid, *input.Body.HTML); err != nil {
				return nil, httpx.Internal(err)
			}
		}
		net, _ = d.Store.GetIsolirNetworkSettings(ctx, tid)
		_ = d.Store.ResolveIsolirPool(ctx, tid, &net)
		htmlBody, _ := d.Store.GetIsolirHTML(ctx, tid)
		isolirURL := store.IsolirLandingURL(net.PortalBaseURL)
		out := &struct {
			Body struct {
				Network   store.IsolirNetworkSettings `json:"network"`
				HTML      string                      `json:"html"`
				IsolirURL string                      `json:"isolir_url"`
				DocsHint  string                      `json:"docs_hint"`
			}
		}{}
		out.Body.Network = net
		out.Body.HTML = htmlBody
		out.Body.IsolirURL = isolirURL
		out.Body.DocsHint = isolirDocsHint(isolirURL, net)
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "sync-isolir-routers", Method: http.MethodPost, Path: "/api/settings/isolir/sync",
		Tags: []string{"Settings"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, _ *struct{}) (*struct {
		Body struct {
			Synced int      `json:"synced"`
			Errors []string `json:"errors"`
		}
	}, error) {
		tid, err := requireSettings(ctx, d)
		if err != nil {
			return nil, err
		}
		net, err := d.Store.GetIsolirNetworkSettings(ctx, tid)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		if err := d.Store.ResolveIsolirPool(ctx, tid, &net); err != nil {
			return nil, httpx.BadRequest("IP pool isolir tidak valid: " + err.Error())
		}
		if net.RouterID == nil || xid.IsNil(*net.RouterID) {
			return nil, httpx.BadRequest("pilih router dulu di pengaturan isolir")
		}
		if net.PoolRanges == "" || net.PortalBaseURL == "" {
			return nil, httpx.BadRequest("pilih IP pool dan isi portal_base_url dulu")
		}
		ten, err := d.Store.GetTenant(ctx, tid)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		r, err := d.Store.GetRouter(ctx, tid, *net.RouterID)
		if err != nil {
			return nil, httpx.BadRequest("router isolir tidak ditemukan")
		}
		synced := 0
		var errs []string
		if !r.IsActive {
			errs = append(errs, r.Name+": router nonaktif")
		} else {
			prov, err := d.Provisioner.Get(r.Provisioner)
			if err != nil {
				errs = append(errs, r.Name+": "+err.Error())
			} else if ensurer, ok := prov.(provision.IsolirEnsurer); !ok {
				errs = append(errs, r.Name+": provisioner tidak mendukung isolir sync")
			} else if err := ensurer.EnsureIsolirInfra(ctx, tid, r.ID, net, ten.Slug); err != nil {
				errs = append(errs, r.Name+": "+err.Error())
			} else {
				synced = 1
			}
		}
		out := &struct {
			Body struct {
				Synced int      `json:"synced"`
				Errors []string `json:"errors"`
			}
		}{}
		out.Body.Synced = synced
		out.Body.Errors = errs
		return out, nil
	})
}

func isolirDocsHint(isolirURL string, net store.IsolirNetworkSettings) string {
	if isolirURL == "" {
		isolirURL = store.IsolirLandingURL("")
	}
	loginURL := store.IsolirPortalURL(net.PortalBaseURL, "")
	pool := net.PoolRanges
	if pool == "" {
		pool = "(pilih IP pool isolir)"
	}
	name := net.PoolName
	if name == "" {
		name = "isolir"
	}
	port := store.IsolirHostPortOrDefault(net)
	override := strings.TrimSpace(net.IsolirHostIP)
	ipHint := "IP dst-nat diambil dari address-list RouterOS (FQDN portal), prefer IP publik"
	if override != "" {
		ipHint = "IP dst-nat override manual: " + override
	}
	return "URL isolir / halaman template:\n" + isolirURL +
		"\nTombol login di template ({{login_url}}) mengarah ke:\n" + loginURL +
		"\n\nPool isolir dipakai saat worker mengisolir langganan di router masing-masing." +
		"\nPool: " + name + " · " + pool +
		"\n\nMode redirect: DST-NAT (tanpa Web Proxy).\n" +
		"Captive listener: ISOLIR_HTTP_ADDR (default 0.0.0.0:" + port + ").\n" +
		"RouterOS DST-NAT: tcp/80 pool isolir → IP portal :" + port + ".\n" +
		"Uji: http://IP_PUBLIK:" + port + "/ (pastikan forward/firewall/CHR/WG).\n" +
		ipHint + "\n" +
		"\nSetting di RouterOS (IP → Firewall):\n" +
		"1. NAT: chain=dstnat, tcp/80 dari pool → action=dst-nat to-addresses=<IP address-list> to-ports=" + port + "\n" +
		"2. Filter: allow DNS; allow portal via address-list FQDN (port 80,443," + port + "); drop trafik lain\n" +
		"3. Urutan filter: block harus setelah SEMUA accept (dns, dns-tcp, portal)\n" +
		"Comment: d5n-isolir:* · Secret isolir: prefix \"ISOLIR \""
}

// renderIsolirPage builds the isolir landing HTML for the single provider,
// applying the admin template + branding. Shared by the /api/public/isolir
// endpoint and the DST-NAT captive listener.
func renderIsolirPage(ctx context.Context, d *Deps) ([]byte, error) {
	ten, err := singleTenant(ctx, d)
	if err != nil {
		return nil, err
	}
	net, _ := d.Store.GetIsolirNetworkSettings(ctx, ten.ID)
	custom, _ := d.Store.GetIsolirHTML(ctx, ten.ID)
	brand, _ := d.Store.ResolveTenantBranding(ctx, ten.ID)
	appName := ten.Name
	logoURL := ""
	if brand != nil {
		if brand.Effective.AppName != "" {
			appName = brand.Effective.AppName
		}
		if brand.Effective.LogoURL != nil {
			logoURL = *brand.Effective.LogoURL
		}
	}
	loginURL := store.IsolirPortalURL(net.PortalBaseURL, ten.Slug)
	body := custom
	if strings.TrimSpace(body) == "" {
		body = store.DefaultIsolirHTML(appName, logoURL, loginURL)
	} else {
		body = strings.ReplaceAll(body, "{{app_name}}", html.EscapeString(appName))
		body = strings.ReplaceAll(body, "{{logo_url}}", html.EscapeString(logoURL))
		body = strings.ReplaceAll(body, "{{login_url}}", html.EscapeString(loginURL))
		body = strings.ReplaceAll(body, "{{tenant_slug}}", html.EscapeString(ten.Slug))
	}
	return []byte(body), nil
}

func registerPublicIsolir(api huma.API, d *Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "public-isolir-page", Method: http.MethodGet, Path: "/api/public/isolir",
		Tags: []string{"Public"},
	}, func(ctx context.Context, _ *struct{}) (*huma.StreamResponse, error) {
		body, err := renderIsolirPage(ctx, d)
		if err != nil {
			return nil, err
		}
		return &huma.StreamResponse{Body: func(ctx huma.Context) {
			ctx.SetHeader("Content-Type", "text/html; charset=utf-8")
			ctx.SetStatus(http.StatusOK)
			_, _ = ctx.BodyWriter().Write(body)
		}}, nil
	})
}

// IsolirCaptiveHandler serves the isolir landing page for ANY host/path/method.
// RouterOS dst-nat redirects isolir-pool HTTP (tcp/80) to this listener, so a
// customer opening any site is shown the isolir notice. Mount it on a dedicated
// plain-HTTP listener (ISOLIR_HTTP_ADDR), never on the main API router.
func IsolirCaptiveHandler(d *Deps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := renderIsolirPage(r.Context(), d)
		if err != nil {
			http.Error(w, "isolir page unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	})
}
