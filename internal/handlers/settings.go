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

	"github.com/danielgtaylor/huma/v2"
	"github.com/dianrp/drp-billing/internal/auth"
	"github.com/dianrp/drp-billing/internal/httpx"
	"github.com/dianrp/drp-billing/internal/store"
	"github.com/dianrp/drp-billing/internal/tenant"
	"github.com/dianrp/drp-billing/internal/xid"
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
		return xid.Nil(), httpx.Unauthorized("role not found")
	}
	if !store.RoleHasPermission(role.Permissions, "settings") && !store.RoleHasPermission(role.Permissions, "*") {
		return xid.Nil(), httpx.Unauthorized("settings permission required")
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
	}, func(ctx context.Context, _ *struct{}) (*struct{ Body store.TenantBrandingView }, error) {
		tid, err := requireSettings(ctx, d)
		if err != nil {
			return nil, err
		}
		view, err := d.Store.ResolveTenantBranding(ctx, tid)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body store.TenantBrandingView }{Body: *view}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "put-tenant-branding", Method: http.MethodPut, Path: "/api/settings/branding",
		Tags: []string{"Settings"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Body struct {
			AppName      string  `json:"app_name"`
			LogoURL      *string `json:"logo_url,omitempty"`
			FaviconURL   *string `json:"favicon_url,omitempty"`
			ClearLogo    bool    `json:"clear_logo,omitempty"`
			ClearFavicon bool    `json:"clear_favicon,omitempty"`
		}
	}) (*struct{ Body store.TenantBrandingView }, error) {
		tid, err := requireSettings(ctx, d)
		if err != nil {
			return nil, err
		}
		raw, err := d.Store.GetTenantBrandingRaw(ctx, tid)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		b := &store.Branding{AppName: strings.TrimSpace(input.Body.AppName)}
		if input.Body.ClearLogo {
			b.LogoURL = nil
		} else if input.Body.LogoURL != nil {
			b.LogoURL = input.Body.LogoURL
		} else {
			b.LogoURL = raw.LogoURL
		}
		if input.Body.ClearFavicon {
			b.FaviconURL = nil
		} else if input.Body.FaviconURL != nil {
			b.FaviconURL = input.Body.FaviconURL
		} else {
			b.FaviconURL = raw.FaviconURL
		}
		if err := d.Store.UpdateTenantBranding(ctx, tid, b); err != nil {
			return nil, httpx.Internal(err)
		}
		view, err := d.Store.ResolveTenantBranding(ctx, tid)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body store.TenantBrandingView }{Body: *view}, nil
	})
}

func registerPlatformBranding(api huma.API, d *Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "get-platform-branding", Method: http.MethodGet, Path: "/api/platform/branding",
		Tags: []string{"Platform"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, _ *struct{}) (*struct{ Body store.Branding }, error) {
		if err := requirePlatform(ctx); err != nil {
			return nil, err
		}
		b, err := d.Store.GetPlatformBranding(ctx)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body store.Branding }{Body: *b}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "put-platform-branding", Method: http.MethodPut, Path: "/api/platform/branding",
		Tags: []string{"Platform"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Body struct {
			AppName      string  `json:"app_name"`
			LogoURL      *string `json:"logo_url,omitempty"`
			FaviconURL   *string `json:"favicon_url,omitempty"`
			ClearLogo    bool    `json:"clear_logo,omitempty"`
			ClearFavicon bool    `json:"clear_favicon,omitempty"`
		}
	}) (*struct{ Body store.Branding }, error) {
		if err := requirePlatform(ctx); err != nil {
			return nil, err
		}
		cur, err := d.Store.GetPlatformBranding(ctx)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		b := &store.Branding{AppName: strings.TrimSpace(input.Body.AppName)}
		if b.AppName == "" {
			b.AppName = "drp-billing"
		}
		if input.Body.ClearLogo {
			b.LogoURL = nil
		} else if input.Body.LogoURL != nil {
			b.LogoURL = input.Body.LogoURL
		} else {
			b.LogoURL = cur.LogoURL
		}
		if input.Body.ClearFavicon {
			b.FaviconURL = nil
		} else if input.Body.FaviconURL != nil {
			b.FaviconURL = input.Body.FaviconURL
		} else {
			b.FaviconURL = cur.FaviconURL
		}
		if err := d.Store.UpdatePlatformBranding(ctx, b); err != nil {
			return nil, httpx.Internal(err)
		}
		out, err := d.Store.GetPlatformBranding(ctx)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body store.Branding }{Body: *out}, nil
	})
}

// MountStaticAndUploads serves uploaded assets and registers multipart upload endpoints on chi.
func MountStaticAndUploads(r chi.Router, d *Deps) {
	_ = os.MkdirAll(d.Config.UploadDir, 0o755)
	r.Handle("/uploads/*", http.StripPrefix("/uploads/", http.FileServer(http.Dir(d.Config.UploadDir))))

	r.Post("/api/settings/branding/logo", uploadHandler(d, false, "logo"))
	r.Post("/api/settings/branding/favicon", uploadHandler(d, false, "favicon"))
	r.Post("/api/platform/branding/logo", uploadHandler(d, true, "logo"))
	r.Post("/api/platform/branding/favicon", uploadHandler(d, true, "favicon"))
}

func uploadHandler(d *Deps, platform bool, kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		var tid xid.ID
		if platform {
			if err := requirePlatform(ctx); err != nil {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
		} else {
			id, err := requireSettings(ctx, d)
			if err != nil {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
			tid = id
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
		url, err := saveUpload(d, tid, platform, kind, file, header.Filename, header.Size)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadRequest)
			return
		}
		if platform {
			cur, _ := d.Store.GetPlatformBranding(ctx)
			if cur == nil {
				cur = &store.Branding{AppName: "drp-billing"}
			}
			if kind == "logo" {
				cur.LogoURL = &url
			} else {
				cur.FaviconURL = &url
			}
			_ = d.Store.UpdatePlatformBranding(ctx, cur)
		} else {
			raw, err := d.Store.GetTenantBrandingRaw(ctx, tid)
			if err != nil {
				http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
				return
			}
			if kind == "logo" {
				raw.LogoURL = &url
			} else {
				raw.FaviconURL = &url
			}
			_ = d.Store.UpdateTenantBranding(ctx, tid, raw)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"url": url})
	}
}

func saveUpload(d *Deps, tenantID xid.ID, platform bool, kind string, src io.Reader, filename string, size int64) (string, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".webp", ".gif", ".ico", ".svg":
	default:
		return "", fmt.Errorf("tipe file tidak didukung")
	}
	if size > 2<<20 {
		return "", fmt.Errorf("file terlalu besar (max 2MB)")
	}
	subdir := "platform"
	if !platform {
		subdir = tenantID.String()
	}
	dir := filepath.Join(d.Config.UploadDir, subdir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("gagal buat folder upload")
	}
	name := kind + ext
	dst := filepath.Join(dir, name)
	out, err := os.Create(dst)
	if err != nil {
		return "", err
	}
	defer out.Close()
	if _, err := io.Copy(out, src); err != nil {
		return "", err
	}
	return "/uploads/" + subdir + "/" + name, nil
}

func registerRolesUsers(api huma.API, d *Deps) {
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
		return &struct{}{}, nil
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
