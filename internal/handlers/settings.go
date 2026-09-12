package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/dianrp-space/d5net-billing/internal/auth"
	"github.com/dianrp-space/d5net-billing/internal/httpx"
	"github.com/dianrp-space/d5net-billing/internal/store"
	"github.com/dianrp-space/d5net-billing/internal/tenant"
	"github.com/dianrp-space/d5net-billing/internal/upload"
	"github.com/dianrp-space/d5net-billing/internal/xid"
	"github.com/go-chi/chi/v5"
)

var roleSlugRE = regexp.MustCompile(`^[a-z][a-z0-9_-]{1,62}$`)

func requireSettings(ctx context.Context, d *Deps) (xid.ID, error) {
	info, ok := tenant.FromContext(ctx)
	if !ok || xid.IsNil(info.ID) {
		return xid.Nil(), httpx.Unauthorized("unauthorized")
	}
	role, err := d.Store.GetRoleBySlug(ctx, info.ID, info.Role)
	if err != nil {
		if info.Role == "admin" {
			return info.ID, nil
		}
		return xid.Nil(), httpx.Forbidden("role not found")
	}
	if !store.RoleHasPermission(role.Permissions, "settings") && !store.RoleHasPermission(role.Permissions, "*") {
		return xid.Nil(), httpx.Forbidden("settings permission required")
	}
	return info.ID, nil
}

func registerSettings(api huma.API, d *Deps) {
	registerTenantBrandingAPI(api, d)
	registerRolesUsers(api, d)
}

