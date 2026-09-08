package handlers

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/dianrp/drp-billing/internal/auth"
	"github.com/dianrp/drp-billing/internal/httpx"
	"github.com/dianrp/drp-billing/internal/store"
	"github.com/dianrp/drp-billing/internal/tenant"
	"github.com/dianrp/drp-billing/internal/xid"
)

var slugRE = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,62}[a-z0-9])?$`)

func requirePlatform(ctx context.Context) error {
	t, ok := tenant.FromContext(ctx)
	if !ok || t.Role != "platform" {
		return httpx.Unauthorized("platform admin required")
	}
	return nil
}

func registerPublicTenant(api huma.API, d *Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "public-branding", Method: http.MethodGet, Path: "/api/public/branding",
		Summary: "Platform/owner branding", Tags: []string{"Public"},
	}, func(ctx context.Context, _ *struct{}) (*struct{ Body store.Branding }, error) {
		b, err := d.Store.GetPlatformBranding(ctx)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body store.Branding }{Body: *b}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "public-tenant-by-slug", Method: http.MethodGet, Path: "/api/public/tenants/{slug}",
		Summary: "Public tenant info by slug", Tags: []string{"Public"},
	}, func(ctx context.Context, input *struct {
		Slug string `path:"slug"`
	}) (*struct {
		Body struct {
			Slug         string  `json:"slug"`
			Name         string  `json:"name"`
			IsActive     bool    `json:"is_active"`
			AppName      string  `json:"app_name"`
			LogoURL      *string `json:"logo_url,omitempty"`
			FaviconURL   *string `json:"favicon_url,omitempty"`
			PrimaryColor string  `json:"primary_color,omitempty"`
		}
	}, error) {
		t, err := d.Store.GetTenantBySlug(ctx, strings.ToLower(strings.TrimSpace(input.Slug)))
		if errors.Is(err, store.ErrNotFound) {
			return nil, httpx.NotFound("tenant not found")
		}
		if err != nil {
			return nil, httpx.Internal(err)
		}
		if !t.IsActive {
			return nil, httpx.NotFound("tenant not found")
		}
		view, err := d.Store.ResolveTenantBranding(ctx, t.ID)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		gen, _ := d.Store.GetGeneralSettings(ctx, t.ID)
		out := &struct {
			Body struct {
				Slug         string  `json:"slug"`
				Name         string  `json:"name"`
				IsActive     bool    `json:"is_active"`
				AppName      string  `json:"app_name"`
				LogoURL      *string `json:"logo_url,omitempty"`
				FaviconURL   *string `json:"favicon_url,omitempty"`
				PrimaryColor string  `json:"primary_color,omitempty"`
			}
		}{}
		out.Body.Slug = t.Slug
		out.Body.Name = t.Name
		out.Body.IsActive = t.IsActive
		out.Body.AppName = view.Effective.AppName
		out.Body.LogoURL = view.Effective.LogoURL
		out.Body.FaviconURL = view.Effective.FaviconURL
		out.Body.PrimaryColor = gen.PrimaryColor
		return out, nil
	})
}

func registerPlatform(api huma.API, d *Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "platform-login", Method: http.MethodPost, Path: "/api/auth/platform/login",
		Summary: "Login platform (superadmin)", Tags: []string{"Auth"},
	}, func(ctx context.Context, input *struct {
		Body struct {
			Email    string `json:"email"`
			Password string `json:"password"`
			TOTPCode string `json:"totp_code,omitempty"`
		}
	}) (*LoginOutput, error) {
		email := strings.TrimSpace(strings.ToLower(input.Body.Email))
		if email == "" || input.Body.Password == "" {
			return nil, httpx.Unauthorized("invalid credentials")
		}
		user, err := d.Store.GetUserByEmail(ctx, email)
		if errors.Is(err, store.ErrNotFound) {
			return nil, httpx.Unauthorized("invalid credentials")
		}
		if err != nil {
			return nil, httpx.Internal(err)
		}
		if !user.IsPlatformAdmin || !user.IsActive {
			return nil, httpx.Unauthorized("invalid credentials")
		}
		ok, err := auth.VerifyPassword(input.Body.Password, user.PasswordHash)
		if err != nil || !ok {
			return nil, httpx.Unauthorized("invalid credentials")
		}
		if user.TOTPEnabled {
			if input.Body.TOTPCode == "" {
				out := &LoginOutput{}
				out.Body.RequiresTOTP = true
				return out, nil
			}
			if user.TOTPSecret == nil || !auth.ValidateTOTPWithWindow(*user.TOTPSecret, input.Body.TOTPCode) {
				return nil, httpx.Unauthorized("invalid TOTP code")
			}
		}
		access, exp, err := d.Tokens.CreateAccessToken(user.ID, xid.Nil(), user.Email, "platform")
		if err != nil {
			return nil, httpx.Internal(err)
		}
		refresh, refreshExp, err := d.Tokens.CreateRefreshToken(user.ID)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		_ = d.Store.SaveRefreshToken(ctx, user.ID, hashToken(refresh), refreshExp)
		_ = d.Store.UpdateLastLogin(ctx, user.ID)

		out := &LoginOutput{}
		out.Body.AccessToken = access
		out.Body.ExpiresAt = exp
		user.PasswordHash = ""
		user.TOTPSecret = nil
		out.Body.User = *user
		out.SetCookie = http.Cookie{
			Name: "refresh_token", Value: refresh, Path: "/api/auth",
			HttpOnly: true, Secure: d.Config.AppEnv == "production",
			SameSite: http.SameSiteStrictMode, Expires: refreshExp,
		}
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "platform-list-tenants", Method: http.MethodGet, Path: "/api/platform/tenants",
		Summary: "List all tenants", Tags: []string{"Platform"},
		Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, _ *struct{}) (*struct {
		Body struct {
			Data []store.Tenant `json:"data"`
		}
	}, error) {
		if err := requirePlatform(ctx); err != nil {
			return nil, err
		}
		list, err := d.Store.ListTenants(ctx)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct {
			Body struct {
				Data []store.Tenant `json:"data"`
			}
		}{Body: struct {
			Data []store.Tenant `json:"data"`
		}{Data: list}}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "platform-create-tenant", Method: http.MethodPost, Path: "/api/platform/tenants",
		Summary: "Create tenant with admin", Tags: []string{"Platform"},
		Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Body struct {
			Slug     string `json:"slug" minLength:"2" maxLength:"64"`
			Name     string `json:"name" minLength:"2"`
			Email    string `json:"email"`
			Password string `json:"password" minLength:"8"`
			FullName string `json:"full_name"`
		}
	}) (*struct {
		Body struct {
			TenantID xid.ID `json:"tenant_id"`
			UserID   xid.ID `json:"user_id"`
			Slug     string `json:"slug"`
		}
	}, error) {
		if err := requirePlatform(ctx); err != nil {
			return nil, err
		}
		slug := strings.ToLower(strings.TrimSpace(input.Body.Slug))
		if !slugRE.MatchString(slug) {
			return nil, httpx.BadRequest("invalid slug (use lowercase letters, numbers, hyphens)")
		}
		hash, err := auth.HashPassword(input.Body.Password)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		tid, uid, err := d.Store.CreateTenantWithAdmin(ctx, slug, input.Body.Name, input.Body.Email, hash, input.Body.FullName)
		if err != nil {
			return nil, httpx.BadRequest(err.Error())
		}
		out := &struct {
			Body struct {
				TenantID xid.ID `json:"tenant_id"`
				UserID   xid.ID `json:"user_id"`
				Slug     string `json:"slug"`
			}
		}{}
		out.Body.TenantID = tid
		out.Body.UserID = uid
		out.Body.Slug = slug
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "platform-get-tenant", Method: http.MethodGet, Path: "/api/platform/tenants/{id}",
		Summary: "Get tenant by id", Tags: []string{"Platform"},
		Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID xid.ID `path:"id"`
	}) (*struct {
		Body store.Tenant
	}, error) {
		if err := requirePlatform(ctx); err != nil {
			return nil, err
		}
		t, err := d.Store.GetTenant(ctx, input.ID)
		if errors.Is(err, store.ErrNotFound) {
			return nil, httpx.NotFound("tenant not found")
		}
		if err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body store.Tenant }{Body: *t}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "platform-update-tenant", Method: http.MethodPut, Path: "/api/platform/tenants/{id}",
		Summary: "Update tenant", Tags: []string{"Platform"},
		Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID   xid.ID `path:"id"`
		Body struct {
			Name     string  `json:"name" minLength:"2"`
			Email    *string `json:"email,omitempty"`
			Phone    *string `json:"phone,omitempty"`
			IsActive bool    `json:"is_active"`
		}
	}) (*struct {
		Body store.Tenant
	}, error) {
		if err := requirePlatform(ctx); err != nil {
			return nil, err
		}
		t, err := d.Store.GetTenant(ctx, input.ID)
		if errors.Is(err, store.ErrNotFound) {
			return nil, httpx.NotFound("tenant not found")
		}
		if err != nil {
			return nil, httpx.Internal(err)
		}
		t.Name = strings.TrimSpace(input.Body.Name)
		if t.Name == "" {
			return nil, httpx.BadRequest("name is required")
		}
		t.Email = input.Body.Email
		t.Phone = input.Body.Phone
		t.IsActive = input.Body.IsActive
		if err := d.Store.UpdateTenant(ctx, t); err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body store.Tenant }{Body: *t}, nil
	})

	// Keep PATCH for partial updates (toggle active, etc.)
	huma.Register(api, huma.Operation{
		OperationID: "platform-patch-tenant", Method: http.MethodPatch, Path: "/api/platform/tenants/{id}",
		Summary: "Partial update tenant", Tags: []string{"Platform"},
		Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID   xid.ID `path:"id"`
		Body struct {
			Name     *string `json:"name,omitempty"`
			Email    *string `json:"email,omitempty"`
			Phone    *string `json:"phone,omitempty"`
			IsActive *bool   `json:"is_active,omitempty"`
		}
	}) (*struct {
		Body store.Tenant
	}, error) {
		if err := requirePlatform(ctx); err != nil {
			return nil, err
		}
		t, err := d.Store.GetTenant(ctx, input.ID)
		if errors.Is(err, store.ErrNotFound) {
			return nil, httpx.NotFound("tenant not found")
		}
		if err != nil {
			return nil, httpx.Internal(err)
		}
		if input.Body.Name != nil {
			t.Name = strings.TrimSpace(*input.Body.Name)
		}
		if input.Body.Email != nil {
			t.Email = input.Body.Email
		}
		if input.Body.Phone != nil {
			t.Phone = input.Body.Phone
		}
		if input.Body.IsActive != nil {
			t.IsActive = *input.Body.IsActive
		}
		if err := d.Store.UpdateTenant(ctx, t); err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body store.Tenant }{Body: *t}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "platform-delete-tenant", Method: http.MethodDelete, Path: "/api/platform/tenants/{id}",
		Summary: "Delete tenant and cascaded data", Tags: []string{"Platform"},
		Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID xid.ID `path:"id"`
	}) (*struct {
		Body map[string]string
	}, error) {
		if err := requirePlatform(ctx); err != nil {
			return nil, err
		}
		if err := d.Store.DeleteTenant(ctx, input.ID); errors.Is(err, store.ErrNotFound) {
			return nil, httpx.NotFound("tenant not found")
		} else if err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body map[string]string }{Body: map[string]string{"status": "deleted"}}, nil
	})
}
