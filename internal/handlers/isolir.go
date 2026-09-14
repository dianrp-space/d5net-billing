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
		ten, _ := d.Store.GetTenant(ctx, tid)
		slug := ""
		if ten != nil {
			slug = ten.Slug
		}
		isolirURL := store.IsolirPortalURL(net.PortalBaseURL, slug)
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
		net.RedirectMode = "web-proxy"
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
		ten, _ := d.Store.GetTenant(ctx, tid)
		slug := ""
		if ten != nil {
			slug = ten.Slug
		}
		isolirURL := store.IsolirPortalURL(net.PortalBaseURL, slug)
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
		isolirURL = "{portal_base_url}/{tenantSlug}/client"
	}
	pool := net.PoolRanges
	if pool == "" {
		pool = "(pilih IP pool isolir)"
	}
	name := net.PoolName
	if name == "" {
		name = "isolir"
	}
	return "URL isolir (Web Proxy redirect-to):\n" + isolirURL +
		"\n\nPool isolir dipakai saat worker mengisolir langganan di router masing-masing." +
		"\nPool: " + name + " · " + pool +
		"\n\nSetting di RouterOS (IP → Web Proxy):\n" +
		"1. /ip proxy: enabled=yes, port=8080\n" +
		"2. /ip proxy access: allow host billing; ROS7 action=redirect action-data=URL (ROS6: deny + redirect-to)\n" +
		"3. NAT: tcp/80 dari pool → redirect ke port 8080\n" +
		"4. Filter: allow DNS; allow HTTPS portal via address-list FQDN (bukan IP publik); drop trafik lain\n" +
		"5. Urutan filter: dns → dns-tcp → portal → block (block harus tepat setelah portal)\n" +
		"Comment: d5n-isolir:* (aturan lama drp-isolir:* otomatis di-rename saat sync) · Secret isolir: prefix \"ISOLIR \""
}

func registerPublicIsolir(api huma.API, d *Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "public-isolir-page", Method: http.MethodGet, Path: "/api/public/isolir",
		Tags: []string{"Public"},
	}, func(ctx context.Context, _ *struct{}) (*huma.StreamResponse, error) {
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
		return &huma.StreamResponse{Body: func(ctx huma.Context) {
			ctx.SetHeader("Content-Type", "text/html; charset=utf-8")
			ctx.SetStatus(http.StatusOK)
			_, _ = ctx.BodyWriter().Write([]byte(body))
		}}, nil
	})
}