func registerTenantBrandingAPI(api huma.API, d *Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "get-tenant-branding", Method: http.MethodGet, Path: "/api/settings/branding",
		Tags: []string{"Settings"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, _ *struct{}) (*struct {
		Body struct {
			store.TenantBrandingView
			TenantName           string  `json:"tenant_name"`
			Timezone             string  `json:"timezone"`
			DefaultTaxPercent    float64 `json:"default_tax_percent"`
			IsolirGraceDays      int     `json:"isolir_grace_days"`
			BillingCycleStartDay int     `json:"billing_cycle_start_day"`
			InvoiceDueDay        int     `json:"invoice_due_day"`
			PrimaryColor         string  `json:"primary_color"`
		}
	}, error) {
		tid, err := requireSettings(ctx, d)
		if err != nil {
			return nil, err
		}
		view, err := d.Store.ResolveTenantBranding(ctx, tid)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		ten, err := d.Store.GetTenant(ctx, tid)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		gen, err := d.Store.GetGeneralSettings(ctx, tid)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		out := &struct {
			Body struct {
				store.TenantBrandingView
				TenantName           string  `json:"tenant_name"`
				Timezone             string  `json:"timezone"`
				DefaultTaxPercent    float64 `json:"default_tax_percent"`
				IsolirGraceDays      int     `json:"isolir_grace_days"`
				BillingCycleStartDay int     `json:"billing_cycle_start_day"`
				InvoiceDueDay        int     `json:"invoice_due_day"`
				PrimaryColor         string  `json:"primary_color"`
			}
		}{}
		out.Body.TenantBrandingView = *view
		out.Body.TenantName = ten.Name
		out.Body.Timezone = gen.Timezone
		out.Body.DefaultTaxPercent = gen.DefaultTaxPercent
		out.Body.IsolirGraceDays = gen.IsolirGraceDays
		out.Body.BillingCycleStartDay = gen.BillingCycleStartDay
		out.Body.InvoiceDueDay = gen.InvoiceDueDay
		out.Body.PrimaryColor = gen.PrimaryColor
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "put-tenant-branding", Method: http.MethodPut, Path: "/api/settings/branding",
		Tags: []string{"Settings"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Body struct {
			TenantName           string  `json:"tenant_name"`
			AppName              string  `json:"app_name,omitempty"`
			Timezone             string  `json:"timezone"`
			DefaultTaxPercent    float64 `json:"default_tax_percent"`
			IsolirGraceDays      int     `json:"isolir_grace_days"`
			BillingCycleStartDay int     `json:"billing_cycle_start_day"`
			InvoiceDueDay        int     `json:"invoice_due_day"`
			PrimaryColor         string  `json:"primary_color"`
			LogoURL              *string `json:"logo_url,omitempty"`
			FaviconURL           *string `json:"favicon_url,omitempty"`
			MapPopIconURL        *string `json:"map_pop_icon_url,omitempty"`
			MapODPIconURL        *string `json:"map_odp_icon_url,omitempty"`
			MapCustomerIconURL   *string `json:"map_customer_icon_url,omitempty"`
			ClearLogo            bool    `json:"clear_logo,omitempty"`
			ClearFavicon         bool    `json:"clear_favicon,omitempty"`
			ClearMapPopIcon      bool    `json:"clear_map_pop_icon,omitempty"`
			ClearMapODPIcon      bool    `json:"clear_map_odp_icon,omitempty"`
			ClearMapCustomerIcon bool    `json:"clear_map_customer_icon,omitempty"`
		}
	}) (*struct {
		Body struct {
			store.TenantBrandingView
			TenantName           string  `json:"tenant_name"`
			Timezone             string  `json:"timezone"`
			DefaultTaxPercent    float64 `json:"default_tax_percent"`
			IsolirGraceDays      int     `json:"isolir_grace_days"`
			BillingCycleStartDay int     `json:"billing_cycle_start_day"`
			InvoiceDueDay        int     `json:"invoice_due_day"`
			PrimaryColor         string  `json:"primary_color"`
		}
	}, error) {
		tid, err := requireSettings(ctx, d)
		if err != nil {
			return nil, err
		}
		tenantName := strings.TrimSpace(input.Body.TenantName)
		if tenantName == "" {
			return nil, httpx.BadRequest("nama tenant wajib diisi")
		}
		if err := d.Store.UpdateTenantName(ctx, tid, tenantName); err != nil {
			return nil, httpx.BadRequest(err.Error())
		}
		// Keep app_name in sync with tenant display name for UI/title/comments.
		raw, err := d.Store.GetTenantBrandingRaw(ctx, tid)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		b := &store.Branding{AppName: tenantName}
		b.LogoURL = pickBrandingURL(input.Body.ClearLogo, input.Body.LogoURL, raw.LogoURL)
		b.FaviconURL = pickBrandingURL(input.Body.ClearFavicon, input.Body.FaviconURL, raw.FaviconURL)
		b.MapPopIconURL = pickBrandingURL(input.Body.ClearMapPopIcon, input.Body.MapPopIconURL, raw.MapPopIconURL)
		b.MapODPIconURL = pickBrandingURL(input.Body.ClearMapODPIcon, input.Body.MapODPIconURL, raw.MapODPIconURL)
		b.MapCustomerIconURL = pickBrandingURL(input.Body.ClearMapCustomerIcon, input.Body.MapCustomerIconURL, raw.MapCustomerIconURL)
		if err := d.Store.UpdateTenantBranding(ctx, tid, b); err != nil {
			return nil, httpx.Internal(err)
		}
		gen := store.NormalizeGeneralSettings(store.GeneralSettings{
			Timezone:             input.Body.Timezone,
			DefaultTaxPercent:    input.Body.DefaultTaxPercent,
			IsolirGraceDays:      input.Body.IsolirGraceDays,
			BillingCycleStartDay: input.Body.BillingCycleStartDay,
			InvoiceDueDay:        input.Body.InvoiceDueDay,
			PrimaryColor:         input.Body.PrimaryColor,
		})
		if err := d.Store.UpsertGeneralSettings(ctx, tid, gen); err != nil {
			return nil, httpx.Internal(err)
		}
		view, err := d.Store.ResolveTenantBranding(ctx, tid)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		ten, err := d.Store.GetTenant(ctx, tid)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		out := &struct {
			Body struct {
				store.TenantBrandingView
				TenantName           string  `json:"tenant_name"`
				Timezone             string  `json:"timezone"`
				DefaultTaxPercent    float64 `json:"default_tax_percent"`
				IsolirGraceDays      int     `json:"isolir_grace_days"`
				BillingCycleStartDay int     `json:"billing_cycle_start_day"`
				InvoiceDueDay        int     `json:"invoice_due_day"`
				PrimaryColor         string  `json:"primary_color"`
			}
		}{}
		out.Body.TenantBrandingView = *view
		out.Body.TenantName = ten.Name
		out.Body.Timezone = gen.Timezone
		out.Body.DefaultTaxPercent = gen.DefaultTaxPercent
		out.Body.IsolirGraceDays = gen.IsolirGraceDays
		out.Body.BillingCycleStartDay = gen.BillingCycleStartDay
		out.Body.InvoiceDueDay = gen.InvoiceDueDay
		out.Body.PrimaryColor = gen.PrimaryColor
		return out, nil
	})
}

