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
			Timezone     string  `json:"timezone,omitempty"`
			AdminTagline string  `json:"admin_tagline,omitempty"`
			PortalTagline string `json:"portal_tagline,omitempty"`
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
				Timezone     string  `json:"timezone,omitempty"`
				AdminTagline string  `json:"admin_tagline,omitempty"`
				PortalTagline string `json:"portal_tagline,omitempty"`
				Chatwoot     *struct {
					BaseURL      string `json:"base_url"`
					WebsiteToken string `json:"website_token"`
				} `json:"chatwoot,omitempty"`
			}
		}{}
		out.Body.Name = ten.Name
		if strings.TrimSpace(out.Body.Name) == "" {
			out.Body.Name = view.Effective.AppName
		}
		out.Body.AppName = view.Effective.AppName
		out.Body.LogoURL = view.Effective.LogoURL
		out.Body.FaviconURL = view.Effective.FaviconURL
		out.Body.PrimaryColor = gen.PrimaryColor
		out.Body.Timezone = gen.Timezone
		out.Body.AdminTagline = gen.AdminTagline
		out.Body.PortalTagline = gen.PortalTagline
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

// registerPublicSite exposes the content required on the public website:
// business description, product/plan list with prices, and support contact.
func registerPublicSite(api huma.API, d *Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "public-site", Method: http.MethodGet, Path: "/api/public/site",
		Summary: "Public site content (business profile, products, support)", Tags: []string{"Public"},
	}, func(ctx context.Context, _ *struct{}) (*struct {
		Body struct {
			Name               string             `json:"name"`
			AppName            string             `json:"app_name"`
			LogoURL            *string            `json:"logo_url,omitempty"`
			FaviconURL         *string            `json:"favicon_url,omitempty"`
			PrimaryColor       string             `json:"primary_color,omitempty"`
			About              string             `json:"about"`
			ProductDescription string             `json:"product_description"`
			SupportEmail       string             `json:"support_email"`
			SupportPhone       string             `json:"support_phone"`
			SupportAddress     string             `json:"support_address"`
			Plans              []store.PublicPlan `json:"plans"`
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
		plans, _ := d.Store.ListPublicPlans(ctx, ten.ID)
		if plans == nil {
			plans = []store.PublicPlan{}
		}
		out := &struct {
			Body struct {
				Name               string             `json:"name"`
				AppName            string             `json:"app_name"`
				LogoURL            *string            `json:"logo_url,omitempty"`
				FaviconURL         *string            `json:"favicon_url,omitempty"`
				PrimaryColor       string             `json:"primary_color,omitempty"`
				About              string             `json:"about"`
				ProductDescription string             `json:"product_description"`
				SupportEmail       string             `json:"support_email"`
				SupportPhone       string             `json:"support_phone"`
				SupportAddress     string             `json:"support_address"`
				Plans              []store.PublicPlan `json:"plans"`
			}
		}{}
		out.Body.Name = ten.Name
		if strings.TrimSpace(out.Body.Name) == "" {
			out.Body.Name = view.Effective.AppName
		}
		out.Body.AppName = view.Effective.AppName
		out.Body.LogoURL = view.Effective.LogoURL
		out.Body.FaviconURL = view.Effective.FaviconURL
		out.Body.PrimaryColor = gen.PrimaryColor
		out.Body.About = gen.About
		out.Body.ProductDescription = gen.ProductDescription
		out.Body.SupportEmail = gen.SupportEmail
		out.Body.SupportPhone = gen.SupportPhone
		out.Body.SupportAddress = gen.SupportAddress
		out.Body.Plans = plans
		return out, nil
	})
}
