package handlers

import (
	"context"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/dianrp-space/d5net-billing/internal/httpx"
	"github.com/dianrp-space/d5net-billing/internal/store"
)

// singleTenant resolves the one and only provider/tenant for this deployment.
func singleTenant(ctx context.Context, d *Deps) (*store.Tenant, error) {
	list, err := d.Store.ListTenants(ctx)
	if err != nil {
		return nil, httpx.Internal(err)
	}
	for i := range list {
		if list[i].IsActive {
			return &list[i], nil
		}
	}
	if len(list) > 0 {
		return &list[0], nil
	}
	return nil, httpx.NotFound("provider belum dikonfigurasi")
}

func registerPublicBranding(api huma.API, d *Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "public-branding", Method: http.MethodGet, Path: "/api/public/branding",
		Summary: "Provider branding", Tags: []string{"Public"},
	}, func(ctx context.Context, _ *struct{}) (*struct {
		Body struct {
			Name         string  `json:"name"`
			AppName      string  `json:"app_name"`
			LogoURL      *string `json:"logo_url,omitempty"`
			FaviconURL   *string `json:"favicon_url,omitempty"`
			PrimaryColor string  `json:"primary_color,omitempty"`
			Chatwoot     *struct {
				BaseURL      string `json:"base_url"`
				WebsiteToken string `json:"website_token"`
			} `json:"chatwoot,omitempty"`
		}
	}, error) {
		ten, err := singleTenant(ctx, d)
		if err != nil {
			return nil, err
		}
		view, err := d.Store.ResolveTenantBranding(ctx, ten.ID)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		gen, _ := d.Store.GetGeneralSettings(ctx, ten.ID)
		out := &struct {
			Body struct {
				Name         string  `json:"name"`
				AppName      string  `json:"app_name"`
				LogoURL      *string `json:"logo_url,omitempty"`
				FaviconURL   *string `json:"favicon_url,omitempty"`
				PrimaryColor string  `json:"primary_color,omitempty"`
				Chatwoot     *struct {
					BaseURL      string `json:"base_url"`
					WebsiteToken string `json:"website_token"`
				} `json:"chatwoot,omitempty"`
			}
		}{}
		out.Body.Name = ten.Name
		out.Body.AppName = view.Effective.AppName
		out.Body.LogoURL = view.Effective.LogoURL
		out.Body.FaviconURL = view.Effective.FaviconURL
		out.Body.PrimaryColor = gen.PrimaryColor
		if msg, lerr := loadMessagingIntegration(ctx, d, ten.ID); lerr == nil && msg.ChatwootEnabled {
			if base := strings.TrimRight(strings.TrimSpace(msg.ChatwootBaseURL), "/"); base != "" &&
				strings.TrimSpace(msg.ChatwootWebsiteToken) != "" {
				out.Body.Chatwoot = &struct {
					BaseURL      string `json:"base_url"`
					WebsiteToken string `json:"website_token"`
				}{BaseURL: base, WebsiteToken: strings.TrimSpace(msg.ChatwootWebsiteToken)}
			}
		}
		return out, nil
	})
}