// MountStaticAndUploads serves uploaded assets and registers multipart upload endpoints on chi.
func MountStaticAndUploads(r chi.Router, d *Deps) {
	_ = os.MkdirAll(d.Config.UploadDir, 0o755)
	r.Handle("/uploads/*", http.StripPrefix("/uploads/", http.FileServer(http.Dir(d.Config.UploadDir))))
	r.Post("/api/settings/branding/logo", uploadHandler(d, "logo"))
	r.Post("/api/settings/branding/favicon", uploadHandler(d, "favicon"))
	r.Post("/api/settings/branding/map-pop", uploadHandler(d, "map-pop"))
	r.Post("/api/settings/branding/map-odp", uploadHandler(d, "map-odp"))
	r.Post("/api/settings/branding/map-customer", uploadHandler(d, "map-customer"))
	r.Post("/api/work-orders/{id}/photos", workOrderPhotoUpload(d))
	r.Post("/api/leads/{id}/comments/photos", leadCommentPhotoUpload(d))
	r.Post("/api/leads/{id}/documents/photos", leadDocumentPhotoUpload(d))
	r.Post("/api/tickets/{id}/messages/photos", ticketMessagePhotoUpload(d))
	r.Post("/api/tickets/photos", ticketDraftPhotoUpload(d))
	r.Post("/api/customers/{id}/documents/photos", customerDocumentPhotoUpload(d))
	r.Post("/api/portal/account/photo", portalCustomerPhotoUpload(d))
	r.Post("/api/me/avatar", meAvatarUpload(d))
	MountDBBackupRoutes(r, d)
}

// MountPaymentReturnPages menangani browser customer yang di-redirect PG ke
// callback URL via GET. DOKU memakai callback_url ganda: notifikasi server
// (POST, ditangani webhook huma) sekaligus redirect browser customer setelah
// bayar. Arahkan ke dashboard portal; guard login meneruskan yang belum login.
func MountPaymentReturnPages(r chi.Router) {
	r.Get("/api/webhooks/payment/doku", func(w http.ResponseWriter, req *http.Request) {
		// #region agent log
		agentDebugLog("settings.go:MountPaymentReturnPages", "DOKU browser GET redirect (no invoice update)", "A", map[string]any{
			"path":       req.URL.Path,
			"query":      req.URL.RawQuery,
			"user_agent": req.UserAgent(),
			"referer":    req.Referer(),
		})
		// #endregion
		http.Redirect(w, req, "/client/dashboard", http.StatusFound)
	})
}

// portalCustomerPhotoUpload lets a portal customer upload/remove their own
// profile photo. Auth via portal token (Authorization header).
func portalCustomerPhotoUpload(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		ten, custs, err := authenticatePortalRequest(ctx, d, r.Header.Get("Authorization"), "", "", "")
		if err != nil || len(custs) == 0 {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		if err := r.ParseMultipartForm(upload.MaxBytes + (2 << 20)); err != nil {
			http.Error(w, `{"error":"invalid multipart"}`, http.StatusBadRequest)
			return
		}
		target := custs[0]
		if raw := strings.TrimSpace(r.FormValue("customer_id")); raw != "" {
			cid, perr := xid.Parse(raw)
			if perr != nil || xid.IsNil(cid) {
				http.Error(w, `{"error":"customer tidak valid"}`, http.StatusBadRequest)
				return
			}
			found := false
			for _, c := range custs {
				if c != nil && c.ID == cid {
					target = c
					found = true
					break
				}
			}
			if !found {
				http.Error(w, `{"error":"akun tidak ditemukan"}`, http.StatusBadRequest)
				return
			}
		} else if len(custs) > 1 {
			http.Error(w, `{"error":"pilih akun yang fotonya diubah"}`, http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(r.FormValue("remove")) == "true" {
			if err := d.Store.UpdateCustomerPhoto(ctx, ten.ID, target.ID, nil); err != nil {
				http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
				return
			}
			writeJSON(w, map[string]any{"url": nil})
			return
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			http.Error(w, `{"error":"file is required"}`, http.StatusBadRequest)
			return
		}
		defer file.Close()
		kind := fmt.Sprintf("customer-photo-%s", target.ID.String())
		url, err := saveUpload(d, ten.ID, false, kind, file, header.Filename, header.Size)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadRequest)
			return
		}
		if err := d.Store.UpdateCustomerPhoto(ctx, ten.ID, target.ID, &url); err != nil {
			http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]string{"url": url})
	}
}

