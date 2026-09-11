package handlers

import (
	"context"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/dianrp-space/d5net-billing/internal/httpx"
	"github.com/dianrp-space/d5net-billing/internal/invoice"
	"github.com/dianrp-space/d5net-billing/internal/store"
	"github.com/dianrp-space/d5net-billing/internal/xid"
)

func registerInvoiceSettings(api huma.API, d *Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "get-invoice-settings", Method: http.MethodGet, Path: "/api/settings/invoice",
		Summary: "Get invoice document customization", Tags: []string{"Settings"},
		Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, _ *struct{}) (*struct {
		Body struct {
			Settings       store.InvoiceSettings `json:"settings"`
			DefaultCompany string                `json:"default_company"`
			LogoURL        string                `json:"logo_url,omitempty"`
			TenantSlug     string                `json:"tenant_slug,omitempty"`
		}
	}, error) {
		tid, err := requireSettings(ctx, d)
		if err != nil {
			return nil, err
		}
		cfg, err := d.Store.GetInvoiceSettings(ctx, tid)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		out := &struct {
			Body struct {
				Settings       store.InvoiceSettings `json:"settings"`
				DefaultCompany string                `json:"default_company"`
				LogoURL        string                `json:"logo_url,omitempty"`
				TenantSlug     string                `json:"tenant_slug,omitempty"`
			}
		}{}
		out.Body.Settings = cfg
		out.Body.DefaultCompany = fallbackCompanyName(ctx, d, tid)
		out.Body.LogoURL = tenantLogoURL(ctx, d, tid)
		if ten, terr := d.Store.GetTenant(ctx, tid); terr == nil && ten != nil {
			out.Body.TenantSlug = ten.Slug
		}
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "put-invoice-settings", Method: http.MethodPut, Path: "/api/settings/invoice",
		Summary: "Update invoice document customization", Tags: []string{"Settings"},
		Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Body store.InvoiceSettings
	}) (*struct {
		Body store.InvoiceSettings
	}, error) {
		tid, err := requireSettings(ctx, d)
		if err != nil {
			return nil, err
		}
		cfg := store.NormalizeInvoiceSettings(input.Body)
		if err := d.Store.UpsertInvoiceSettings(ctx, tid, cfg); err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body store.InvoiceSettings }{Body: cfg}, nil
	})
}

// fallbackCompanyName resolves the best default business name for the invoice
// header (branding app name, then tenant name).
func fallbackCompanyName(ctx context.Context, d *Deps, tenantID xid.ID) string {
	if name, err := d.Store.EffectiveAppName(ctx, tenantID); err == nil && strings.TrimSpace(name) != "" {
		return strings.TrimSpace(name)
	}
	if ten, err := d.Store.GetTenant(ctx, tenantID); err == nil && ten != nil {
		return strings.TrimSpace(ten.Name)
	}
	return ""
}

func tenantLogoURL(ctx context.Context, d *Deps, tenantID xid.ID) string {
	view, err := d.Store.ResolveTenantBranding(ctx, tenantID)
	if err != nil || view == nil || view.Effective.LogoURL == nil {
		return ""
	}
	return strings.TrimSpace(*view.Effective.LogoURL)
}

// invoiceRenderOptions assembles tenant + customer context for the PDF renderer.
func invoiceRenderOptions(ctx context.Context, d *Deps, tenantID xid.ID, inv *store.Invoice) invoice.RenderOptions {
	opts := invoice.RenderOptions{FallbackCompany: fallbackCompanyName(ctx, d, tenantID)}
	if cfg, err := d.Store.GetInvoiceSettings(ctx, tenantID); err == nil {
		opts.Settings = cfg
	}
	if d.Config != nil {
		opts.Logo = invoice.LoadLogoJPEG(d.Config.UploadDir, tenantLogoURL(ctx, d, tenantID))
	}
	if inv != nil {
		if cust, err := d.Store.GetCustomer(ctx, tenantID, inv.CustomerID); err == nil && cust != nil {
			if cust.Address != nil {
				opts.CustomerAddress = strings.TrimSpace(*cust.Address)
			}
			opts.CustomerPhone = strings.TrimSpace(cust.Phone)
			if cust.Email != nil {
				opts.CustomerEmail = strings.TrimSpace(*cust.Email)
			}
		}
	}
	return opts
}

// renderInvoicePDF builds the invoice PDF bytes for a tenant invoice.
func renderInvoicePDF(ctx context.Context, d *Deps, tenantID xid.ID, inv *store.Invoice, items []store.InvoiceItem) []byte {
	return invoice.RenderPDF(inv, items, invoiceRenderOptions(ctx, d, tenantID, inv))
}