func ticketDraftPhotoUpload(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		uid := userIDFromCtx(ctx)
		if xid.IsNil(uid) {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		if err := r.ParseMultipartForm(upload.MaxBytes + (2 << 20)); err != nil {
			http.Error(w, `{"error":"invalid multipart"}`, http.StatusBadRequest)
			return
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			http.Error(w, `{"error":"file is required"}`, http.StatusBadRequest)
			return
		}
		defer file.Close()
		kind := fmt.Sprintf("ticket-draft-%s-%d", uid.String(), time.Now().UnixNano())
		url, err := saveUpload(d, tid, false, kind, file, header.Filename, header.Size)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"url": url})
	}
}

func meAvatarUpload(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		uid := userIDFromCtx(ctx)
		if xid.IsNil(uid) {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		if _, err := d.Store.GetUserByID(ctx, uid); errors.Is(err, store.ErrNotFound) {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		} else if err != nil {
			http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
			return
		}
		if err := r.ParseMultipartForm(upload.MaxBytes + (2 << 20)); err != nil {
			http.Error(w, `{"error":"invalid multipart"}`, http.StatusBadRequest)
			return
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			http.Error(w, `{"error":"file is required"}`, http.StatusBadRequest)
			return
		}
		defer file.Close()
		kind := fmt.Sprintf("avatar-%s", uid.String())
		url, err := saveUpload(d, tid, false, kind, file, header.Filename, header.Size)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadRequest)
			return
		}
		if err := d.Store.UpdateUserAvatar(ctx, uid, &url); err != nil {
			http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"url": url})
	}
}

func leadCommentPhotoUpload(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		rawID := chi.URLParam(r, "id")
		leadID, err := xid.Parse(rawID)
		if err != nil || xid.IsNil(leadID) {
			http.Error(w, `{"error":"id tidak valid"}`, http.StatusBadRequest)
			return
		}
		lead, err := d.Store.GetLead(ctx, tid, leadID)
		if errors.Is(err, store.ErrNotFound) {
			http.Error(w, `{"error":"lead tidak ditemukan"}`, http.StatusNotFound)
			return
		} else if err != nil {
			http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
			return
		}
		if err := enforceLeadAssigned(ctx, d, lead); err != nil {
			http.Error(w, `{"error":"lead ini tidak di-assign ke Anda"}`, http.StatusForbidden)
			return
		}
		if err := r.ParseMultipartForm(upload.MaxBytes + (2 << 20)); err != nil {
			http.Error(w, `{"error":"invalid multipart"}`, http.StatusBadRequest)
			return
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			http.Error(w, `{"error":"file is required"}`, http.StatusBadRequest)
			return
		}
		defer file.Close()
		kind := fmt.Sprintf("lead-%s-%d", leadID.String(), time.Now().UnixNano())
		url, err := saveUpload(d, tid, false, kind, file, header.Filename, header.Size)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"url": url})
	}
}

func ticketMessagePhotoUpload(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		rawID := chi.URLParam(r, "id")
		ticketID, err := xid.Parse(rawID)
		if err != nil || xid.IsNil(ticketID) {
			http.Error(w, `{"error":"id tidak valid"}`, http.StatusBadRequest)
			return
		}
		t, err := d.Store.GetTicket(ctx, tid, ticketID)
		if errors.Is(err, store.ErrNotFound) {
			http.Error(w, `{"error":"tiket tidak ditemukan"}`, http.StatusNotFound)
			return
		} else if err != nil {
			http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
			return
		}
		if err := enforceTicketAssigned(ctx, d, t); err != nil {
			http.Error(w, `{"error":"tiket ini tidak di-assign ke Anda"}`, http.StatusForbidden)
			return
		}
		if err := r.ParseMultipartForm(upload.MaxBytes + (2 << 20)); err != nil {
			http.Error(w, `{"error":"invalid multipart"}`, http.StatusBadRequest)
			return
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			http.Error(w, `{"error":"file is required"}`, http.StatusBadRequest)
			return
		}
		defer file.Close()
		kind := fmt.Sprintf("ticket-%s-%d", ticketID.String(), time.Now().UnixNano())
		url, err := saveUpload(d, tid, false, kind, file, header.Filename, header.Size)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"url": url})
	}
}

func leadDocumentPhotoUpload(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		rawID := chi.URLParam(r, "id")
		leadID, err := xid.Parse(rawID)
		if err != nil || xid.IsNil(leadID) {
			http.Error(w, `{"error":"id tidak valid"}`, http.StatusBadRequest)
			return
		}
		lead, err := d.Store.GetLead(ctx, tid, leadID)
		if errors.Is(err, store.ErrNotFound) {
			http.Error(w, `{"error":"lead tidak ditemukan"}`, http.StatusNotFound)
			return
		} else if err != nil {
			http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
			return
		}
		if err := enforceLeadAssigned(ctx, d, lead); err != nil {
			http.Error(w, `{"error":"lead ini tidak di-assign ke Anda"}`, http.StatusForbidden)
			return
		}
		st := store.NormalizeLeadStatus(lead.Status)
		if st != "survey" && st != "qualified" {
			http.Error(w, `{"error":"dokumen PSB hanya saat survey / proses pasang"}`, http.StatusBadRequest)
			return
		}
		if err := r.ParseMultipartForm(upload.MaxBytes + (2 << 20)); err != nil {
			http.Error(w, `{"error":"invalid multipart"}`, http.StatusBadRequest)
			return
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			http.Error(w, `{"error":"file is required"}`, http.StatusBadRequest)
			return
		}
		defer file.Close()
		kind := fmt.Sprintf("lead-doc-%s-%d", leadID.String(), time.Now().UnixNano())
		url, err := saveUpload(d, tid, false, kind, file, header.Filename, header.Size)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"url": url})
	}
}

func customerDocumentPhotoUpload(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		rawID := chi.URLParam(r, "id")
		customerID, err := xid.Parse(rawID)
		if err != nil || xid.IsNil(customerID) {
			http.Error(w, `{"error":"id tidak valid"}`, http.StatusBadRequest)
			return
		}
		if _, err := d.Store.GetCustomer(ctx, tid, customerID); errors.Is(err, store.ErrNotFound) {
			http.Error(w, `{"error":"pelanggan tidak ditemukan"}`, http.StatusNotFound)
			return
		} else if err != nil {
			http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
			return
		}
		if err := r.ParseMultipartForm(upload.MaxBytes + (2 << 20)); err != nil {
			http.Error(w, `{"error":"invalid multipart"}`, http.StatusBadRequest)
			return
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			http.Error(w, `{"error":"file is required"}`, http.StatusBadRequest)
			return
		}
		defer file.Close()
		kind := fmt.Sprintf("cust-doc-%s-%d", customerID.String(), time.Now().UnixNano())
		url, err := saveUpload(d, tid, false, kind, file, header.Filename, header.Size)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"url": url})
	}
}

func workOrderPhotoUpload(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		rawID := chi.URLParam(r, "id")
		woID, err := xid.Parse(rawID)
		if err != nil || xid.IsNil(woID) {
			http.Error(w, `{"error":"id tidak valid"}`, http.StatusBadRequest)
			return
		}
		cur, err := d.Store.GetWorkOrder(ctx, tid, woID)
		if errors.Is(err, store.ErrNotFound) {
			http.Error(w, `{"error":"work order not found"}`, http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
			return
		}
		if err := enforceWorkOrderAssigned(ctx, d, tid, cur); err != nil {
			http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
			return
		}
		if err := r.ParseMultipartForm(upload.MaxBytes + (2 << 20)); err != nil {
			http.Error(w, `{"error":"invalid multipart"}`, http.StatusBadRequest)
			return
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			http.Error(w, `{"error":"file is required"}`, http.StatusBadRequest)
			return
		}
		defer file.Close()
		kind := fmt.Sprintf("wo-%s-%d", woID.String(), time.Now().UnixNano())
		url, err := saveUpload(d, tid, false, kind, file, header.Filename, header.Size)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadRequest)
			return
		}
		wo, err := d.Store.AppendWorkOrderPhoto(ctx, tid, woID, url)
		if errors.Is(err, store.ErrNotFound) {
			http.Error(w, `{"error":"work order not found"}`, http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(wo)
	}
}

func uploadHandler(d *Deps, kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		tid, err := requireSettings(ctx, d)
		if err != nil {
			// Distinguish auth vs permission so the SPA does not treat 403 as session expiry.
			if _, ok := tenant.FromContext(ctx); !ok {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
			http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
			return
		}
		if err := r.ParseMultipartForm(2 << 20); err != nil {
			http.Error(w, `{"error":"invalid multipart"}`, http.StatusBadRequest)
			return
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			http.Error(w, `{"error":"file is required"}`, http.StatusBadRequest)
			return
		}
		defer file.Close()
		url, err := saveUpload(d, tid, false, kind, file, header.Filename, header.Size)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadRequest)
			return
		}
		raw, err := d.Store.GetTenantBrandingRaw(ctx, tid)
		if err != nil {
			http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
			return
		}
		setBrandingURL(raw, kind, &url)
		_ = d.Store.UpdateTenantBranding(ctx, tid, raw)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"url": url})
	}
}

// pickBrandingURL returns the URL to persist for a branding field, honouring clear/override/keep.
func pickBrandingURL(clear bool, given, current *string) *string {
	if clear {
		return nil
	}
	if given != nil {
		return given
	}
	return current
}

// setBrandingURL assigns an uploaded URL to the matching branding field for kind.
func setBrandingURL(b *store.Branding, kind string, url *string) {
	switch kind {
	case "logo":
		b.LogoURL = url
	case "favicon":
		b.FaviconURL = url
	case "map-pop":
		b.MapPopIconURL = url
	case "map-odp":
		b.MapODPIconURL = url
	case "map-customer":
		b.MapCustomerIconURL = url
	}
}

func isMapIconKind(kind string) bool {
	return kind == "map-pop" || kind == "map-odp" || kind == "map-customer"
}

func saveUpload(d *Deps, tenantID xid.ID, platform bool, kind string, src io.Reader, filename string, size int64) (string, error) {
	subdir := "platform"
	if !platform {
		subdir = tenantID.String()
	}
	dir := filepath.Join(d.Config.UploadDir, subdir)
	var name string
	var err error
	if isMapIconKind(kind) {
		name, err = upload.SaveMapIconAsWebP(dir, kind, src, filename, size)
	} else {
		name, err = upload.SaveImageAsWebP(dir, kind, src, filename, size)
	}
	if err != nil {
		return "", err
	}
	return "/uploads/" + subdir + "/" + name, nil
}

// UserOption is a lightweight user row for assign pickers (OpenAPI-named to avoid Huma "Item" clashes).
type UserOption struct {
	UserID   xid.ID `json:"user_id"`
	FullName string `json:"full_name"`
	Email    string `json:"email"`
	IsActive bool   `json:"is_active"`
	RoleSlug string `json:"role_slug"`
}

func registerRolesUsers(api huma.API, d *Deps) {
	// Lightweight staff picker for assign UI — any authenticated tenant user (not settings-only).
	huma.Register(api, huma.Operation{
		OperationID: "list-user-options", Method: http.MethodGet, Path: "/api/users/options",
		Tags: []string{"Users"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, _ *struct{}) (*struct{ Body []UserOption }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		list, err := d.Store.ListTenantUsers(ctx, tid)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		out := make([]UserOption, 0, len(list))
		for _, u := range list {
			out = append(out, UserOption{
				UserID: u.UserID, FullName: u.FullName, Email: u.Email,
				IsActive: u.IsActive, RoleSlug: u.RoleSlug,
			})
		}
		return &struct{ Body []UserOption }{Body: out}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-roles", Method: http.MethodGet, Path: "/api/settings/roles",
		Tags: []string{"Settings"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, _ *struct{}) (*struct{ Body []store.Role }, error) {
		tid, err := requireSettings(ctx, d)
		if err != nil {
			return nil, err
		}
		list, err := d.Store.ListRoles(ctx, tid)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		if list == nil {
			list = []store.Role{}
		}
		return &struct{ Body []store.Role }{Body: list}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "create-role", Method: http.MethodPost, Path: "/api/settings/roles",
		Tags: []string{"Settings"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Body struct {
			Name        string   `json:"name"`
			Slug        string   `json:"slug"`
			Permissions []string `json:"permissions"`
		}
	}) (*struct{ Body store.Role }, error) {
		tid, err := requireSettings(ctx, d)
		if err != nil {
			return nil, err
		}
		name := strings.TrimSpace(input.Body.Name)
		slug := strings.ToLower(strings.TrimSpace(input.Body.Slug))
		if name == "" || !roleSlugRE.MatchString(slug) {
			return nil, httpx.BadRequest("name/slug tidak valid")
		}
		r := &store.Role{TenantID: tid, Name: name, Slug: slug, Permissions: input.Body.Permissions, IsSystem: false}
		if err := d.Store.CreateRole(ctx, r); err != nil {
			return nil, httpx.Internal(err)
		}
		auditEvent(ctx, d, AuditRoleCreate, "role", &r.ID, map[string]any{"slug": slug, "name": name})
		return &struct{ Body store.Role }{Body: *r}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "update-role", Method: http.MethodPut, Path: "/api/settings/roles/{id}",
		Tags: []string{"Settings"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID   xid.ID `path:"id"`
		Body struct {
			Name        string   `json:"name"`
			Slug        string   `json:"slug"`
			Permissions []string `json:"permissions"`
		}
	}) (*struct{ Body store.Role }, error) {
		tid, err := requireSettings(ctx, d)
		if err != nil {
			return nil, err
		}
		existing, err := d.Store.GetRole(ctx, tid, input.ID)
		if errors.Is(err, store.ErrNotFound) {
			return nil, httpx.NotFound("role tidak ditemukan")
		}
		if err != nil {
			return nil, httpx.Internal(err)
		}
		name := strings.TrimSpace(input.Body.Name)
		slug := strings.ToLower(strings.TrimSpace(input.Body.Slug))
		if name == "" || !roleSlugRE.MatchString(slug) {
			return nil, httpx.BadRequest("name/slug tidak valid")
		}
		if existing.IsSystem {
			slug = existing.Slug // system slug immutable
		}
		existing.Name = name
		existing.Slug = slug
		existing.Permissions = input.Body.Permissions
		if err := d.Store.UpdateRole(ctx, existing); err != nil {
			return nil, httpx.Internal(err)
		}
		auditEvent(ctx, d, AuditRoleUpdate, "role", &existing.ID, map[string]any{"slug": slug})
		return &struct{ Body store.Role }{Body: *existing}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "delete-role", Method: http.MethodDelete, Path: "/api/settings/roles/{id}",
		Tags: []string{"Settings"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID xid.ID `path:"id"`
	}) (*struct{}, error) {
		tid, err := requireSettings(ctx, d)
		if err != nil {
			return nil, err
		}
		if err := d.Store.DeleteRole(ctx, tid, input.ID); err != nil {
			if err.Error() == "system role cannot be deleted" {
				return nil, httpx.BadRequest(err.Error())
			}
			if errors.Is(err, store.ErrNotFound) {
				return nil, httpx.NotFound("role tidak ditemukan")
			}
			return nil, httpx.Internal(err)
		}
		auditEvent(ctx, d, AuditRoleDelete, "role", &input.ID, nil)
		return &struct{}{}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-tenant-users", Method: http.MethodGet, Path: "/api/settings/users",
		Tags: []string{"Settings"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, _ *struct{}) (*struct{ Body []store.TenantUser }, error) {
		tid, err := requireSettings(ctx, d)
		if err != nil {
			return nil, err
		}
		list, err := d.Store.ListTenantUsers(ctx, tid)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		if list == nil {
			list = []store.TenantUser{}
		}
		return &struct{ Body []store.TenantUser }{Body: list}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "create-tenant-user", Method: http.MethodPost, Path: "/api/settings/users",
		Tags: []string{"Settings"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Body struct {
			Email    string  `json:"email"`
			Password string  `json:"password"`
			FullName string  `json:"full_name"`
			Phone    *string `json:"phone,omitempty"`
			RoleID   xid.ID  `json:"role_id"`
		}
	}) (*struct{ Body map[string]string }, error) {
		tid, err := requireSettings(ctx, d)
		if err != nil {
			return nil, err
		}
		email := strings.TrimSpace(strings.ToLower(input.Body.Email))
		fullName := strings.TrimSpace(input.Body.FullName)
		if email == "" || fullName == "" || len(input.Body.Password) < 8 {
			return nil, httpx.BadRequest("email, nama, dan password (min 8) wajib")
		}
		if _, err := d.Store.GetRole(ctx, tid, input.Body.RoleID); err != nil {
			return nil, httpx.BadRequest("role tidak valid")
		}
		hash, err := auth.HashPassword(input.Body.Password)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		uid, err := d.Store.AddTenantUser(ctx, tid, input.Body.RoleID, email, hash, fullName, input.Body.Phone)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		auditEvent(ctx, d, AuditUserCreate, "user", &uid, map[string]any{"email": email})
		return &struct{ Body map[string]string }{Body: map[string]string{"user_id": uid.String()}}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "update-tenant-user", Method: http.MethodPut, Path: "/api/settings/users/{id}",
		Tags: []string{"Settings"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID   xid.ID `path:"id"`
		Body struct {
			FullName string  `json:"full_name"`
			Phone    *string `json:"phone,omitempty"`
			RoleID   xid.ID  `json:"role_id"`
			IsActive bool    `json:"is_active"`
			Password string  `json:"password,omitempty"`
		}
	}) (*struct{}, error) {
		tid, err := requireSettings(ctx, d)
		if err != nil {
			return nil, err
		}
		if _, err := d.Store.GetRole(ctx, tid, input.Body.RoleID); err != nil {
			return nil, httpx.BadRequest("role tidak valid")
		}
		fullName := strings.TrimSpace(input.Body.FullName)
		if fullName == "" {
			return nil, httpx.BadRequest("nama wajib")
		}
		if err := d.Store.UpdateTenantUser(ctx, tid, input.ID, input.Body.RoleID, fullName, input.Body.Phone, input.Body.IsActive); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return nil, httpx.NotFound("user tidak ditemukan")
			}
			return nil, httpx.Internal(err)
		}
		if pw := strings.TrimSpace(input.Body.Password); pw != "" {
			if len(pw) < 8 {
				return nil, httpx.BadRequest("password min 8 karakter")
			}
			hash, err := auth.HashPassword(pw)
			if err != nil {
				return nil, httpx.Internal(err)
			}
			_ = d.Store.SetUserPassword(ctx, input.ID, hash)
		}
		return &struct{}{}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "delete-tenant-user", Method: http.MethodDelete, Path: "/api/settings/users/{id}",
		Tags: []string{"Settings"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID xid.ID `path:"id"`
	}) (*struct{}, error) {
		tid, err := requireSettings(ctx, d)
		if err != nil {
			return nil, err
		}
		if err := d.Store.RemoveTenantUser(ctx, tid, input.ID); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return nil, httpx.NotFound("user tidak ditemukan")
			}
			return nil, httpx.Internal(err)
		}
		auditEvent(ctx, d, AuditUserDelete, "user", &input.ID, nil)
		return &struct{}{}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-audit-logs", Method: http.MethodGet, Path: "/api/audit-logs",
		Tags: []string{"Settings"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Action string `query:"action"`
		Search string `query:"search"`
		Limit  int    `query:"limit"`
		Offset int    `query:"offset"`
	}) (*struct {
		Body struct {
			Data  []store.AuditLog `json:"data"`
			Total int64            `json:"total"`
		}
	}, error) {
		tid, err := requireSettings(ctx, d)
		if err != nil {
			return nil, err
		}
		list, total, err := d.Store.ListAuditLogs(ctx, tid, input.Action, input.Search, input.Limit, input.Offset)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		if list == nil {
			list = []store.AuditLog{}
		}
		out := &struct {
			Body struct {
				Data  []store.AuditLog `json:"data"`
				Total int64            `json:"total"`
			}
		}{}
		out.Body.Data = list
		out.Body.Total = total
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-portal-users", Method: http.MethodGet, Path: "/api/settings/portal-users",
		Tags: []string{"Settings"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, _ *struct{}) (*struct{ Body []store.PortalUser }, error) {
		tid, err := requireSettings(ctx, d)
		if err != nil {
			return nil, err
		}
		list, err := d.Store.ListPortalUsers(ctx, tid)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		if list == nil {
			list = []store.PortalUser{}
		}
		return &struct{ Body []store.PortalUser }{Body: list}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "update-portal-user", Method: http.MethodPut, Path: "/api/settings/portal-users/{id}",
		Tags: []string{"Settings"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID   xid.ID `path:"id"`
		Body struct {
			PortalEnabled bool   `json:"portal_enabled"`
			Password      string `json:"password,omitempty"`
		}
	}) (*struct{}, error) {
		tid, err := requireSettings(ctx, d)
		if err != nil {
			return nil, err
		}
		cust, err := d.Store.GetCustomer(ctx, tid, input.ID)
		if errors.Is(err, store.ErrNotFound) {
			return nil, httpx.NotFound("pelanggan tidak ditemukan")
		}
		if err != nil {
			return nil, httpx.Internal(err)
		}
		var hashPtr *string
		if pw := strings.TrimSpace(input.Body.Password); pw != "" {
			if len(pw) < 6 {
				return nil, httpx.BadRequest("password portal min 6 karakter")
			}
			hash, err := auth.HashPassword(pw)
			if err != nil {
				return nil, httpx.Internal(err)
			}
			hashPtr = &hash
		} else {
			existing, _ := d.Store.GetCustomerPasswordHash(ctx, tid, input.ID)
			if existing == "" {
				hash, err := auth.HashPassword(cust.Phone)
				if err != nil {
					return nil, httpx.Internal(err)
				}
				hashPtr = &hash
			}
		}
		if err := d.Store.UpdatePortalUser(ctx, tid, input.ID, input.Body.PortalEnabled, hashPtr); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return nil, httpx.NotFound("pelanggan tidak ditemukan")
			}
			return nil, httpx.Internal(err)
		}
		return &struct{}{}, nil
	})
}
