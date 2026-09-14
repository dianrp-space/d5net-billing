package handlers

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/dianrp-space/d5net-billing/internal/audit"
	"github.com/dianrp-space/d5net-billing/internal/auth"
	"github.com/dianrp-space/d5net-billing/internal/billing"
	"github.com/dianrp-space/d5net-billing/internal/config"
	"github.com/dianrp-space/d5net-billing/internal/dbbackup"
	"github.com/dianrp-space/d5net-billing/internal/httpx"
	"github.com/dianrp-space/d5net-billing/internal/job"
	"github.com/dianrp-space/d5net-billing/internal/notify"
	"github.com/dianrp-space/d5net-billing/internal/payment"
	"github.com/dianrp-space/d5net-billing/internal/provision"
	"github.com/dianrp-space/d5net-billing/internal/provisioner"
	"github.com/dianrp-space/d5net-billing/internal/store"
	"github.com/dianrp-space/d5net-billing/internal/tenant"
	"github.com/dianrp-space/d5net-billing/internal/xid"
)

type Deps struct {
	Store       *store.Store
	Tokens      *auth.TokenService
	Encryptor   *auth.Encryptor
	Billing     *billing.Engine
	Notify      *notify.Service
	Payments    *payment.Registry
	Provisioner *provisioner.Registry
	Config      *config.Config
	DBBackup    *dbbackup.Service
	Audit       *audit.Logger
	Jobs        JobRunner
}

// JobRunner runs a tenant worker cycle on demand (API process; no ticker).
type JobRunner interface {
	RunTenantNow(ctx context.Context, tenantID xid.ID) (job.TenantCycleResult, error)
}

func RegisterAll(api huma.API, d *Deps) {
	httpx.RegisterHealth(api)
	registerAuth(api, d)
	registerPublicBranding(api, d)
	registerSettings(api, d)
	registerIsolirSettings(api, d)
	registerJobsSettings(api, d)
	registerInvoiceSettings(api, d)
	registerPublicIsolir(api, d)
	registerNotifications(api, d)
	registerIntegrations(api, d)
	registerDBBackup(api, d)
	registerCustomers(api, d)
	registerClusters(api, d)
	registerPlans(api, d)
	registerPlanOffers(api, d)
	registerPlanDiscounts(api, d)
	registerSubscriptions(api, d)
	registerRouters(api, d)
	registerIPAM(api, d)
	registerInvoices(api, d)
	registerTickets(api, d)
	registerODP(api, d)
	registerCableRoutes(api, d)
	registerDashboard(api, d)
	registerSearch(api, d)
	registerAlerts(api, d)
	registerPortal(api, d)
	registerWallet(api, d)
	registerWebhooks(api, d)
	registerVouchers(api, d)
	registerReports(api, d)
	registerAccounting(api, d)
	registerAdvanced(api, d)
	registerOpsExtra(api, d)
}

type loginBody struct {
	Email      string `json:"email"`
	Password   string `json:"password"`
	TOTPCode   string `json:"totp_code,omitempty"`
	TenantID   xid.ID `json:"tenant_id,omitempty"`
	TenantSlug string `json:"tenant_slug,omitempty"`
}

type LoginInput struct {
	RawBody []byte
	Body    loginBody
}

type LoginOutput struct {
	SetCookie http.Cookie `header:"Set-Cookie"`
	Body      struct {
		AccessToken  string             `json:"access_token"`
		ExpiresAt    time.Time          `json:"expires_at"`
		User         store.User         `json:"user"`
		Tenants      []store.UserTenant `json:"tenants"`
		RequiresTOTP bool               `json:"requires_totp,omitempty"`
	}
}

func registerAuth(api huma.API, d *Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "login", Method: http.MethodPost, Path: "/api/auth/login",
		Summary: "Login admin/user", Tags: []string{"Auth"},
	}, func(ctx context.Context, input *LoginInput) (*LoginOutput, error) {
		body := input.Body
		if body.Email == "" && len(input.RawBody) > 0 {
			_ = json.Unmarshal(input.RawBody, &body)
		}
		body.Email = strings.TrimSpace(strings.ToLower(body.Email))
		if body.Email == "" || body.Password == "" {
			return nil, httpx.Unauthorized("invalid credentials")
		}
		user, err := d.Store.GetUserByEmail(ctx, body.Email)
		if errors.Is(err, store.ErrNotFound) {
			return nil, httpx.Unauthorized("invalid credentials")
		}
		if err != nil {
			return nil, httpx.Internal(err)
		}
		ok, err := auth.VerifyPassword(body.Password, user.PasswordHash)
		if err != nil || !ok {
			auditEvent(ctx, d, AuditAuthLoginFailed, "user", &user.ID, map[string]any{"email": body.Email})
			return nil, httpx.Unauthorized("invalid credentials")
		}
		if user.TOTPEnabled {
			if body.TOTPCode == "" {
				out := &LoginOutput{}
				out.Body.RequiresTOTP = true
				return out, nil
			}
			if user.TOTPSecret == nil || !auth.ValidateTOTPWithWindow(*user.TOTPSecret, body.TOTPCode) {
				return nil, httpx.Unauthorized("invalid TOTP code")
			}
		}
		tenants, err := d.Store.ListUserTenants(ctx, user.ID)
		if err != nil || len(tenants) == 0 {
			return nil, httpx.Unauthorized("no tenant access")
		}
		slug := strings.ToLower(strings.TrimSpace(body.TenantSlug))
		tid := tenants[0].TenantID
		role := tenants[0].RoleSlug
		if slug != "" {
			matched := false
			for _, t := range tenants {
				if strings.EqualFold(t.TenantSlug, slug) {
					tid = t.TenantID
					role = t.RoleSlug
					matched = true
					break
				}
			}
			if !matched {
				return nil, httpx.Unauthorized("no access to this tenant")
			}
		} else if !xid.IsNil(body.TenantID) {
			matched := false
			for _, t := range tenants {
				if t.TenantID == body.TenantID {
					tid = t.TenantID
					role = t.RoleSlug
					matched = true
					break
				}
			}
			if !matched {
				return nil, httpx.Unauthorized("no access to this tenant")
			}
		}
		ten, err := d.Store.GetTenant(ctx, tid)
		if err != nil || !ten.IsActive {
			return nil, httpx.Unauthorized("tenant inactive")
		}
		access, exp, err := d.Tokens.CreateAccessToken(user.ID, tid, user.Email, role)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		refresh, refreshExp, err := d.Tokens.CreateRefreshToken(user.ID)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		hash := hashToken(refresh)
		_ = d.Store.SaveRefreshToken(ctx, user.ID, hash, refreshExp)
		_ = d.Store.UpdateLastLogin(ctx, user.ID)
		loginCtx := tenant.WithInfo(ctx, tenant.Info{ID: tid, Role: role, UserID: user.ID})
		auditEvent(loginCtx, d, AuditAuthLogin, "user", &user.ID, map[string]any{"email": user.Email, "role": role})

		out := &LoginOutput{}
		out.Body.AccessToken = access
		out.Body.ExpiresAt = exp
		user.PasswordHash = ""
		user.TOTPSecret = nil
		out.Body.User = *user
		out.Body.Tenants = tenants
		out.SetCookie = http.Cookie{
			Name: "refresh_token", Value: refresh, Path: "/api/auth",
			HttpOnly: true, Secure: d.Config.AppEnv == "production",
			SameSite: http.SameSiteStrictMode, Expires: refreshExp,
		}
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "refresh-token", Method: http.MethodPost, Path: "/api/auth/refresh",
		Summary: "Refresh access token using HttpOnly refresh cookie", Tags: []string{"Auth"},
	}, func(ctx context.Context, input *struct {
		RefreshToken string `cookie:"refresh_token"`
		Body         struct {
			TenantSlug string `json:"tenant_slug,omitempty"`
		}
	}) (*LoginOutput, error) {
		raw := strings.TrimSpace(input.RefreshToken)
		if raw == "" {
			return nil, httpx.Unauthorized("missing refresh token")
		}
		claims, err := d.Tokens.ParseToken(raw)
		if err != nil || claims.Type != "refresh" {
			return nil, httpx.Unauthorized("invalid refresh token")
		}
		ok, err := d.Store.IsRefreshTokenValid(ctx, hashToken(raw))
		if err != nil {
			return nil, httpx.Internal(err)
		}
		if !ok {
			return nil, httpx.Unauthorized("invalid refresh token")
		}
		uid, err := xid.Parse(claims.UserID)
		if err != nil {
			return nil, httpx.Unauthorized("invalid refresh token")
		}
		user, err := d.Store.GetUserByID(ctx, uid)
		if errors.Is(err, store.ErrNotFound) || (err == nil && !user.IsActive) {
			return nil, httpx.Unauthorized("invalid refresh token")
		}
		if err != nil {
			return nil, httpx.Internal(err)
		}

		var tid xid.ID
		role := ""
		tenants, err := d.Store.ListUserTenants(ctx, user.ID)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		if len(tenants) == 0 {
			return nil, httpx.Unauthorized("no tenant access")
		}
		slug := strings.ToLower(strings.TrimSpace(input.Body.TenantSlug))
		tid = tenants[0].TenantID
		role = tenants[0].RoleSlug
		if slug != "" {
			matched := false
			for _, t := range tenants {
				if strings.EqualFold(t.TenantSlug, slug) {
					tid = t.TenantID
					role = t.RoleSlug
					matched = true
					break
				}
			}
			if !matched {
				return nil, httpx.Unauthorized("no access to this tenant")
			}
		}
		ten, err := d.Store.GetTenant(ctx, tid)
		if err != nil || !ten.IsActive {
			return nil, httpx.Unauthorized("tenant inactive")
		}

		access, exp, err := d.Tokens.CreateAccessToken(user.ID, tid, user.Email, role)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		_ = d.Store.RevokeRefreshToken(ctx, hashToken(raw))
		refresh, refreshExp, err := d.Tokens.CreateRefreshToken(user.ID)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		_ = d.Store.SaveRefreshToken(ctx, user.ID, hashToken(refresh), refreshExp)

		out := &LoginOutput{}
		out.Body.AccessToken = access
		out.Body.ExpiresAt = exp
		user.PasswordHash = ""
		user.TOTPSecret = nil
		out.Body.User = *user
		out.Body.Tenants = tenants
		out.SetCookie = http.Cookie{
			Name: "refresh_token", Value: refresh, Path: "/api/auth",
			HttpOnly: true, Secure: d.Config.AppEnv == "production",
			SameSite: http.SameSiteStrictMode, Expires: refreshExp,
		}
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "me", Method: http.MethodGet, Path: "/api/me",
		Summary: "Current user + role permissions for this tenant", Tags: []string{"Auth"},
		Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, _ *struct{}) (*struct {
		Body struct {
			UserID      xid.ID   `json:"user_id"`
			Email       string   `json:"email"`
			FullName    string   `json:"full_name"`
			AvatarURL   *string  `json:"avatar_url,omitempty"`
			RoleSlug    string   `json:"role_slug"`
			RoleName    string   `json:"role_name"`
			Permissions []string `json:"permissions"`
			TenantID    xid.ID   `json:"tenant_id"`
		}
	}, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		uid := userIDFromCtx(ctx)
		if xid.IsNil(uid) {
			return nil, httpx.Unauthorized("unauthorized")
		}
		user, err := d.Store.GetUserByID(ctx, uid)
		if errors.Is(err, store.ErrNotFound) {
			return nil, httpx.Unauthorized("unauthorized")
		}
		if err != nil {
			return nil, httpx.Internal(err)
		}
		roleSlug := roleSlugFromCtx(ctx)
		roleName := roleSlug
		perms := []string{}
		if roleSlug != "" {
			role, rerr := d.Store.GetRoleBySlug(ctx, tid, roleSlug)
			if rerr == nil && role != nil {
				roleName = role.Name
				if role.Permissions != nil {
					perms = role.Permissions
				}
			}
		}
		return &struct {
			Body struct {
				UserID      xid.ID   `json:"user_id"`
				Email       string   `json:"email"`
				FullName    string   `json:"full_name"`
				AvatarURL   *string  `json:"avatar_url,omitempty"`
				RoleSlug    string   `json:"role_slug"`
				RoleName    string   `json:"role_name"`
				Permissions []string `json:"permissions"`
				TenantID    xid.ID   `json:"tenant_id"`
			}
		}{Body: struct {
			UserID      xid.ID   `json:"user_id"`
			Email       string   `json:"email"`
			FullName    string   `json:"full_name"`
			AvatarURL   *string  `json:"avatar_url,omitempty"`
			RoleSlug    string   `json:"role_slug"`
			RoleName    string   `json:"role_name"`
			Permissions []string `json:"permissions"`
			TenantID    xid.ID   `json:"tenant_id"`
		}{
			UserID: uid, Email: user.Email, FullName: user.FullName, AvatarURL: user.AvatarURL,
			RoleSlug: roleSlug, RoleName: roleName, Permissions: perms, TenantID: tid,
		}}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "update-me", Method: http.MethodPut, Path: "/api/me",
		Summary: "Update own profile (name, password)", Tags: []string{"Auth"},
		Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Body struct {
			FullName        string `json:"full_name"`
			CurrentPassword string `json:"current_password,omitempty"`
			NewPassword     string `json:"new_password,omitempty"`
			ClearAvatar     bool   `json:"clear_avatar,omitempty"`
		}
	}) (*struct {
		Body struct {
			UserID    xid.ID  `json:"user_id"`
			Email     string  `json:"email"`
			FullName  string  `json:"full_name"`
			AvatarURL *string `json:"avatar_url,omitempty"`
		}
	}, error) {
		uid := userIDFromCtx(ctx)
		if xid.IsNil(uid) {
			return nil, httpx.Unauthorized("unauthorized")
		}
		user, err := d.Store.GetUserByID(ctx, uid)
		if errors.Is(err, store.ErrNotFound) {
			return nil, httpx.Unauthorized("unauthorized")
		}
		if err != nil {
			return nil, httpx.Internal(err)
		}
		fullName := strings.TrimSpace(input.Body.FullName)
		if fullName == "" {
			return nil, httpx.BadRequest("nama wajib diisi")
		}
		var newHash *string
		if pw := strings.TrimSpace(input.Body.NewPassword); pw != "" {
			if len(pw) < 8 {
				return nil, httpx.BadRequest("password baru minimal 8 karakter")
			}
			ok, verr := auth.VerifyPassword(input.Body.CurrentPassword, user.PasswordHash)
			if verr != nil || !ok {
				return nil, httpx.BadRequest("password saat ini salah")
			}
			hash, herr := auth.HashPassword(pw)
			if herr != nil {
				return nil, httpx.Internal(herr)
			}
			newHash = &hash
		}
		if err := d.Store.UpdateMyProfile(ctx, uid, fullName, newHash); err != nil {
			return nil, httpx.Internal(err)
		}
		avatar := user.AvatarURL
		if input.Body.ClearAvatar {
			if err := d.Store.UpdateUserAvatar(ctx, uid, nil); err != nil {
				return nil, httpx.Internal(err)
			}
			avatar = nil
		}
		return &struct {
			Body struct {
				UserID    xid.ID  `json:"user_id"`
				Email     string  `json:"email"`
				FullName  string  `json:"full_name"`
				AvatarURL *string `json:"avatar_url,omitempty"`
			}
		}{Body: struct {
			UserID    xid.ID  `json:"user_id"`
			Email     string  `json:"email"`
			FullName  string  `json:"full_name"`
			AvatarURL *string `json:"avatar_url,omitempty"`
		}{
			UserID: uid, Email: user.Email, FullName: fullName, AvatarURL: avatar,
		}}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "register-tenant", Method: http.MethodPost, Path: "/api/auth/register",
		Summary: "Deprecated: use POST /api/platform/tenants", Tags: []string{"Auth"},
		Deprecated: true,
	}, func(ctx context.Context, input *struct {
		Body struct {
			Slug     string `json:"slug" minLength:"2"`
			Name     string `json:"name" minLength:"2"`
			Email    string `json:"email"`
			Password string `json:"password" minLength:"8"`
			FullName string `json:"full_name"`
		}
	}) (*struct {
		Body struct {
			TenantID xid.ID `json:"tenant_id"`
			UserID   xid.ID `json:"user_id"`
		}
	}, error) {
		return nil, httpx.BadRequest("public registration disabled; use drpctl create-tenant")
	})
}

func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

func tenantIDFromCtx(ctx context.Context) (xid.ID, error) {
	t, ok := tenant.FromContext(ctx)
	if !ok || xid.IsNil(t.ID) {
		return xid.Nil(), huma.Error401Unauthorized("unauthorized")
	}
	return t.ID, nil
}

func roleSlugFromCtx(ctx context.Context) string {
	t, ok := tenant.FromContext(ctx)
	if !ok {
		return ""
	}
	return t.Role
}

func userIDFromCtx(ctx context.Context) xid.ID {
	t, ok := tenant.FromContext(ctx)
	if !ok {
		return xid.Nil()
	}
	return t.UserID
}

// filterTenantUploadURLs keeps only paths under this tenant's /uploads/{tenantID}/ tree.
func filterTenantUploadURLs(tenantID xid.ID, urls []string) []string {
	if len(urls) == 0 {
		return nil
	}
	prefix := "/uploads/" + tenantID.String() + "/"
	out := make([]string, 0, len(urls))
	seen := map[string]struct{}{}
	for _, u := range urls {
		u = strings.TrimSpace(u)
		if u == "" || !strings.HasPrefix(u, prefix) {
			continue
		}
		if _, ok := seen[u]; ok {
			continue
		}
		seen[u] = struct{}{}
		out = append(out, u)
	}
	return out
}

func rolePermissionsFromCtx(ctx context.Context, d *Deps) ([]string, error) {
	tid, err := tenantIDFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	slug := roleSlugFromCtx(ctx)
	if slug == "" {
		return []string{}, nil
	}
	role, err := d.Store.GetRoleBySlug(ctx, tid, slug)
	if err != nil {
		if slug == "admin" {
			return []string{"*"}, nil
		}
		return []string{}, nil
	}
	if role.Permissions == nil {
		return []string{}, nil
	}
	return role.Permissions, nil
}

func requireDispatchOps(ctx context.Context, d *Deps) error {
	perms, err := rolePermissionsFromCtx(ctx, d)
	if err != nil {
		return err
	}
	if store.CanDispatchOps(perms) {
		return nil
	}
	if roleSlugFromCtx(ctx) == "admin" {
		return nil
	}
	return httpx.Forbidden("hanya admin/dispatcher yang boleh membuat atau assign")
}

func isFieldOps(ctx context.Context, d *Deps) bool {
	perms, err := rolePermissionsFromCtx(ctx, d)
	if err != nil {
		return false
	}
	if roleSlugFromCtx(ctx) == "teknisi" && !store.CanDispatchOps(perms) {
		return true
	}
	return store.IsFieldOpsRole(perms)
}

func enforceTicketAssigned(ctx context.Context, d *Deps, t *store.Ticket) error {
	if !isFieldOps(ctx, d) {
		return nil
	}
	uid := userIDFromCtx(ctx)
	if t.AssignedTo == nil || xid.IsNil(*t.AssignedTo) || *t.AssignedTo != uid {
		return httpx.Forbidden("tiket ini tidak di-assign ke Anda")
	}
	return nil
}

func fieldOpsTechnicianFilter(ctx context.Context, d *Deps, tid xid.ID) (mineOnly bool, techID *xid.ID, err error) {
	if !isFieldOps(ctx, d) {
		return false, nil, nil
	}
	uid := userIDFromCtx(ctx)
	if xid.IsNil(uid) {
		return true, nil, httpx.Forbidden("unauthorized")
	}
	tech, err := d.Store.GetTechnicianByUserID(ctx, tid, uid)
	if errors.Is(err, store.ErrNotFound) {
		return true, nil, nil
	}
	if err != nil {
		return true, nil, httpx.Internal(err)
	}
	id := tech.ID
	return true, &id, nil
}

func enforceLeadAssigned(ctx context.Context, d *Deps, lead *store.Lead) error {
	if !isFieldOps(ctx, d) {
		return nil
	}
	uid := userIDFromCtx(ctx)
	if lead.AssignedTo == nil || xid.IsNil(*lead.AssignedTo) || *lead.AssignedTo != uid {
		return httpx.Forbidden("lead ini tidak di-assign ke Anda")
	}
	return nil
}

func enforceWorkOrderAssigned(ctx context.Context, d *Deps, tid xid.ID, wo *store.WorkOrder) error {
	mineOnly, techID, err := fieldOpsTechnicianFilter(ctx, d, tid)
	if err != nil {
		return err
	}
	if !mineOnly {
		return nil
	}
	if techID == nil || wo.TechnicianID == nil || xid.IsNil(*wo.TechnicianID) || *wo.TechnicianID != *techID {
		return httpx.Forbidden("work order ini tidak di-assign ke Anda")
	}
	return nil
}

// optionalQueryID parses an optional UUID query string (Huma rejects *xid.ID on query params).
func optionalQueryID(raw string) (*xid.ID, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	id, err := xid.Parse(raw)
	if err != nil {
		return nil, httpx.BadRequest("invalid id: " + raw)
	}
	return &id, nil
}

func trimPtr(s *string) *string {
	if s == nil {
		return nil
	}
	v := strings.TrimSpace(*s)
	if v == "" {
		return nil
	}
	return &v
}

func normalizeIdentityType(s *string) *string {
	v := trimPtr(s)
	if v == nil {
		return nil
	}
	switch strings.ToLower(*v) {
	case "ktp", "sim", "passport", "other":
		out := strings.ToLower(*v)
		return &out
	default:
		out := "other"
		return &out
	}
}

func registerCustomers(api huma.API, d *Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "list-customers", Method: http.MethodGet, Path: "/api/customers",
		Summary: "List customers", Tags: []string{"Customers"},
		Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Search    string `query:"search"`
		ClusterID string `query:"cluster_id"`
		IsActive  string `query:"is_active"`
		Status    string `query:"status"`
		Limit     int    `query:"limit" minimum:"1" maximum:"1000"`
		Offset    int    `query:"offset" minimum:"0"`
	}) (*struct {
		Body struct {
			Data  []store.Customer `json:"data"`
			Total int64            `json:"total"`
		}
	}, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		clusterID, err := optionalQueryID(input.ClusterID)
		if err != nil {
			return nil, err
		}
		var isActive *bool
		switch strings.ToLower(strings.TrimSpace(input.IsActive)) {
		case "true", "1", "aktif", "active":
			v := true
			isActive = &v
		case "false", "0", "nonaktif", "inactive":
			v := false
			isActive = &v
		}
		list, total, err := d.Store.ListCustomers(ctx, store.CustomerFilter{
			TenantID: tid, ClusterID: clusterID, Search: input.Search, IsActive: isActive,
			ServiceStatus: strings.TrimSpace(input.Status), Limit: input.Limit, Offset: input.Offset,
		})
		if err != nil {
			return nil, httpx.Internal(err)
		}
		out := &struct {
			Body struct {
				Data  []store.Customer `json:"data"`
				Total int64            `json:"total"`
			}
		}{}
		out.Body.Data = list
		out.Body.Total = total
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "create-customer", Method: http.MethodPost, Path: "/api/customers",
		Summary: "Create customer", Tags: []string{"Customers"},
		Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Body struct {
			ClusterID       *xid.ID  `json:"cluster_id,omitempty"`
			CustomerCode    *string  `json:"customer_code,omitempty"`
			FullName        string   `json:"full_name"`
			Email           *string  `json:"email,omitempty"`
			Phone           string   `json:"phone"`
			Address         *string  `json:"address,omitempty"`
			Latitude        *float64 `json:"latitude,omitempty"`
			Longitude       *float64 `json:"longitude,omitempty"`
			IdentityType    *string  `json:"identity_type,omitempty"`
			IdentityNumber  *string  `json:"identity_number,omitempty"`
			IsActive        *bool    `json:"is_active,omitempty"`
			PortalEnabled   *bool    `json:"portal_enabled,omitempty"`
			Status          string   `json:"status,omitempty"`
			ResellerID      *xid.ID  `json:"reseller_id,omitempty"`
			SalesUserID     *xid.ID  `json:"sales_user_id,omitempty"`
			CommissionBasis string   `json:"commission_basis,omitempty"`
		}
	}) (*struct{ Body store.Customer }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		name := strings.TrimSpace(input.Body.FullName)
		phone := strings.TrimSpace(input.Body.Phone)
		if name == "" || phone == "" {
			return nil, httpx.BadRequest("full_name and phone are required")
		}

		var code string
		custom := ""
		if input.Body.CustomerCode != nil {
			custom = strings.TrimSpace(*input.Body.CustomerCode)
		}
		if custom != "" {
			code = custom
		} else if input.Body.ClusterID != nil {
			code, err = d.Store.NextCustomerCodeForCluster(ctx, tid, *input.Body.ClusterID)
			if errors.Is(err, store.ErrNotFound) {
				return nil, httpx.BadRequest("cluster not found")
			}
			if err != nil {
				return nil, httpx.Internal(err)
			}
		} else {
			code, err = d.Store.NextCustomerCode(ctx, tid)
			if err != nil {
				return nil, httpx.Internal(err)
			}
		}

		active := true
		if input.Body.IsActive != nil {
			active = *input.Body.IsActive
		}
		switch normalizeCustomerStatus(input.Body.Status) {
		case "inactive":
			active = false
		case "active", "isolir":
			active = true
		}
		portal := true
		if input.Body.PortalEnabled != nil {
			portal = *input.Body.PortalEnabled
		}

		rID, sID := store.NormalizeAttribution(input.Body.ResellerID, input.Body.SalesUserID)
		c := &store.Customer{
			TenantID: tid, ClusterID: input.Body.ClusterID, CustomerCode: code,
			FullName: name, Email: input.Body.Email, Phone: phone, Address: input.Body.Address,
			Latitude: input.Body.Latitude, Longitude: input.Body.Longitude,
			IdentityType: normalizeIdentityType(input.Body.IdentityType), IdentityNumber: trimPtr(input.Body.IdentityNumber),
			IsActive: active, PortalEnabled: portal,
			ResellerID: rID, SalesUserID: sID,
		}
		hash, err := auth.HashPassword(phone)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		c.PasswordHash = hash
		if err := d.Store.CreateCustomer(ctx, c); err != nil {
			return nil, httpx.Internal(err)
		}
		_, _ = d.Store.CreditCommission(ctx, tid, c.ID, nil, rID, sID, input.Body.CommissionBasis)
		full, gerr := d.Store.GetCustomer(ctx, tid, c.ID)
		if gerr == nil {
			return &struct{ Body store.Customer }{Body: *full}, nil
		}
		return &struct{ Body store.Customer }{Body: *c}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-customer", Method: http.MethodGet, Path: "/api/customers/{id}",
		Tags: []string{"Customers"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID xid.ID `path:"id"`
	}) (*struct{ Body store.Customer }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		c, err := d.Store.GetCustomer(ctx, tid, input.ID)
		if errors.Is(err, store.ErrNotFound) {
			return nil, httpx.NotFound("customer not found")
		}
		if err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body store.Customer }{Body: *c}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "dismantle-customer", Method: http.MethodPost, Path: "/api/customers/{id}/dismantle",
		Summary: "Cabut pelanggan (hentikan layanan)", Tags: []string{"Customers"},
		Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID xid.ID `path:"id"`
	}) (*struct{ Body store.Customer }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		c, err := d.Store.GetCustomer(ctx, tid, input.ID)
		if errors.Is(err, store.ErrNotFound) {
			return nil, httpx.NotFound("customer not found")
		}
		if err != nil {
			return nil, httpx.Internal(err)
		}
		if c.IsDismantled() {
			return nil, httpx.BadRequest("pelanggan sudah cabut")
		}
		if err := dismantleCustomerServices(ctx, d, tid, c.ID); err != nil {
			return nil, httpx.Internal(err)
		}
		if err := d.Store.DismantleCustomer(ctx, tid, c.ID); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return nil, httpx.BadRequest("pelanggan sudah cabut")
			}
			return nil, httpx.Internal(err)
		}
		if d.Notify != nil {
			_ = d.Notify.QueueTenantTelegram(ctx, tid, notify.OpsMsg("cabut", "Pelanggan cabut",
				c.CustomerCode+" — "+c.FullName,
			))
		}
		out, err := d.Store.GetCustomer(ctx, tid, c.ID)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		auditEvent(ctx, d, AuditCustomerDismantle, "customer", &c.ID, map[string]any{
			"customer_code": c.CustomerCode, "full_name": c.FullName,
		})
		return &struct{ Body store.Customer }{Body: *out}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "update-customer", Method: http.MethodPut, Path: "/api/customers/{id}",
		Tags: []string{"Customers"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID   xid.ID `path:"id"`
		Body struct {
			ClusterID      *xid.ID  `json:"cluster_id,omitempty"`
			CustomerCode   string   `json:"customer_code"`
			FullName       string   `json:"full_name"`
			Email          *string  `json:"email,omitempty"`
			Phone          string   `json:"phone"`
			Address        *string  `json:"address,omitempty"`
			Latitude       *float64 `json:"latitude,omitempty"`
			Longitude      *float64 `json:"longitude,omitempty"`
			IdentityType   *string  `json:"identity_type,omitempty"`
			IdentityNumber *string  `json:"identity_number,omitempty"`
			IsActive       bool     `json:"is_active"`
			PortalEnabled  bool     `json:"portal_enabled"`
			Status         string   `json:"status,omitempty"`
			ResellerID     *xid.ID  `json:"reseller_id,omitempty"`
			SalesUserID    *xid.ID  `json:"sales_user_id,omitempty"`
		}
	}) (*struct{ Body store.Customer }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		existing, err := d.Store.GetCustomer(ctx, tid, input.ID)
		if errors.Is(err, store.ErrNotFound) {
			return nil, httpx.NotFound("customer not found")
		}
		if err != nil {
			return nil, httpx.Internal(err)
		}
		name := strings.TrimSpace(input.Body.FullName)
		phone := strings.TrimSpace(input.Body.Phone)
		code := strings.TrimSpace(input.Body.CustomerCode)
		if name == "" || phone == "" || code == "" {
			return nil, httpx.BadRequest("full_name, phone, and customer_code are required")
		}
		if input.Body.ClusterID != nil {
			if _, err := d.Store.GetCluster(ctx, tid, *input.Body.ClusterID); errors.Is(err, store.ErrNotFound) {
				return nil, httpx.BadRequest("cluster not found")
			} else if err != nil {
				return nil, httpx.Internal(err)
			}
		}
		existing.ClusterID = input.Body.ClusterID
		existing.CustomerCode = code
		existing.FullName = name
		existing.Email = input.Body.Email
		phoneChanged := existing.Phone != phone
		existing.Phone = phone
		existing.Address = input.Body.Address
		existing.Latitude = input.Body.Latitude
		existing.Longitude = input.Body.Longitude
		existing.IdentityType = normalizeIdentityType(input.Body.IdentityType)
		existing.IdentityNumber = trimPtr(input.Body.IdentityNumber)
		if existing.IsDismantled() {
			existing.IsActive = false
			existing.PortalEnabled = false
		} else {
			existing.IsActive = input.Body.IsActive
			existing.PortalEnabled = input.Body.PortalEnabled
		}
		existing.ResellerID, existing.SalesUserID = store.NormalizeAttribution(input.Body.ResellerID, input.Body.SalesUserID)
		if phoneChanged {
			hash, err := auth.HashPassword(phone)
			if err != nil {
				return nil, httpx.Internal(err)
			}
			existing.PasswordHash = hash
		}
		if err := d.Store.UpdateCustomer(ctx, existing); err != nil {
			return nil, httpx.Internal(err)
		}
		if want := normalizeCustomerStatus(input.Body.Status); want != "" && want != existing.ServiceStatus && !existing.IsDismantled() {
			if err := applyCustomerStatus(ctx, d, tid, existing.ID, want); err != nil {
				return nil, err
			}
		}
		full, err := d.Store.GetCustomer(ctx, tid, existing.ID)
		if err != nil {
			return &struct{ Body store.Customer }{Body: *existing}, nil
		}
		return &struct{ Body store.Customer }{Body: *full}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "batch-customer-status", Method: http.MethodPost, Path: "/api/customers/batch-status",
		Tags: []string{"Customers"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Body struct {
			IDs      []xid.ID `json:"ids"`
			IsActive bool     `json:"is_active"`
		}
	}) (*struct {
		Body struct {
			Updated           int `json:"updated"`
			SkippedDismantled int `json:"skipped_dismantled"`
			Failed            int `json:"failed"`
		}
	}, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		if len(input.Body.IDs) == 0 {
			return nil, httpx.BadRequest("minimal satu pelanggan")
		}
		if len(input.Body.IDs) > 500 {
			return nil, httpx.BadRequest("maksimal 500 pelanggan per batch")
		}
		out := &struct {
			Body struct {
				Updated           int `json:"updated"`
				SkippedDismantled int `json:"skipped_dismantled"`
				Failed            int `json:"failed"`
			}
		}{}
		for _, id := range input.Body.IDs {
			cust, gerr := d.Store.GetCustomer(ctx, tid, id)
			if gerr != nil {
				out.Body.Failed++
				continue
			}
			if cust.IsDismantled() {
				out.Body.SkippedDismantled++
				continue
			}
			cust.IsActive = input.Body.IsActive
			if uerr := d.Store.UpdateCustomer(ctx, cust); uerr != nil {
				out.Body.Failed++
				continue
			}
			out.Body.Updated++
		}
		auditEvent(ctx, d, AuditCustomerStatus, "customer", nil, map[string]any{
			"is_active": input.Body.IsActive, "updated": out.Body.Updated,
			"skipped_dismantled": out.Body.SkippedDismantled, "failed": out.Body.Failed,
		})
		return out, nil
	})
	huma.Register(api, huma.Operation{
		OperationID: "delete-customer", Method: http.MethodDelete, Path: "/api/customers/{id}",
		Tags: []string{"Customers"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID xid.ID `path:"id"`
	}) (*struct{ Body map[string]string }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		c, err := d.Store.GetCustomer(ctx, tid, input.ID)
		if errors.Is(err, store.ErrNotFound) {
			return nil, httpx.NotFound("customer not found")
		}
		if err != nil {
			return nil, httpx.Internal(err)
		}
		// Guard: cabut dulu supaya secret router, port ODP, IP assignment, dan
		// langganan tidak tertinggal sebelum baris pelanggan dihapus permanen.
		if err := dismantleCustomerServices(ctx, d, tid, c.ID); err != nil {
			return nil, httpx.Internal(err)
		}
		if !c.IsDismantled() {
			_ = d.Store.DismantleCustomer(ctx, tid, c.ID)
		}
		if err := d.Store.DeleteCustomer(ctx, tid, c.ID); errors.Is(err, store.ErrNotFound) {
			return nil, httpx.NotFound("customer not found")
		} else if err != nil {
			return nil, httpx.Internal(err)
		}
		if d.Notify != nil {
			_ = d.Notify.QueueTenantTelegram(ctx, tid, notify.OpsMsg("hapus", "Pelanggan dihapus (auto-cabut)",
				c.CustomerCode+" — "+c.FullName,
			))
		}
		auditEvent(ctx, d, AuditCustomerDelete, "customer", &c.ID, map[string]any{
			"customer_code": c.CustomerCode, "full_name": c.FullName,
		})
		return &struct{ Body map[string]string }{Body: map[string]string{"status": "deleted"}}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "export-customers-csv", Method: http.MethodGet, Path: "/api/customers/export.csv",
		Summary: "Export customers as CSV", Tags: []string{"Customers"},
		Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ClusterID string `query:"cluster_id"`
		IsActive  string `query:"is_active"`
		Status    string `query:"status"`
	}) (*struct {
		ContentType        string `header:"Content-Type"`
		ContentDisposition string `header:"Content-Disposition"`
		Body               []byte
	}, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		clusterID, err := optionalQueryID(input.ClusterID)
		if err != nil {
			return nil, err
		}
		var isActive *bool
		switch strings.ToLower(strings.TrimSpace(input.IsActive)) {
		case "true", "1", "aktif", "active":
			v := true
			isActive = &v
		case "false", "0", "nonaktif", "inactive":
			v := false
			isActive = &v
		}
		list, _, err := d.Store.ListCustomers(ctx, store.CustomerFilter{
			TenantID: tid, ClusterID: clusterID, IsActive: isActive, ServiceStatus: strings.TrimSpace(input.Status), Limit: 100000, Offset: 0,
		})
		if err != nil {
			return nil, httpx.Internal(err)
		}
		var buf bytes.Buffer
		w := csv.NewWriter(&buf)
		_ = w.Write([]string{"customer_code", "full_name", "phone", "email", "address", "cluster_code",
			"latitude", "longitude", "identity_type", "identity_number", "is_active", "portal_enabled", "service_status"})
		for _, c := range list {
			email, addr, ccode := "", "", ""
			if c.Email != nil {
				email = *c.Email
			}
			if c.Address != nil {
				addr = *c.Address
			}
			if c.ClusterID != nil {
				ccode = c.ClusterCode
			}
			lat, lng := "", ""
			if c.Latitude != nil {
				lat = strconv.FormatFloat(*c.Latitude, 'f', -1, 64)
			}
			if c.Longitude != nil {
				lng = strconv.FormatFloat(*c.Longitude, 'f', -1, 64)
			}
			idType, idNum := "", ""
			if c.IdentityType != nil {
				idType = *c.IdentityType
			}
			if c.IdentityNumber != nil {
				idNum = *c.IdentityNumber
			}
			_ = w.Write([]string{c.CustomerCode, c.FullName, c.Phone, email, addr, ccode, lat, lng,
				idType, idNum, strconv.FormatBool(c.IsActive), strconv.FormatBool(c.PortalEnabled), c.ServiceStatus})
		}
		w.Flush()
		if err := w.Error(); err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct {
			ContentType        string `header:"Content-Type"`
			ContentDisposition string `header:"Content-Disposition"`
			Body               []byte
		}{
			ContentType:        "text/csv; charset=utf-8",
			ContentDisposition: `attachment; filename="customers.csv"`,
			Body:               buf.Bytes(),
		}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "import-customers-csv", Method: http.MethodPost, Path: "/api/customers/import",
		Summary: "Import customers from CSV (upsert by customer_code)", Tags: []string{"Customers"},
		Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Body struct {
			CSV string `json:"csv"`
		}
	}) (*struct {
		Body struct {
			Created int      `json:"created"`
			Updated int      `json:"updated"`
			Skipped int      `json:"skipped"`
			Errors  []string `json:"errors,omitempty"`
		}
	}, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		raw := strings.TrimSpace(input.Body.CSV)
		if raw == "" {
			return nil, httpx.BadRequest("csv is required")
		}
		clusters, err := d.Store.ListClusters(ctx, tid)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		idByCode := map[string]xid.ID{}
		for _, c := range clusters {
			idByCode[strings.ToUpper(c.Code)] = c.ID
		}

		r := csv.NewReader(strings.NewReader(raw))
		r.TrimLeadingSpace = true
		r.FieldsPerRecord = -1
		records, err := r.ReadAll()
		if err != nil {
			return nil, httpx.BadRequest("invalid csv: " + err.Error())
		}
		if len(records) == 0 {
			return nil, httpx.BadRequest("csv is empty")
		}

		header := map[string]int{}
		start := 0
		first := records[0]
		looksHeader := false
		for i, h := range first {
			key := strings.ToLower(strings.TrimSpace(h))
			header[key] = i
			if key == "full_name" || key == "phone" || key == "customer_code" {
				looksHeader = true
			}
		}
		if looksHeader {
			start = 1
		} else {
			header = map[string]int{"customer_code": 0, "full_name": 1, "phone": 2, "email": 3, "address": 4,
				"cluster_code": 5, "latitude": 6, "longitude": 7, "identity_type": 8, "identity_number": 9,
				"is_active": 10, "portal_enabled": 11}
		}
		col := func(row []string, key string) string {
			i, ok := header[key]
			if !ok || i < 0 || i >= len(row) {
				return ""
			}
			return strings.TrimSpace(row[i])
		}
		parseBool := func(v string, def bool) bool {
			switch strings.ToLower(v) {
			case "true", "1", "yes", "y", "aktif", "active":
				return true
			case "false", "0", "no", "n", "nonaktif", "inactive":
				return false
			}
			return def
		}

		created, updated, skipped := 0, 0, 0
		var errs []string
		for i := start; i < len(records); i++ {
			row := records[i]
			if len(row) == 0 || (len(row) == 1 && strings.TrimSpace(row[0]) == "") {
				continue
			}
			name := col(row, "full_name")
			phone := col(row, "phone")
			if name == "" || phone == "" {
				skipped++
				errs = append(errs, fmt.Sprintf("baris %d: full_name dan phone wajib", i+1))
				continue
			}
			var clusterID *xid.ID
			if cc := col(row, "cluster_code"); cc != "" {
				if id, ok := idByCode[strings.ToUpper(cc)]; ok {
					cid := id
					clusterID = &cid
				} else {
					errs = append(errs, fmt.Sprintf("baris %d: cluster_code %q tidak ditemukan", i+1, cc))
					skipped++
					continue
				}
			}
			var lat, lng *float64
			if v := col(row, "latitude"); v != "" {
				if f, perr := strconv.ParseFloat(v, 64); perr == nil {
					lat = &f
				} else {
					errs = append(errs, fmt.Sprintf("baris %d: latitude tidak valid", i+1))
					skipped++
					continue
				}
			}
			if v := col(row, "longitude"); v != "" {
				if f, perr := strconv.ParseFloat(v, 64); perr == nil {
					lng = &f
				} else {
					errs = append(errs, fmt.Sprintf("baris %d: longitude tidak valid", i+1))
					skipped++
					continue
				}
			}
			var email, addr *string
			if v := col(row, "email"); v != "" {
				email = &v
			}
			if v := col(row, "address"); v != "" {
				addr = &v
			}
			var identityType *string
			if v := col(row, "identity_type"); v != "" {
				identityType = normalizeIdentityType(&v)
			}
			var identityNumber *string
			if v := col(row, "identity_number"); v != "" {
				identityNumber = &v
			}

			code := col(row, "customer_code")
			existing, gerr := d.Store.GetCustomerByCode(ctx, tid, code)
			if gerr != nil && !errors.Is(gerr, store.ErrNotFound) {
				errs = append(errs, fmt.Sprintf("baris %d: gagal baca", i+1))
				skipped++
				continue
			}
			if gerr == nil {
				phoneChanged := existing.Phone != phone
				existing.FullName = name
				existing.Phone = phone
				existing.Email = email
				existing.Address = addr
				existing.Latitude = lat
				existing.Longitude = lng
				existing.IdentityType = identityType
				existing.IdentityNumber = identityNumber
				if clusterID != nil {
					existing.ClusterID = clusterID
				}
				existing.IsActive = parseBool(col(row, "is_active"), existing.IsActive)
				existing.PortalEnabled = parseBool(col(row, "portal_enabled"), existing.PortalEnabled)
				if phoneChanged {
					if hash, herr := auth.HashPassword(phone); herr == nil {
						existing.PasswordHash = hash
					}
				}
				if err := d.Store.UpdateCustomer(ctx, existing); err != nil {
					errs = append(errs, fmt.Sprintf("baris %d: gagal update", i+1))
					skipped++
					continue
				}
				updated++
				continue
			}
			if code == "" {
				if clusterID != nil {
					code, err = d.Store.NextCustomerCodeForCluster(ctx, tid, *clusterID)
				} else {
					code, err = d.Store.NextCustomerCode(ctx, tid)
				}
				if err != nil {
					errs = append(errs, fmt.Sprintf("baris %d: gagal buat kode", i+1))
					skipped++
					continue
				}
			}
			hash, herr := auth.HashPassword(phone)
			if herr != nil {
				errs = append(errs, fmt.Sprintf("baris %d: gagal proses password", i+1))
				skipped++
				continue
			}
			nc := &store.Customer{
				TenantID: tid, ClusterID: clusterID, CustomerCode: code,
				FullName: name, Email: email, Phone: phone, Address: addr,
				Latitude: lat, Longitude: lng,
				IdentityType: identityType, IdentityNumber: identityNumber,
				IsActive:      parseBool(col(row, "is_active"), true),
				PortalEnabled: parseBool(col(row, "portal_enabled"), true),
				PasswordHash:  hash,
			}
			if err := d.Store.CreateCustomer(ctx, nc); err != nil {
				errs = append(errs, fmt.Sprintf("baris %d: gagal buat", i+1))
				skipped++
				continue
			}
			created++
		}

		out := &struct {
			Body struct {
				Created int      `json:"created"`
				Updated int      `json:"updated"`
				Skipped int      `json:"skipped"`
				Errors  []string `json:"errors,omitempty"`
			}
		}{}
		out.Body.Created = created
		out.Body.Updated = updated
		out.Body.Skipped = skipped
		if len(errs) > 20 {
			errs = append(errs[:20], fmt.Sprintf("… +%d error lain", len(errs)-20))
		}
		out.Body.Errors = errs
		return out, nil
	})

	registerCustomerDocuments(api, d)
}

func registerCustomerDocuments(api huma.API, d *Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "list-customer-documents", Method: http.MethodGet, Path: "/api/customers/{id}/documents",
		Tags: []string{"Customers"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID xid.ID `path:"id"`
	}) (*struct{ Body []store.CustomerDocument }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		list, err := d.Store.ListCustomerDocuments(ctx, tid, input.ID)
		if errors.Is(err, store.ErrNotFound) {
			return nil, httpx.NotFound("customer not found")
		}
		if err != nil {
			return nil, httpx.Internal(err)
		}
		if list == nil {
			list = []store.CustomerDocument{}
		}
		return &struct{ Body []store.CustomerDocument }{Body: list}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "add-customer-document", Method: http.MethodPost, Path: "/api/customers/{id}/documents",
		Tags: []string{"Customers"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID   xid.ID `path:"id"`
		Body struct {
			Kind    string `json:"kind"`
			URL     string `json:"url"`
			Caption string `json:"caption,omitempty"`
		}
	}) (*struct{ Body store.CustomerDocument }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		var uploader *xid.ID
		if uid := userIDFromCtx(ctx); !xid.IsNil(uid) {
			uploader = &uid
		}
		doc, err := d.Store.AddCustomerDocument(ctx, tid, input.ID, uploader, input.Body.Kind, input.Body.URL, input.Body.Caption, "upload", nil)
		if errors.Is(err, store.ErrNotFound) {
			return nil, httpx.NotFound("customer not found")
		}
		if err != nil {
			return nil, httpx.BadRequest(err.Error())
		}
		return &struct{ Body store.CustomerDocument }{Body: *doc}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "delete-customer-document", Method: http.MethodDelete, Path: "/api/customers/{id}/documents/{doc_id}",
		Tags: []string{"Customers"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID    xid.ID `path:"id"`
		DocID xid.ID `path:"doc_id"`
	}) (*struct{ Body map[string]string }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		if err := d.Store.DeleteCustomerDocument(ctx, tid, input.ID, input.DocID); errors.Is(err, store.ErrNotFound) {
			return nil, httpx.NotFound("dokumen tidak ditemukan")
		} else if err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body map[string]string }{Body: map[string]string{"status": "deleted"}}, nil
	})
}

func registerClusters(api huma.API, d *Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "list-clusters", Method: http.MethodGet, Path: "/api/clusters",
		Summary: "List POP / clusters", Tags: []string{"Clusters"},
		Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, _ *struct{}) (*struct{ Body []store.Cluster }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		list, err := d.Store.ListClusters(ctx, tid)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body []store.Cluster }{Body: list}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "create-cluster", Method: http.MethodPost, Path: "/api/clusters",
		Summary: "Create cluster", Tags: []string{"Clusters"},
		Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Body struct {
			Name                string   `json:"name"`
			Code                string   `json:"code"`
			CustomerCodePrefix  string   `json:"customer_code_prefix,omitempty"`
			CustomerCodePattern string   `json:"customer_code_pattern,omitempty"`
			SeqWidth            int      `json:"seq_width,omitempty"`
			Address             *string  `json:"address,omitempty"`
			Latitude            *float64 `json:"latitude,omitempty"`
			Longitude           *float64 `json:"longitude,omitempty"`
			CoverageRadiusKm    *float64 `json:"coverage_radius_km,omitempty"`
			Notes               *string  `json:"notes,omitempty"`
		}
	}) (*struct{ Body store.Cluster }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		name := strings.TrimSpace(input.Body.Name)
		code := strings.TrimSpace(input.Body.Code)
		if name == "" || code == "" {
			return nil, httpx.BadRequest("name and code are required")
		}
		c := &store.Cluster{
			TenantID: tid, Name: name, Code: code,
			CustomerCodePrefix:  input.Body.CustomerCodePrefix,
			CustomerCodePattern: input.Body.CustomerCodePattern,
			SeqWidth:            input.Body.SeqWidth,
			Address:             input.Body.Address,
			Latitude:            input.Body.Latitude,
			Longitude:           input.Body.Longitude,
			CoverageRadiusKm:    input.Body.CoverageRadiusKm,
			Notes:               input.Body.Notes,
			IsActive:            true,
		}
		if err := d.Store.CreateCluster(ctx, c); err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body store.Cluster }{Body: *c}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-cluster", Method: http.MethodGet, Path: "/api/clusters/{id}",
		Tags: []string{"Clusters"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID xid.ID `path:"id"`
	}) (*struct{ Body store.Cluster }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		c, err := d.Store.GetCluster(ctx, tid, input.ID)
		if errors.Is(err, store.ErrNotFound) {
			return nil, httpx.NotFound("cluster not found")
		}
		if err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body store.Cluster }{Body: *c}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "update-cluster", Method: http.MethodPut, Path: "/api/clusters/{id}",
		Tags: []string{"Clusters"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID   xid.ID `path:"id"`
		Body struct {
			Name                string   `json:"name"`
			Code                string   `json:"code"`
			CustomerCodePrefix  string   `json:"customer_code_prefix"`
			CustomerCodePattern string   `json:"customer_code_pattern"`
			SeqWidth            int      `json:"seq_width"`
			Address             *string  `json:"address,omitempty"`
			Latitude            *float64 `json:"latitude,omitempty"`
			Longitude           *float64 `json:"longitude,omitempty"`
			CoverageRadiusKm    *float64 `json:"coverage_radius_km,omitempty"`
			Notes               *string  `json:"notes,omitempty"`
			IsActive            bool     `json:"is_active"`
		}
	}) (*struct{ Body store.Cluster }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		c, err := d.Store.GetCluster(ctx, tid, input.ID)
		if errors.Is(err, store.ErrNotFound) {
			return nil, httpx.NotFound("cluster not found")
		}
		if err != nil {
			return nil, httpx.Internal(err)
		}
		name := strings.TrimSpace(input.Body.Name)
		code := strings.TrimSpace(input.Body.Code)
		if name == "" || code == "" {
			return nil, httpx.BadRequest("name and code are required")
		}
		c.Name = name
		c.Code = code
		c.CustomerCodePrefix = input.Body.CustomerCodePrefix
		c.CustomerCodePattern = input.Body.CustomerCodePattern
		c.SeqWidth = input.Body.SeqWidth
		c.Address = input.Body.Address
		c.Latitude = input.Body.Latitude
		c.Longitude = input.Body.Longitude
		c.CoverageRadiusKm = input.Body.CoverageRadiusKm
		c.Notes = input.Body.Notes
		c.IsActive = input.Body.IsActive
		if err := d.Store.UpdateCluster(ctx, c); err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body store.Cluster }{Body: *c}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "put-cluster-coverage", Method: http.MethodPut, Path: "/api/clusters/{id}/coverage",
		Summary: "Set POP coverage radius (km)", Tags: []string{"Clusters"},
		Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID   xid.ID `path:"id"`
		Body struct {
			CoverageRadiusKm *float64 `json:"coverage_radius_km"`
		}
	}) (*struct{ Body store.Cluster }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		if err := d.Store.UpdateClusterCoverage(ctx, tid, input.ID, input.Body.CoverageRadiusKm); errors.Is(err, store.ErrNotFound) {
			return nil, httpx.NotFound("cluster not found")
		} else if err != nil {
			return nil, httpx.Internal(err)
		}
		c, err := d.Store.GetCluster(ctx, tid, input.ID)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body store.Cluster }{Body: *c}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "delete-cluster", Method: http.MethodDelete, Path: "/api/clusters/{id}",
		Tags: []string{"Clusters"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID xid.ID `path:"id"`
	}) (*struct{ Body map[string]string }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		if err := d.Store.DeleteCluster(ctx, tid, input.ID); errors.Is(err, store.ErrNotFound) {
			return nil, httpx.NotFound("cluster not found")
		} else if err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body map[string]string }{Body: map[string]string{"status": "deleted"}}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "preview-cluster-customer-code", Method: http.MethodGet, Path: "/api/clusters/{id}/next-customer-code",
		Summary: "Preview next customer code for cluster", Tags: []string{"Clusters"},
		Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID xid.ID `path:"id"`
	}) (*struct {
		Body struct {
			Preview string `json:"preview"`
		}
	}, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		preview, err := d.Store.PreviewCustomerCode(ctx, tid, input.ID)
		if errors.Is(err, store.ErrNotFound) {
			return nil, httpx.NotFound("cluster not found")
		}
		if err != nil {
			return nil, httpx.Internal(err)
		}
		out := &struct {
			Body struct {
				Preview string `json:"preview"`
			}
		}{}
		out.Body.Preview = preview
		return out, nil
	})
}

func registerPlans(api huma.API, d *Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "list-plans", Method: http.MethodGet, Path: "/api/plans",
		Tags: []string{"Plans"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, _ *struct{}) (*struct{ Body []store.Plan }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		list, err := d.Store.ListPlans(ctx, tid)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body []store.Plan }{Body: list}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "create-plan", Method: http.MethodPost, Path: "/api/plans",
		Tags: []string{"Plans"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Body struct {
			Name          string   `json:"name"`
			Code          string   `json:"code"`
			ServiceType   string   `json:"service_type"`
			Price         int64    `json:"price"`
			BillingCycle  string   `json:"billing_cycle,omitempty"`
			DownloadMbps  int      `json:"download_mbps,omitempty"`
			UploadMbps    int      `json:"upload_mbps,omitempty"`
			QuotaGB       *int     `json:"quota_gb,omitempty"`
			LimitUptime   *string  `json:"limit_uptime,omitempty"`
			SharedUsers   *int     `json:"shared_users,omitempty"`
			ProfileName   *string  `json:"profile_name,omitempty"`
			IsolirProfile *string  `json:"isolir_profile,omitempty"`
			DueDay        *int     `json:"due_day,omitempty"`
			TaxPercent    *float64 `json:"tax_percent,omitempty"`
			IsActive      *bool    `json:"is_active,omitempty"`
			PortalVisible *bool    `json:"portal_visible,omitempty"`
		}
	}) (*struct{ Body store.Plan }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		name := strings.TrimSpace(input.Body.Name)
		code := strings.TrimSpace(input.Body.Code)
		if name == "" || code == "" {
			return nil, httpx.BadRequest("name and code are required")
		}
		cycle := strings.TrimSpace(input.Body.BillingCycle)
		if cycle == "" {
			cycle = "monthly"
		}
		svc := strings.TrimSpace(input.Body.ServiceType)
		if svc == "" {
			svc = "pppoe"
		}
		p := store.Plan{
			TenantID: tid, Name: name, Code: code, ServiceType: svc, Price: input.Body.Price,
			BillingCycle: cycle, DownloadMbps: input.Body.DownloadMbps, UploadMbps: input.Body.UploadMbps,
			QuotaGB: input.Body.QuotaGB, LimitUptime: input.Body.LimitUptime, SharedUsers: input.Body.SharedUsers,
			ProfileName: input.Body.ProfileName, IsolirProfile: input.Body.IsolirProfile,
			IsActive: true,
		}
		if p.LimitUptime != nil {
			u := strings.TrimSpace(*p.LimitUptime)
			if u == "" {
				p.LimitUptime = nil
			} else {
				p.LimitUptime = &u
			}
		}
		if p.SharedUsers != nil && *p.SharedUsers <= 0 {
			p.SharedUsers = nil
		}
		if p.QuotaGB != nil && *p.QuotaGB <= 0 {
			p.QuotaGB = nil
		}
		if input.Body.DueDay != nil {
			dd := store.ClampDueDay(*input.Body.DueDay, 1)
			p.DueDay = &dd
		}
		if input.Body.TaxPercent != nil {
			p.TaxPercent = *input.Body.TaxPercent
		}
		if input.Body.IsActive != nil {
			p.IsActive = *input.Body.IsActive
		}
		if input.Body.PortalVisible != nil {
			p.PortalVisible = *input.Body.PortalVisible
		}
		if p.ProfileName == nil || strings.TrimSpace(*p.ProfileName) == "" {
			pn := code
			p.ProfileName = &pn
		}
		if err := d.Store.CreatePlan(ctx, &p); err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body store.Plan }{Body: p}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "update-plan", Method: http.MethodPut, Path: "/api/plans/{id}",
		Tags: []string{"Plans"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID   xid.ID `path:"id"`
		Body struct {
			Name          string   `json:"name"`
			ServiceType   string   `json:"service_type,omitempty"`
			Price         int64    `json:"price"`
			BillingCycle  string   `json:"billing_cycle,omitempty"`
			DownloadMbps  int      `json:"download_mbps,omitempty"`
			UploadMbps    int      `json:"upload_mbps,omitempty"`
			QuotaGB       *int     `json:"quota_gb,omitempty"`
			LimitUptime   *string  `json:"limit_uptime,omitempty"`
			SharedUsers   *int     `json:"shared_users,omitempty"`
			ProfileName   *string  `json:"profile_name,omitempty"`
			IsolirProfile *string  `json:"isolir_profile,omitempty"`
			DueDay        *int     `json:"due_day,omitempty"`
			TaxPercent    *float64 `json:"tax_percent,omitempty"`
			IsActive      *bool    `json:"is_active,omitempty"`
			PortalVisible *bool    `json:"portal_visible,omitempty"`
		}
	}) (*struct{ Body store.Plan }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		existing, err := d.Store.GetPlan(ctx, tid, input.ID)
		if errors.Is(err, store.ErrNotFound) {
			return nil, httpx.NotFound("plan not found")
		}
		if err != nil {
			return nil, httpx.Internal(err)
		}
		name := strings.TrimSpace(input.Body.Name)
		if name == "" {
			return nil, httpx.BadRequest("name is required")
		}
		existing.Name = name
		if svc := strings.TrimSpace(input.Body.ServiceType); svc != "" {
			existing.ServiceType = svc
		}
		existing.Price = input.Body.Price
		if cycle := strings.TrimSpace(input.Body.BillingCycle); cycle != "" {
			existing.BillingCycle = cycle
		}
		existing.DownloadMbps = input.Body.DownloadMbps
		existing.UploadMbps = input.Body.UploadMbps
		existing.QuotaGB = input.Body.QuotaGB
		if existing.QuotaGB != nil && *existing.QuotaGB <= 0 {
			existing.QuotaGB = nil
		}
		existing.LimitUptime = input.Body.LimitUptime
		if existing.LimitUptime != nil {
			u := strings.TrimSpace(*existing.LimitUptime)
			if u == "" {
				existing.LimitUptime = nil
			} else {
				existing.LimitUptime = &u
			}
		}
		existing.SharedUsers = input.Body.SharedUsers
		if existing.SharedUsers != nil && *existing.SharedUsers <= 0 {
			existing.SharedUsers = nil
		}
		if input.Body.ProfileName != nil {
			pn := strings.TrimSpace(*input.Body.ProfileName)
			if pn == "" {
				pn = existing.Code
			}
			existing.ProfileName = &pn
		}
		if input.Body.IsolirProfile != nil {
			existing.IsolirProfile = input.Body.IsolirProfile
		}
		if input.Body.DueDay != nil {
			dd := store.ClampDueDay(*input.Body.DueDay, 1)
			existing.DueDay = &dd
		} else {
			existing.DueDay = nil
		}
		if input.Body.TaxPercent != nil {
			existing.TaxPercent = *input.Body.TaxPercent
		}
		if input.Body.IsActive != nil {
			existing.IsActive = *input.Body.IsActive
		}
		if input.Body.PortalVisible != nil {
			existing.PortalVisible = *input.Body.PortalVisible
		}
		if err := d.Store.UpdatePlan(ctx, existing); errors.Is(err, store.ErrNotFound) {
			return nil, httpx.NotFound("plan not found")
		} else if err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body store.Plan }{Body: *existing}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "delete-plan", Method: http.MethodDelete, Path: "/api/plans/{id}",
		Tags: []string{"Plans"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID xid.ID `path:"id"`
	}) (*struct{ Body map[string]string }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		if err := d.Store.DeletePlan(ctx, tid, input.ID); errors.Is(err, store.ErrNotFound) {
			return nil, httpx.NotFound("plan not found")
		} else if err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body map[string]string }{Body: map[string]string{"status": "deleted"}}, nil
	})
}

func registerPlanOffers(api huma.API, d *Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "list-plan-offers", Method: http.MethodGet, Path: "/api/plan-offers",
		Summary: "List plan offers per cluster", Tags: []string{"Plans"},
		Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ClusterID string `query:"cluster_id"`
		PlanID    string `query:"plan_id"`
	}) (*struct{ Body []store.PlanClusterOffer }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		clusterID, err := optionalQueryID(input.ClusterID)
		if err != nil {
			return nil, err
		}
		planID, err := optionalQueryID(input.PlanID)
		if err != nil {
			return nil, err
		}
		list, err := d.Store.ListPlanOffers(ctx, tid, clusterID, planID)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body []store.PlanClusterOffer }{Body: list}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "upsert-plan-offer", Method: http.MethodPost, Path: "/api/plan-offers",
		Summary: "Create or update plan offer for a cluster", Tags: []string{"Plans"},
		Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Body struct {
			PlanID    xid.ID  `json:"plan_id"`
			ClusterID xid.ID  `json:"cluster_id"`
			Price     int64   `json:"price"`
			IPPoolID  *xid.ID `json:"ip_pool_id,omitempty"`
			IsActive  *bool   `json:"is_active,omitempty"`
			DueDay    *int    `json:"due_day,omitempty"`
			Sync      bool    `json:"sync_profiles,omitempty"`
		}
	}) (*struct{ Body store.PlanClusterOffer }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		if input.Body.Price < 0 {
			return nil, httpx.BadRequest("price must be >= 0")
		}
		if _, err := d.Store.GetPlan(ctx, tid, input.Body.PlanID); errors.Is(err, store.ErrNotFound) {
			return nil, httpx.BadRequest("plan not found")
		} else if err != nil {
			return nil, httpx.Internal(err)
		}
		if _, err := d.Store.GetCluster(ctx, tid, input.Body.ClusterID); errors.Is(err, store.ErrNotFound) {
			return nil, httpx.BadRequest("cluster not found")
		} else if err != nil {
			return nil, httpx.Internal(err)
		}
		if input.Body.IPPoolID != nil {
			pool, err := d.Store.GetIPPool(ctx, tid, *input.Body.IPPoolID)
			if errors.Is(err, store.ErrNotFound) {
				return nil, httpx.BadRequest("ip pool tidak ditemukan")
			}
			if err != nil {
				return nil, httpx.Internal(err)
			}
			if pool.RouterID == nil {
				return nil, httpx.BadRequest("ip pool belum terhubung ke router")
			}
			routers, err := d.Store.ListRoutersByCluster(ctx, tid, input.Body.ClusterID)
			if err != nil {
				return nil, httpx.Internal(err)
			}
			ok := false
			for _, r := range routers {
				if r.ID == *pool.RouterID {
					ok = true
					break
				}
			}
			if !ok {
				return nil, httpx.BadRequest("ip pool harus dari router di cluster yang sama")
			}
		}
		active := true
		if input.Body.IsActive != nil {
			active = *input.Body.IsActive
		}
		o := &store.PlanClusterOffer{
			TenantID: tid, PlanID: input.Body.PlanID, ClusterID: input.Body.ClusterID,
			Price: input.Body.Price, IsActive: active, IPPoolID: input.Body.IPPoolID,
			DueDay: input.Body.DueDay,
		}
		if err := d.Store.UpsertPlanOffer(ctx, o); err != nil {
			return nil, httpx.Internal(err)
		}
		full, err := d.Store.GetPlanOffer(ctx, tid, o.ID)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		if input.Body.Sync {
			_, _, _ = syncPlanProfileToCluster(ctx, d, tid, full)
		}
		return &struct{ Body store.PlanClusterOffer }{Body: *full}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "update-plan-offer", Method: http.MethodPut, Path: "/api/plan-offers/{id}",
		Tags: []string{"Plans"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID   xid.ID `path:"id"`
		Body struct {
			Price    int64   `json:"price"`
			IsActive bool    `json:"is_active"`
			IPPoolID *xid.ID `json:"ip_pool_id,omitempty"`
			DueDay   *int    `json:"due_day,omitempty"`
			Sync     bool    `json:"sync_profiles,omitempty"`
		}
	}) (*struct{ Body store.PlanClusterOffer }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		o, err := d.Store.GetPlanOffer(ctx, tid, input.ID)
		if errors.Is(err, store.ErrNotFound) {
			return nil, httpx.NotFound("offer not found")
		}
		if err != nil {
			return nil, httpx.Internal(err)
		}
		if input.Body.Price < 0 {
			return nil, httpx.BadRequest("price must be >= 0")
		}
		if input.Body.IPPoolID != nil {
			pool, err := d.Store.GetIPPool(ctx, tid, *input.Body.IPPoolID)
			if errors.Is(err, store.ErrNotFound) {
				return nil, httpx.BadRequest("ip pool tidak ditemukan")
			}
			if err != nil {
				return nil, httpx.Internal(err)
			}
			if pool.RouterID == nil {
				return nil, httpx.BadRequest("ip pool belum terhubung ke router")
			}
			routers, err := d.Store.ListRoutersByCluster(ctx, tid, o.ClusterID)
			if err != nil {
				return nil, httpx.Internal(err)
			}
			ok := false
			for _, r := range routers {
				if r.ID == *pool.RouterID {
					ok = true
					break
				}
			}
			if !ok {
				return nil, httpx.BadRequest("ip pool harus dari router di cluster yang sama")
			}
		}
		o.Price = input.Body.Price
		o.IsActive = input.Body.IsActive
		o.IPPoolID = input.Body.IPPoolID
		o.DueDay = input.Body.DueDay
		if err := d.Store.UpdatePlanOffer(ctx, o); err != nil {
			return nil, httpx.Internal(err)
		}
		full, err := d.Store.GetPlanOffer(ctx, tid, o.ID)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		if input.Body.Sync {
			_, _, _ = syncPlanProfileToCluster(ctx, d, tid, full)
		}
		return &struct{ Body store.PlanClusterOffer }{Body: *full}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "delete-plan-offer", Method: http.MethodDelete, Path: "/api/plan-offers/{id}",
		Tags: []string{"Plans"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID xid.ID `path:"id"`
	}) (*struct{ Body map[string]string }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		if err := d.Store.DeletePlanOffer(ctx, tid, input.ID); errors.Is(err, store.ErrNotFound) {
			return nil, httpx.NotFound("offer not found")
		} else if err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body map[string]string }{Body: map[string]string{"status": "deleted"}}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "sync-plan-offer-profiles", Method: http.MethodPost, Path: "/api/plan-offers/{id}/sync-profiles",
		Summary: "Push PPP/hotspot profile to all routers in the offer cluster",
		Tags:    []string{"Plans"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID xid.ID `path:"id"`
	}) (*struct {
		Body struct {
			Synced  int                 `json:"synced"`
			Failed  int                 `json:"failed"`
			Results []map[string]string `json:"results"`
		}
	}, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		o, err := d.Store.GetPlanOffer(ctx, tid, input.ID)
		if errors.Is(err, store.ErrNotFound) {
			return nil, httpx.NotFound("offer not found")
		}
		if err != nil {
			return nil, httpx.Internal(err)
		}
		results, synced, failed := syncPlanProfileToCluster(ctx, d, tid, o)
		out := &struct {
			Body struct {
				Synced  int                 `json:"synced"`
				Failed  int                 `json:"failed"`
				Results []map[string]string `json:"results"`
			}
		}{}
		out.Body.Synced = synced
		out.Body.Failed = failed
		out.Body.Results = results
		return out, nil
	})
}

func syncPlanProfileToCluster(ctx context.Context, d *Deps, tid xid.ID, o *store.PlanClusterOffer) (results []map[string]string, synced, failed int) {
	profile := ""
	if o.ProfileName != nil {
		profile = strings.TrimSpace(*o.ProfileName)
	}
	if profile == "" {
		profile = o.PlanCode
	}
	if profile == "" {
		return []map[string]string{{"error": "plan has no profile_name or code"}}, 0, 0
	}
	routers, err := d.Store.ListRoutersByCluster(ctx, tid, o.ClusterID)
	if err != nil {
		return []map[string]string{{"error": err.Error()}}, 0, 1
	}
	if len(routers) == 0 {
		return []map[string]string{{"info": "no routers in cluster"}}, 0, 0
	}
	for _, r := range routers {
		row := map[string]string{"router_id": r.ID.String(), "router": r.Name}
		if !r.IsActive {
			row["status"] = "skipped"
			row["message"] = "inactive"
			results = append(results, row)
			continue
		}
		prov, err := d.Provisioner.Get(r.Provisioner)
		if err != nil {
			row["status"] = "failed"
			row["message"] = err.Error()
			failed++
			results = append(results, row)
			continue
		}
		ensurer, ok := prov.(provision.ProfileEnsurer)
		if !ok {
			row["status"] = "skipped"
			row["message"] = "provisioner does not support profile sync"
			results = append(results, row)
			continue
		}
		poolName := ensureProfileIPPool(ctx, d, tid, r.ID, o.IPPoolID)
		if err := ensurer.EnsureBandwidthProfile(ctx, tid, r.ID, profile, o.DownloadMbps, o.UploadMbps, o.ServiceType, poolName, o.Price); err != nil {
			row["status"] = "failed"
			row["message"] = err.Error()
			failed++
		} else {
			row["status"] = "ok"
			row["profile"] = profile
			synced++
		}
		results = append(results, row)
	}
	return results, synced, failed
}

// parseFlexibleDateTime parses RFC 3339 ("2006-01-02T15:04:05Z07:00"),
// local datetime ("2006-01-02T15:04:05", assumed server-local, e.g. what the
// web date inputs send), or a plain date ("2006-01-02", assumed local noon).
// Returns ok=false when input is nil/empty (caller applies its default).
func parseFlexibleDateTime(raw *string, loc *time.Location) (t time.Time, ok bool, err error) {
	if raw == nil {
		return time.Time{}, false, nil
	}
	s := strings.TrimSpace(*raw)
	if s == "" {
		return time.Time{}, false, nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, true, nil
	}
	if t, err := time.ParseInLocation("2006-01-02T15:04:05", s, loc); err == nil {
		return t, true, nil
	}
	if t, err := time.ParseInLocation("2006-01-02", s, loc); err == nil {
		return time.Date(t.Year(), t.Month(), t.Day(), 12, 0, 0, 0, loc), true, nil
	}
	return time.Time{}, false, httpx.BadRequest("format tanggal tidak valid (pakai YYYY-MM-DD atau RFC 3339)")
}

func registerSubscriptions(api huma.API, d *Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "list-subscriptions", Method: http.MethodGet, Path: "/api/subscriptions",
		Tags: []string{"Subscriptions"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Status     string `query:"status"`
		CustomerID string `query:"customer_id"`
		Limit      int    `query:"limit"`
		Offset     int    `query:"offset"`
	}) (*struct {
		Body struct {
			Data  []store.Subscription `json:"data"`
			Total int64                `json:"total"`
		}
	}, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		customerID, err := optionalQueryID(input.CustomerID)
		if err != nil {
			return nil, err
		}
		list, total, err := d.Store.ListSubscriptions(ctx, tid, input.Status, customerID, input.Limit, input.Offset)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		out := &struct {
			Body struct {
				Data  []store.Subscription `json:"data"`
				Total int64                `json:"total"`
			}
		}{}
		out.Body.Data = list
		out.Body.Total = total
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "create-subscription", Method: http.MethodPost, Path: "/api/subscriptions",
		Tags: []string{"Subscriptions"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Body struct {
			CustomerID  xid.ID  `json:"customer_id"`
			PlanID      xid.ID  `json:"plan_id"`
			RouterID    *xid.ID `json:"router_id,omitempty"`
			Username    string  `json:"username"`
			Password    string  `json:"password"`
			ServiceType string  `json:"service_type,omitempty"`
			ODPID       *xid.ID `json:"odp_id,omitempty"`
			PortNumber  *int    `json:"port_number,omitempty"`
		}
	}) (*struct{ Body store.Subscription }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		cust, err := d.Store.GetCustomer(ctx, tid, input.Body.CustomerID)
		if errors.Is(err, store.ErrNotFound) {
			return nil, httpx.BadRequest("customer not found")
		}
		if err != nil {
			return nil, httpx.Internal(err)
		}
		if cust.IsDismantled() {
			return nil, httpx.BadRequest("pelanggan sudah cabut — tidak bisa menambah secret")
		}
		plan, err := d.Store.GetPlan(ctx, tid, input.Body.PlanID)
		if errors.Is(err, store.ErrNotFound) {
			return nil, httpx.BadRequest("plan not found")
		}
		if err != nil {
			return nil, httpx.Internal(err)
		}
		if cust.ClusterID != nil {
			offer, err := d.Store.GetPlanOfferByPlanCluster(ctx, tid, plan.ID, *cust.ClusterID)
			if errors.Is(err, store.ErrNotFound) || (err == nil && !offer.IsActive) {
				return nil, httpx.BadRequest("paket belum ditawarkan di cluster pelanggan")
			}
			if err != nil {
				return nil, httpx.Internal(err)
			}
		}
		if input.Body.RouterID != nil {
			r, err := d.Store.GetRouter(ctx, tid, *input.Body.RouterID)
			if errors.Is(err, store.ErrNotFound) {
				return nil, httpx.BadRequest("router not found")
			}
			if err != nil {
				return nil, httpx.Internal(err)
			}
			if cust.ClusterID != nil && r.SiteID != nil && *r.SiteID != *cust.ClusterID {
				return nil, httpx.BadRequest("router harus dari cluster yang sama dengan pelanggan")
			}
			if cust.ClusterID != nil && r.SiteID == nil {
				return nil, httpx.BadRequest("router belum terhubung ke cluster")
			}
		}
		var odp *store.ODP
		if input.Body.ODPID != nil {
			odp, err = d.Store.GetODP(ctx, tid, *input.Body.ODPID)
			if errors.Is(err, store.ErrNotFound) {
				return nil, httpx.BadRequest("odp not found")
			}
			if err != nil {
				return nil, httpx.Internal(err)
			}
			if cust.ClusterID != nil && odp.ClusterID != nil && *cust.ClusterID != *odp.ClusterID {
				return nil, httpx.BadRequest("odp harus dari cluster yang sama dengan pelanggan")
			}
			if odp.FreePorts <= 0 {
				return nil, httpx.BadRequest("odp tidak punya port kosong")
			}
			if input.Body.PortNumber != nil && *input.Body.PortNumber > 0 {
				// validated on assign
			}
		} else if input.Body.PortNumber != nil {
			return nil, httpx.BadRequest("port_number membutuhkan odp_id")
		}
		st := "pppoe"
		if input.Body.ServiceType != "" {
			st = input.Body.ServiceType
		} else if plan.ServiceType != "" {
			st = plan.ServiceType
		}
		username := strings.TrimSpace(input.Body.Username)
		if username == "" {
			return nil, httpx.BadRequest("username is required")
		}
		password := strings.TrimSpace(input.Body.Password)
		if password == "" {
			return nil, httpx.BadRequest("password PPPoE wajib diisi")
		}
		pw := password
		sub := &store.Subscription{
			TenantID: tid, CustomerID: input.Body.CustomerID, PlanID: input.Body.PlanID,
			RouterID: input.Body.RouterID, Username: username, Password: &pw, ServiceType: st, Status: "pending",
		}
		if err := d.Store.CreateSubscription(ctx, sub); err != nil {
			return nil, httpx.Internal(err)
		}
		sub.CustomerName = cust.FullName
		sub.CustomerCode = cust.CustomerCode
		sub.PlanName = plan.Name
		// Jangan kirim password balik di list/create response body.
		sub.Password = nil
		if odp != nil {
			custID := sub.CustomerID
			subID := sub.ID
			rollbackSub := func() {
				_, _ = d.Store.Pool.Exec(ctx, `DELETE FROM subscriptions WHERE tenant_id=$1 AND id=$2`, tid, sub.ID)
			}
			if input.Body.PortNumber != nil && *input.Body.PortNumber > 0 {
				if err := d.Store.AssignODPPort(ctx, tid, odp.ID, *input.Body.PortNumber, &custID, &subID); errors.Is(err, store.ErrNotFound) {
					rollbackSub()
					return nil, httpx.BadRequest("port odp tidak tersedia")
				} else if err != nil {
					rollbackSub()
					return nil, httpx.Internal(err)
				}
				n := *input.Body.PortNumber
				sub.PortNumber = &n
			} else {
				port, err := d.Store.AssignNextAvailableODPPort(ctx, tid, odp.ID, custID, subID)
				if errors.Is(err, store.ErrNotFound) {
					rollbackSub()
					return nil, httpx.BadRequest("odp tidak punya port kosong")
				}
				if err != nil {
					rollbackSub()
					return nil, httpx.Internal(err)
				}
				n := port.PortNumber
				sub.PortNumber = &n
			}
			sub.ODPID = &odp.ID
			sub.ODPCode = odp.Code
			sub.ODPName = odp.Name
		}
		return &struct{ Body store.Subscription }{Body: *sub}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-subscription", Method: http.MethodGet, Path: "/api/subscriptions/{id}",
		Tags: []string{"Subscriptions"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID xid.ID `path:"id"`
	}) (*struct{ Body store.Subscription }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		sub, err := d.Store.GetSubscription(ctx, tid, input.ID)
		if errors.Is(err, store.ErrNotFound) {
			return nil, httpx.NotFound("subscription not found")
		}
		if err != nil {
			return nil, httpx.Internal(err)
		}
		sub.Password = nil
		return &struct{ Body store.Subscription }{Body: *sub}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "update-subscription", Method: http.MethodPut, Path: "/api/subscriptions/{id}",
		Tags: []string{"Subscriptions"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID   xid.ID `path:"id"`
		Body struct {
			PlanID      xid.ID  `json:"plan_id"`
			RouterID    *xid.ID `json:"router_id,omitempty"`
			Username    string  `json:"username"`
			Password    string  `json:"password,omitempty"`
			ServiceType string  `json:"service_type,omitempty"`
			ODPID       *xid.ID `json:"odp_id,omitempty"`
			PortNumber  *int    `json:"port_number,omitempty"`
			ClearODP    bool    `json:"clear_odp,omitempty"`
		}
	}) (*struct{ Body store.Subscription }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		cur, err := d.Store.GetSubscription(ctx, tid, input.ID)
		if errors.Is(err, store.ErrNotFound) {
			return nil, httpx.NotFound("subscription not found")
		}
		if err != nil {
			return nil, httpx.Internal(err)
		}
		oldUsername := cur.Username
		oldRouterID := cur.RouterID
		oldStatus := cur.Status

		cust, err := d.Store.GetCustomer(ctx, tid, cur.CustomerID)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		plan, err := d.Store.GetPlan(ctx, tid, input.Body.PlanID)
		if errors.Is(err, store.ErrNotFound) {
			return nil, httpx.BadRequest("plan not found")
		}
		if err != nil {
			return nil, httpx.Internal(err)
		}
		if cust.ClusterID != nil {
			offer, err := d.Store.GetPlanOfferByPlanCluster(ctx, tid, plan.ID, *cust.ClusterID)
			if errors.Is(err, store.ErrNotFound) || (err == nil && !offer.IsActive) {
				return nil, httpx.BadRequest("paket belum ditawarkan di cluster pelanggan")
			}
			if err != nil {
				return nil, httpx.Internal(err)
			}
		}
		if input.Body.RouterID != nil {
			r, err := d.Store.GetRouter(ctx, tid, *input.Body.RouterID)
			if errors.Is(err, store.ErrNotFound) {
				return nil, httpx.BadRequest("router not found")
			}
			if err != nil {
				return nil, httpx.Internal(err)
			}
			if cust.ClusterID != nil && r.SiteID != nil && *r.SiteID != *cust.ClusterID {
				return nil, httpx.BadRequest("router harus dari cluster yang sama dengan pelanggan")
			}
			if cust.ClusterID != nil && r.SiteID == nil {
				return nil, httpx.BadRequest("router belum terhubung ke cluster")
			}
		}
		username := strings.TrimSpace(input.Body.Username)
		if username == "" {
			return nil, httpx.BadRequest("username is required")
		}
		st := cur.ServiceType
		if input.Body.ServiceType != "" {
			st = input.Body.ServiceType
		} else if plan.ServiceType != "" {
			st = plan.ServiceType
		}
		pw := strings.TrimSpace(input.Body.Password)
		updatePW := pw != ""
		var pwPtr *string
		if updatePW {
			pwPtr = &pw
		}
		cur.PlanID = input.Body.PlanID
		cur.RouterID = input.Body.RouterID
		cur.Username = username
		cur.ServiceType = st
		cur.Password = pwPtr
		if err := d.Store.UpdateSubscription(ctx, tid, cur, updatePW); err != nil {
			return nil, httpx.Internal(err)
		}

		// ODP reassignment
		if input.Body.ClearODP {
			_ = d.Store.ReleaseODPPortBySubscription(ctx, tid, cur.ID)
			cur.ODPID = nil
			cur.PortNumber = nil
			cur.ODPCode = ""
			cur.ODPName = ""
		} else if input.Body.ODPID != nil {
			odp, err := d.Store.GetODP(ctx, tid, *input.Body.ODPID)
			if errors.Is(err, store.ErrNotFound) {
				return nil, httpx.BadRequest("odp not found")
			}
			if err != nil {
				return nil, httpx.Internal(err)
			}
			if cust.ClusterID != nil && odp.ClusterID != nil && *cust.ClusterID != *odp.ClusterID {
				return nil, httpx.BadRequest("odp harus dari cluster yang sama dengan pelanggan")
			}
			sameODP := cur.ODPID != nil && *cur.ODPID == odp.ID
			samePort := sameODP && input.Body.PortNumber != nil && cur.PortNumber != nil && *input.Body.PortNumber == *cur.PortNumber
			if !samePort {
				_ = d.Store.ReleaseODPPortBySubscription(ctx, tid, cur.ID)
				custID := cur.CustomerID
				subID := cur.ID
				if input.Body.PortNumber != nil && *input.Body.PortNumber > 0 {
					if err := d.Store.AssignODPPort(ctx, tid, odp.ID, *input.Body.PortNumber, &custID, &subID); errors.Is(err, store.ErrNotFound) {
						return nil, httpx.BadRequest("port odp tidak tersedia")
					} else if err != nil {
						return nil, httpx.Internal(err)
					}
					n := *input.Body.PortNumber
					cur.PortNumber = &n
				} else {
					port, err := d.Store.AssignNextAvailableODPPort(ctx, tid, odp.ID, custID, subID)
					if errors.Is(err, store.ErrNotFound) {
						return nil, httpx.BadRequest("odp tidak punya port kosong")
					}
					if err != nil {
						return nil, httpx.Internal(err)
					}
					n := port.PortNumber
					cur.PortNumber = &n
				}
			}
			cur.ODPID = &odp.ID
			cur.ODPCode = odp.Code
			cur.ODPName = odp.Name
		}

		out, err := d.Store.GetSubscription(ctx, tid, cur.ID)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		out.Status = oldStatus // status unchanged by this endpoint
		out.CustomerName = cust.FullName
		out.CustomerCode = cust.CustomerCode
		out.PlanName = plan.Name

		// Sync secret ke RouterOS untuk langganan yang sudah pernah aktif.
		if err := syncSubscriptionToRouter(ctx, d, out, plan, oldUsername, oldRouterID); err != nil {
			out.Password = nil
			return nil, httpx.BadRequest("data tersimpan, tapi gagal sync RouterOS: " + err.Error())
		}
		out.Password = nil
		return &struct{ Body store.Subscription }{Body: *out}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "delete-subscription", Method: http.MethodDelete, Path: "/api/subscriptions/{id}",
		Tags: []string{"Subscriptions"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID xid.ID `path:"id"`
	}) (*struct{}, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		sub, err := d.Store.GetSubscription(ctx, tid, input.ID)
		if errors.Is(err, store.ErrNotFound) {
			return nil, httpx.NotFound("subscription not found")
		}
		if err != nil {
			return nil, httpx.Internal(err)
		}
		if sub.RouterID != nil {
			removeSubscriptionFromRouter(ctx, d, tid, sub)
		}
		if err := d.Store.DeleteSubscription(ctx, tid, input.ID); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return nil, httpx.NotFound("subscription not found")
			}
			return nil, httpx.Internal(err)
		}
		return &struct{}{}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "activate-subscription", Method: http.MethodPost, Path: "/api/subscriptions/{id}/activate",
		Tags: []string{"Subscriptions"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID   xid.ID `path:"id"`
		Body struct {
			// Accepts RFC 3339, local "YYYY-MM-DDTHH:mm:ss", or plain "YYYY-MM-DD".
			StartedAt  *string `json:"started_at,omitempty"`
			NextBillAt *string `json:"next_bill_at,omitempty"`
			Prorate    *bool   `json:"prorate,omitempty"`
		}
	}) (*struct {
		Body struct {
			Status       string `json:"status"`
			ProrateDays  int    `json:"prorate_days,omitempty"`
			PeriodDays   int    `json:"period_days,omitempty"`
			InvoiceTotal int64  `json:"invoice_total,omitempty"`
		}
	}, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		sub, err := d.Store.GetSubscription(ctx, tid, input.ID)
		if err != nil {
			return nil, httpx.NotFound("subscription not found")
		}
		if cust, cerr := d.Store.GetCustomer(ctx, tid, sub.CustomerID); cerr == nil && cust.IsDismantled() {
			return nil, httpx.BadRequest("pelanggan sudah cabut — tidak bisa mengaktifkan secret")
		}
		plan, err := d.Store.GetPlan(ctx, tid, sub.PlanID)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		if sub.RouterID != nil {
			r, err := d.Store.GetRouter(ctx, tid, *sub.RouterID)
			if err == nil {
				prov, err := d.Provisioner.Get(r.Provisioner)
				if err == nil {
					profile := ""
					if plan.ProfileName != nil {
						profile = strings.TrimSpace(*plan.ProfileName)
					}
					if profile == "" {
						profile = plan.Code
					}
					password := ""
					if sub.Password != nil {
						password = strings.TrimSpace(*sub.Password)
					}
					if password == "" {
						return nil, httpx.BadRequest("password PPPoE belum diisi untuk langganan ini")
					}
					spec := &provision.ServiceSpec{
						TenantID: tid, SubscriptionID: sub.ID, RouterID: *sub.RouterID,
						Username: sub.Username, Password: password, ServiceType: sub.ServiceType, ProfileName: profile,
						DownloadMbps: plan.DownloadMbps, UploadMbps: plan.UploadMbps,
						Comment: ownershipComment(ctx, d, tid, sub.CustomerCode, sub.CustomerName),
					}
					if sub.ServiceType == "hotspot" {
						applyPlanHotspotLimits(spec, plan)
					}
					applyIPAMToSpec(ctx, d, sub, spec)
					preferred := resolveOfferIPPoolID(ctx, d, tid, plan.ID, clusterIDForSubscription(ctx, d, sub))
					if p := resolveIPPoolForRouter(ctx, d, tid, *sub.RouterID, preferred); p != nil {
						_ = syncIPPoolToRouter(ctx, d, p)
						if spec.IPAddress == "" {
							spec.AddressPool = p.Name
							if p.Gateway != nil {
								spec.LocalAddress = strings.TrimSpace(*p.Gateway)
							}
						}
					}
					if ensurer, ok := prov.(provision.ProfileEnsurer); ok && profile != "" {
						price := plan.Price
						if cust, err := d.Store.GetCustomer(ctx, tid, sub.CustomerID); err == nil && cust != nil {
							if p, err := d.Store.ResolvePlanPrice(ctx, tid, plan.ID, cust.ClusterID); err == nil {
								price = p
							}
						}
						_ = ensurer.EnsureBandwidthProfile(ctx, tid, *sub.RouterID, profile, plan.DownloadMbps, plan.UploadMbps, sub.ServiceType, spec.AddressPool, price)
					}
					_ = prov.Apply(ctx, spec)
				}
			}
		}

		now := time.Now()
		start := now
		if st, ok, perr := parseFlexibleDateTime(input.Body.StartedAt, now.Location()); perr != nil {
			return nil, perr
		} else if ok {
			start = st.In(now.Location())
		}
		// Normalize to local noon to avoid timezone day-shift when only a date is sent.
		start = time.Date(start.Year(), start.Month(), start.Day(), 12, 0, 0, 0, now.Location())

		useProrate := true
		if input.Body.Prorate != nil {
			useProrate = *input.Body.Prorate
		}

		var nextBill time.Time
		if useProrate {
			if nb, ok, perr := parseFlexibleDateTime(input.Body.NextBillAt, now.Location()); perr != nil {
				return nil, perr
			} else if ok {
				n := nb.In(now.Location())
				nextBill = time.Date(n.Year(), n.Month(), n.Day(), 12, 0, 0, 0, now.Location())
				if !nextBill.After(start) {
					return nil, httpx.BadRequest("tanggal tagihan berikutnya harus setelah tanggal mulai")
				}
			} else {
				nextBill = billing.NextCycleAnchor(start, d.Store.BillingCycleStartDay(ctx, tid))
			}
		} else {
			nextBill = d.Billing.NextBillDate(start, plan.BillingCycle)
		}

		_, _ = d.Store.Pool.Exec(ctx, `
			UPDATE subscriptions SET status='active', started_at=$3, next_bill_at=$4, updated_at=NOW()
			WHERE tenant_id=$1 AND id=$2
		`, tid, input.ID, start, nextBill)
		sub.Status = "active"
		sub.StartedAt = &start
		sub.NextBillAt = &nextBill

		inv, err := d.Billing.GenerateInvoiceForSubscription(ctx, tid, input.ID)
		if err != nil {
			return nil, httpx.Internal(err)
		}

		cycleDays := 30
		switch plan.BillingCycle {
		case "daily":
			cycleDays = 1
		case "weekly":
			cycleDays = 7
		case "yearly":
			cycleDays = 365
		}
		prorateDays := 0
		if useProrate {
			prorateDays = int(math.Ceil(nextBill.Sub(start).Hours() / 24))
			if prorateDays < 1 {
				prorateDays = 1
			}
			if prorateDays > cycleDays {
				prorateDays = cycleDays
			}
			if prorateDays >= cycleDays {
				prorateDays = 0
			}
		}
		out := &struct {
			Body struct {
				Status       string `json:"status"`
				ProrateDays  int    `json:"prorate_days,omitempty"`
				PeriodDays   int    `json:"period_days,omitempty"`
				InvoiceTotal int64  `json:"invoice_total,omitempty"`
			}
		}{}
		out.Body.Status = "active"
		if prorateDays > 0 {
			out.Body.ProrateDays = prorateDays
			out.Body.PeriodDays = cycleDays
		}
		if inv != nil {
			out.Body.InvoiceTotal = inv.TotalAmount
		}
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "suspend-subscription", Method: http.MethodPost, Path: "/api/subscriptions/{id}/suspend",
		Tags: []string{"Subscriptions"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID xid.ID `path:"id"`
	}) (*struct {
		Body struct {
			Status string `json:"status"`
		}
	}, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		if _, err := d.Store.GetSubscription(ctx, tid, input.ID); err != nil {
			return nil, httpx.NotFound("subscription not found")
		}
		if err := isolirSubscription(ctx, d, tid, input.ID); err != nil {
			return nil, err
		}
		out := &struct {
			Body struct {
				Status string `json:"status"`
			}
		}{}
		out.Body.Status = "suspended"
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "resume-subscription", Method: http.MethodPost, Path: "/api/subscriptions/{id}/resume",
		Tags: []string{"Subscriptions"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID xid.ID `path:"id"`
	}) (*struct {
		Body struct {
			Status string `json:"status"`
		}
	}, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		if _, err := d.Store.GetSubscription(ctx, tid, input.ID); err != nil {
			return nil, httpx.NotFound("subscription not found")
		}
		if err := d.Store.UpdateSubscriptionStatus(ctx, tid, input.ID, "active"); err != nil {
			return nil, httpx.Internal(err)
		}
		resumeSubscription(ctx, d, tid, input.ID)
		out := &struct {
			Body struct {
				Status string `json:"status"`
			}
		}{}
		out.Body.Status = "active"
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "preview-change-plan", Method: http.MethodPost, Path: "/api/subscriptions/{id}/change-plan/preview",
		Tags: []string{"Subscriptions"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID   xid.ID `path:"id"`
		Body struct {
			PlanID xid.ID `json:"plan_id"`
		}
	}) (*struct{ Body billing.PlanChangeQuote }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		if xid.IsNil(input.Body.PlanID) {
			return nil, httpx.BadRequest("plan_id wajib")
		}
		sub, err := d.Store.GetSubscription(ctx, tid, input.ID)
		if errors.Is(err, store.ErrNotFound) {
			return nil, httpx.NotFound("subscription not found")
		}
		if err != nil {
			return nil, httpx.Internal(err)
		}
		cust, err := d.Store.GetCustomer(ctx, tid, sub.CustomerID)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		if err := ensurePlanOfferedForCustomer(ctx, d, tid, input.Body.PlanID, cust); err != nil {
			return nil, err
		}
		q, _, _, _, err := d.Billing.QuotePlanChange(ctx, tid, input.ID, input.Body.PlanID, time.Now())
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return nil, httpx.NotFound(err.Error())
			}
			return nil, httpx.BadRequest(err.Error())
		}
		return &struct{ Body billing.PlanChangeQuote }{Body: *q}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "change-subscription-plan", Method: http.MethodPost, Path: "/api/subscriptions/{id}/change-plan",
		Tags: []string{"Subscriptions"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID   xid.ID `path:"id"`
		Body struct {
			PlanID xid.ID `json:"plan_id"`
		}
	}) (*struct {
		Body struct {
			Quote   billing.PlanChangeQuote `json:"quote"`
			Invoice *store.Invoice          `json:"invoice,omitempty"`
			Status  string                  `json:"status"`
		}
	}, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		if xid.IsNil(input.Body.PlanID) {
			return nil, httpx.BadRequest("plan_id wajib")
		}
		sub, err := d.Store.GetSubscription(ctx, tid, input.ID)
		if errors.Is(err, store.ErrNotFound) {
			return nil, httpx.NotFound("subscription not found")
		}
		if err != nil {
			return nil, httpx.Internal(err)
		}
		cust, err := d.Store.GetCustomer(ctx, tid, sub.CustomerID)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		if err := ensurePlanOfferedForCustomer(ctx, d, tid, input.Body.PlanID, cust); err != nil {
			return nil, err
		}

		q, inv, outSub, err := applyPlanChangeToRouter(ctx, d, tid, sub, cust, input.Body.PlanID)
		if err != nil {
			return nil, err
		}

		return &struct {
			Body struct {
				Quote   billing.PlanChangeQuote `json:"quote"`
				Invoice *store.Invoice          `json:"invoice,omitempty"`
				Status  string                  `json:"status"`
			}
		}{Body: struct {
			Quote   billing.PlanChangeQuote `json:"quote"`
			Invoice *store.Invoice          `json:"invoice,omitempty"`
			Status  string                  `json:"status"`
		}{Quote: *q, Invoice: inv, Status: outSub.Status}}, nil
	})
}

func registerRouters(api huma.API, d *Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "list-routers", Method: http.MethodGet, Path: "/api/routers",
		Tags: []string{"Routers"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, _ *struct{}) (*struct{ Body []store.Router }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		list, err := d.Store.ListRouters(ctx, tid)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body []store.Router }{Body: list}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "create-router", Method: http.MethodPost, Path: "/api/routers",
		Tags: []string{"Routers"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Body struct {
			Name        string  `json:"name"`
			Address     string  `json:"address"`
			Port        int     `json:"port"`
			UseTLS      bool    `json:"use_tls"`
			Provisioner string  `json:"provisioner"`
			Username    string  `json:"username"`
			Password    string  `json:"password"`
			ClusterID   *xid.ID `json:"cluster_id,omitempty"`
		}
	}) (*struct{ Body store.Router }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		port := input.Body.Port
		if port == 0 {
			port = 8728
		}
		enc, err := d.Encryptor.Encrypt(input.Body.Password)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		prov := input.Body.Provisioner
		if prov == "" {
			prov = "routeros"
		}
		if input.Body.ClusterID != nil {
			if _, err := d.Store.GetCluster(ctx, tid, *input.Body.ClusterID); errors.Is(err, store.ErrNotFound) {
				return nil, httpx.BadRequest("cluster not found")
			} else if err != nil {
				return nil, httpx.Internal(err)
			}
		}
		r := &store.Router{
			TenantID: tid, SiteID: input.Body.ClusterID, Name: input.Body.Name, Address: input.Body.Address, Port: port,
			UseTLS: input.Body.UseTLS, Provisioner: prov, Username: input.Body.Username,
			PasswordEnc: enc, IsActive: true,
		}
		if err := d.Store.CreateRouter(ctx, r); err != nil {
			return nil, httpx.Internal(err)
		}
		r.PasswordEnc = nil
		return &struct{ Body store.Router }{Body: *r}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-router", Method: http.MethodGet, Path: "/api/routers/{id}",
		Tags: []string{"Routers"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID xid.ID `path:"id"`
	}) (*struct{ Body store.Router }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		r, err := d.Store.GetRouter(ctx, tid, input.ID)
		if errors.Is(err, store.ErrNotFound) {
			return nil, httpx.NotFound("router not found")
		}
		if err != nil {
			return nil, httpx.Internal(err)
		}
		r.PasswordEnc = nil
		return &struct{ Body store.Router }{Body: *r}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "update-router", Method: http.MethodPut, Path: "/api/routers/{id}",
		Tags: []string{"Routers"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID   xid.ID `path:"id"`
		Body struct {
			Name        string  `json:"name"`
			Address     string  `json:"address"`
			Port        int     `json:"port"`
			UseTLS      bool    `json:"use_tls"`
			Provisioner string  `json:"provisioner"`
			Username    string  `json:"username"`
			Password    string  `json:"password,omitempty"`
			IsActive    bool    `json:"is_active"`
			ClusterID   *xid.ID `json:"cluster_id,omitempty"`
		}
	}) (*struct{ Body store.Router }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		r, err := d.Store.GetRouter(ctx, tid, input.ID)
		if errors.Is(err, store.ErrNotFound) {
			return nil, httpx.NotFound("router not found")
		}
		if err != nil {
			return nil, httpx.Internal(err)
		}
		name := strings.TrimSpace(input.Body.Name)
		addr := strings.TrimSpace(input.Body.Address)
		if name == "" || addr == "" {
			return nil, httpx.BadRequest("name and address are required")
		}
		port := input.Body.Port
		if port == 0 {
			port = 8728
		}
		prov := input.Body.Provisioner
		if prov == "" {
			prov = "routeros"
		}
		if input.Body.ClusterID != nil {
			if _, err := d.Store.GetCluster(ctx, tid, *input.Body.ClusterID); errors.Is(err, store.ErrNotFound) {
				return nil, httpx.BadRequest("cluster not found")
			} else if err != nil {
				return nil, httpx.Internal(err)
			}
		}
		r.Name = name
		r.Address = addr
		r.Port = port
		r.UseTLS = input.Body.UseTLS
		r.Provisioner = prov
		r.Username = strings.TrimSpace(input.Body.Username)
		r.IsActive = input.Body.IsActive
		r.SiteID = input.Body.ClusterID
		if input.Body.Password != "" {
			enc, err := d.Encryptor.Encrypt(input.Body.Password)
			if err != nil {
				return nil, httpx.Internal(err)
			}
			r.PasswordEnc = enc
		}
		if err := d.Store.UpdateRouter(ctx, r); err != nil {
			return nil, httpx.Internal(err)
		}
		r.PasswordEnc = nil
		return &struct{ Body store.Router }{Body: *r}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "delete-router", Method: http.MethodDelete, Path: "/api/routers/{id}",
		Tags: []string{"Routers"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID xid.ID `path:"id"`
	}) (*struct {
		Body map[string]string
	}, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		blockers, err := d.Store.RouterBlockers(ctx, tid, input.ID)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		if len(blockers) > 0 {
			return nil, httpx.BadRequest(
				"Router tidak bisa dihapus karena masih dipakai: " + strings.Join(blockers, ", ") +
					". Pindahkan atau hapus data tersebut dulu.",
			)
		}
		if err := d.Store.DeleteRouter(ctx, tid, input.ID); errors.Is(err, store.ErrNotFound) {
			return nil, httpx.NotFound("router not found")
		} else if err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body map[string]string }{Body: map[string]string{"status": "deleted"}}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "test-router", Method: http.MethodPost, Path: "/api/routers/{id}/test",
		Tags: []string{"Routers"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID xid.ID `path:"id"`
	}) (*struct {
		Body struct {
			Success bool   `json:"success"`
			Message string `json:"message"`
		}
	}, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		r, err := d.Store.GetRouter(ctx, tid, input.ID)
		if err != nil {
			return nil, httpx.NotFound("router not found")
		}
		prov, err := d.Provisioner.Get(r.Provisioner)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		info, err := prov.TestConnection(ctx, input.ID)
		out := &struct {
			Body struct {
				Success bool   `json:"success"`
				Message string `json:"message"`
			}
		}{}
		if err != nil {
			out.Body.Success = false
			out.Body.Message = err.Error()
			msg := err.Error()
			_ = d.Store.UpdateRouterStatus(ctx, tid, input.ID, nil, &msg)
			return out, nil
		}
		out.Body.Success = true
		out.Body.Message = info
		if out.Body.Message == "" {
			out.Body.Message = "connected"
		}
		now := time.Now()
		_ = d.Store.UpdateRouterStatus(ctx, tid, input.ID, &now, nil)
		return out, nil
	})
}

// Tipe bernama untuk create-manual-invoice: huma menamai struct anonim dari
// nama field ("Items" → "Item") sehingga bentrok dengan endpoint lain.
type manualInvoiceItemInput struct {
	Description string `json:"description"`
	Quantity    int    `json:"quantity"`
	UnitPrice   int64  `json:"unit_price"`
}

type manualInvoiceInput struct {
	CustomerID     string                   `json:"customer_id"`
	DueDate        string                   `json:"due_date,omitempty"`
	DiscountAmount int64                    `json:"discount_amount,omitempty"`
	TaxPercent     float64                  `json:"tax_percent,omitempty"`
	Items          []manualInvoiceItemInput `json:"items"`
}

type manualInvoiceOutput struct {
	ID             xid.ID `json:"id"`
	InvoiceNumber  string `json:"invoice_number"`
	TotalAmount    int64  `json:"total_amount"`
	DueDate        string `json:"due_date"`
	WhatsAppQueued bool   `json:"whatsapp_queued"`
}

func registerInvoices(api huma.API, d *Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "create-manual-invoice", Method: http.MethodPost, Path: "/api/invoices",
		Tags: []string{"Invoices"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Body manualInvoiceInput
	}) (*struct {
		Body manualInvoiceOutput
	}, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		cid, err := xid.Parse(strings.TrimSpace(input.Body.CustomerID))
		if err != nil {
			return nil, httpx.BadRequest("customer_id tidak valid")
		}
		cust, err := d.Store.GetCustomer(ctx, tid, cid)
		if err != nil {
			return nil, httpx.NotFound("pelanggan tidak ditemukan")
		}
		if cust.ServiceStatus == "dismantled" {
			return nil, httpx.BadRequest("pelanggan sudah dismantle")
		}
		if len(input.Body.Items) == 0 {
			return nil, httpx.BadRequest("minimal satu item tagihan")
		}
		var subtotal int64
		items := make([]store.InvoiceItem, 0, len(input.Body.Items))
		for _, it := range input.Body.Items {
			desc := strings.TrimSpace(it.Description)
			if desc == "" {
				return nil, httpx.BadRequest("deskripsi item tidak boleh kosong")
			}
			if it.Quantity < 1 {
				return nil, httpx.BadRequest("qty item minimal 1")
			}
			if it.UnitPrice < 0 {
				return nil, httpx.BadRequest("harga item tidak boleh negatif")
			}
			amount := int64(it.Quantity) * it.UnitPrice
			subtotal += amount
			items = append(items, store.InvoiceItem{
				Description: desc,
				Quantity:    it.Quantity,
				UnitPrice:   it.UnitPrice,
				Amount:      amount,
			})
		}
		discount := input.Body.DiscountAmount
		if discount < 0 || discount > subtotal {
			return nil, httpx.BadRequest("diskon tidak valid")
		}
		taxPct := input.Body.TaxPercent
		if taxPct < 0 || taxPct > 100 {
			return nil, httpx.BadRequest("pajak tidak valid")
		}
		taxable := subtotal - discount
		tax := int64(math.Round(float64(taxable) * taxPct / 100))
		total := taxable + tax
		if total <= 0 {
			return nil, httpx.BadRequest("total tagihan harus lebih dari 0")
		}
		var dueDate time.Time
		if s := strings.TrimSpace(input.Body.DueDate); s != "" {
			dueDate, err = time.Parse("2006-01-02", s)
			if err != nil {
				return nil, httpx.BadRequest("due_date harus format YYYY-MM-DD")
			}
		} else {
			dueDate = billing.NextDueDate(time.Now(), d.Store.InvoiceDueDay(ctx, tid))
		}
		invNum, err := d.Store.NextInvoiceNumber(ctx, tid, cust.CustomerCode)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		inv := &store.Invoice{
			TenantID:       tid,
			CustomerID:     cid,
			InvoiceNumber:  invNum,
			Subtotal:       subtotal,
			TaxAmount:      tax,
			DiscountAmount: discount,
			TotalAmount:    total,
			Status:         "issued",
			DueDate:        dueDate,
		}
		if err := d.Store.CreateInvoice(ctx, inv, items); err != nil {
			return nil, httpx.Internal(err)
		}
		waQueued := false
		planName := d.Store.PlanNameForSubscription(ctx, tid, inv.SubscriptionID)
		itemName := store.NotificationItemName(planName, items)
		walletHandled := false
		if d.Billing != nil {
			if auto, aerr := d.Billing.TryAutoPayInvoice(ctx, tid, inv.ID); aerr == nil && auto != nil && auto.WalletEnabled {
				walletHandled = true
				if auto.Paid && auto.Payment != nil {
					resumeAfterInvoicePaid(ctx, d, tid, inv)
					if d.Notify != nil && strings.TrimSpace(cust.Phone) != "" {
						if err := d.Notify.SendPaymentConfirmation(ctx, tid, cust.Phone, cust.FullName, planName, itemName, invNum, auto.Payment.Amount); err == nil {
							waQueued = true
						}
					}
				} else if d.Notify != nil && strings.TrimSpace(cust.Phone) != "" {
					if err := d.Notify.SendWalletInsufficient(ctx, tid, cust.Phone, cust.FullName, invNum, total, auto.Balance); err == nil {
						waQueued = true
					}
				}
			}
		}
		if !walletHandled && d.Notify != nil && strings.TrimSpace(cust.Phone) != "" {
			if err := d.Notify.SendInvoiceIssued(ctx, tid, cust.Phone, cust.FullName, planName, itemName, invNum, total, dueDate.Format("02/01/2006")); err != nil {
				slog.Warn("manual invoice whatsapp", "invoice", invNum, "err", err)
			} else {
				waQueued = true
			}
		}
		auditEvent(ctx, d, AuditInvoiceIssue, "invoice", &inv.ID, map[string]any{
			"invoice_number": invNum, "total": total, "items": len(items), "whatsapp_queued": waQueued,
		})
		out := &struct {
			Body manualInvoiceOutput
		}{}
		out.Body.ID = inv.ID
		out.Body.InvoiceNumber = invNum
		out.Body.TotalAmount = total
		out.Body.DueDate = dueDate.Format("2006-01-02")
		out.Body.WhatsAppQueued = waQueued
		return out, nil
	})
	huma.Register(api, huma.Operation{
		OperationID: "list-customer-unpaid-invoices", Method: http.MethodGet, Path: "/api/customers/{id}/unpaid-invoices",
		Tags: []string{"Invoices"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID xid.ID `path:"id"`
	}) (*struct {
		Body struct {
			Data []store.UnpaidInvoiceWithItems `json:"data"`
		}
	}, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		list, err := d.Store.ListCustomerUnpaidInvoicesWithItems(ctx, tid, input.ID)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		if list == nil {
			list = []store.UnpaidInvoiceWithItems{}
		}
		out := &struct {
			Body struct {
				Data []store.UnpaidInvoiceWithItems `json:"data"`
			}
		}{}
		out.Body.Data = list
		return out, nil
	})
	huma.Register(api, huma.Operation{
		OperationID: "list-invoices", Method: http.MethodGet, Path: "/api/invoices",
		Tags: []string{"Invoices"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Status  string `query:"status"`
		Search  string `query:"search"`
		Trashed bool   `query:"trashed"`
		Limit   int    `query:"limit"`
		Offset  int    `query:"offset"`
	}) (*struct {
		Body struct {
			Data  []store.Invoice `json:"data"`
			Total int64           `json:"total"`
		}
	}, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		list, total, err := d.Store.ListInvoices(ctx, tid, input.Status, input.Search, input.Trashed, input.Limit, input.Offset)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		out := &struct {
			Body struct {
				Data  []store.Invoice `json:"data"`
				Total int64           `json:"total"`
			}
		}{}
		out.Body.Data = list
		out.Body.Total = total
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "pay-invoice", Method: http.MethodPost, Path: "/api/invoices/{id}/pay",
		Tags: []string{"Invoices"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID   xid.ID `path:"id"`
		Body struct {
			Amount    int64  `json:"amount,omitempty"`
			Method    string `json:"method,omitempty"`
			Reference string `json:"reference,omitempty"`
		}
	}) (*struct{ Body store.Payment }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		inv, items, err := d.Store.GetInvoice(ctx, tid, input.ID)
		if err != nil {
			return nil, httpx.NotFound("invoice not found")
		}
		amount := input.Body.Amount
		if amount == 0 {
			amount = inv.TotalAmount - inv.PaidAmount
		}
		method := payment.NormalizeMethod(input.Body.Method)
		if method == "" {
			return nil, httpx.BadRequest("metode pembayaran wajib")
		}
		ref := input.Body.Reference
		p := &store.Payment{
			TenantID: tid, CustomerID: inv.CustomerID, InvoiceID: &input.ID,
			Amount: amount, Method: method, Status: "paid", Reference: &ref,
		}
		if err := d.Store.RecordPayment(ctx, p); err != nil {
			return nil, httpx.Internal(err)
		}
		_ = d.Store.CancelPendingPaymentIntentsForInvoice(ctx, tid, inv.ID)
		if cashID, revID, err := d.Store.FindCashAndRevenueAccounts(ctx, tid); err == nil && !xid.IsNil(cashID) && !xid.IsNil(revID) {
			_ = d.Store.RecordPaymentJournal(ctx, tid, amount, cashID, revID, inv.InvoiceNumber)
		}
		cust, _ := d.Store.GetCustomer(ctx, tid, inv.CustomerID)
		if cust != nil {
			planName := d.Store.PlanNameForSubscription(ctx, tid, inv.SubscriptionID)
			itemName := store.NotificationItemName(planName, items)
			_ = d.Notify.SendPaymentConfirmation(ctx, tid, cust.Phone, cust.FullName, planName, itemName, inv.InvoiceNumber, amount)
		}
		resumeAfterInvoicePaid(ctx, d, tid, inv)
		auditEvent(ctx, d, AuditInvoicePay, "invoice", &inv.ID, map[string]any{
			"invoice_number": inv.InvoiceNumber, "amount": amount, "method": method,
		})
		return &struct{ Body store.Payment }{Body: *p}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "delete-invoice", Method: http.MethodDelete, Path: "/api/invoices/{id}",
		Tags: []string{"Invoices"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID xid.ID `path:"id"`
	}) (*struct{ Body map[string]string }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		if err := d.Store.DeleteInvoice(ctx, tid, input.ID); errors.Is(err, store.ErrNotFound) {
			return nil, httpx.NotFound("tagihan tidak ditemukan")
		} else if err != nil {
			return nil, httpx.Internal(err)
		}
		auditEvent(ctx, d, AuditInvoiceDelete, "invoice", &input.ID, nil)
		return &struct{ Body map[string]string }{Body: map[string]string{"status": "deleted"}}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "restore-invoice", Method: http.MethodPost, Path: "/api/invoices/{id}/restore",
		Tags: []string{"Invoices"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID xid.ID `path:"id"`
	}) (*struct{ Body map[string]string }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		if err := d.Store.RestoreInvoice(ctx, tid, input.ID); errors.Is(err, store.ErrNotFound) {
			return nil, httpx.NotFound("tagihan tidak ditemukan")
		} else if err != nil {
			return nil, httpx.Internal(err)
		}
		auditEvent(ctx, d, AuditInvoiceRestore, "invoice", &input.ID, nil)
		return &struct{ Body map[string]string }{Body: map[string]string{"status": "restored"}}, nil
	})
	huma.Register(api, huma.Operation{
		OperationID: "apply-invoice-discount", Method: http.MethodPost, Path: "/api/invoices/{id}/apply-discount",
		Tags: []string{"Invoices"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID xid.ID `path:"id"`
	}) (*struct {
		Body struct {
			DiscountName   string `json:"discount_name"`
			DiscountAmount int64  `json:"discount_amount"`
			TotalAmount    int64  `json:"total_amount"`
		}
	}, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		inv, items, err := d.Store.GetInvoice(ctx, tid, input.ID)
		if errors.Is(err, store.ErrNotFound) {
			return nil, httpx.NotFound("tagihan tidak ditemukan")
		} else if err != nil {
			return nil, httpx.Internal(err)
		}
		switch strings.ToLower(strings.TrimSpace(inv.Status)) {
		case "paid", "void", "cancelled":
			return nil, httpx.BadRequest("tagihan sudah lunas / tidak bisa diubah")
		}
		if inv.SubscriptionID == nil || xid.IsNil(*inv.SubscriptionID) {
			return nil, httpx.BadRequest("hanya tagihan langganan yang bisa diterapkan aturan diskon")
		}
		if inv.DiscountAmount > 0 {
			return nil, httpx.BadRequest("tagihan ini sudah punya diskon manual")
		}
		for _, it := range items {
			// Engine menandai diskon bawaan di deskripsi: "Nama −20%".
			if strings.Contains(it.Description, "−") && strings.HasSuffix(strings.TrimSpace(it.Description), ")") {
				return nil, httpx.BadRequest("diskon sudah termasuk saat tagihan dibuat (lihat deskripsi item)")
			}
		}
		sub, err := d.Store.GetSubscription(ctx, tid, *inv.SubscriptionID)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		plan, err := d.Store.GetPlan(ctx, tid, sub.PlanID)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		cust, err := d.Store.GetCustomer(ctx, tid, sub.CustomerID)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		var lateFee int64
		var planBase int64
		for _, it := range items {
			if strings.HasPrefix(strings.TrimSpace(it.Description), "Denda keterlambatan") {
				lateFee += it.Amount
			} else {
				planBase += it.Amount
			}
		}
		if planBase <= 0 {
			return nil, httpx.BadRequest("tidak ada nominal langganan yang bisa didiskon")
		}
		disc, err := d.Store.FindBestPlanDiscount(ctx, tid, plan.ID, cust.ID, time.Now().Format("2006-01-02"), planBase)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		if disc == nil {
			return nil, httpx.BadRequest("tidak ada aturan diskon yang cocok untuk tagihan ini")
		}
		discountAmt := disc.Value
		if disc.Kind == store.DiscountKindPercent {
			discountAmt = int64(math.Round(float64(planBase) * float64(disc.Value) / 100))
		}
		if discountAmt > planBase {
			discountAmt = planBase
		}
		if discountAmt <= 0 {
			return nil, httpx.BadRequest("tidak ada aturan diskon yang cocok untuk tagihan ini")
		}
		taxPct := plan.TaxPercent
		if pct, terr := d.Store.EffectiveTaxPercent(ctx, tid); terr == nil {
			taxPct = pct
		}
		taxable := planBase - discountAmt
		tax := int64(math.Round(float64(taxable) * taxPct / 100))
		total := taxable + tax + lateFee
		if total <= 0 {
			return nil, httpx.BadRequest("total baru tidak valid")
		}
		if total < inv.PaidAmount {
			return nil, httpx.BadRequest("total baru lebih kecil dari yang sudah dibayar")
		}
		if err := d.Store.SetInvoiceDiscountAmounts(ctx, tid, inv.ID, discountAmt, tax, total); err != nil {
			return nil, httpx.Internal(err)
		}
		auditEvent(ctx, d, AuditInvoiceDiscount, "invoice", &inv.ID, map[string]any{
			"invoice_number": inv.InvoiceNumber, "discount": disc.Name, "discount_amount": discountAmt, "total": total,
		})
		out := &struct {
			Body struct {
				DiscountName   string `json:"discount_name"`
				DiscountAmount int64  `json:"discount_amount"`
				TotalAmount    int64  `json:"total_amount"`
			}
		}{}
		out.Body.DiscountName = disc.Name
		out.Body.DiscountAmount = discountAmt
		out.Body.TotalAmount = total
		return out, nil
	})
	huma.Register(api, huma.Operation{
		OperationID: "purge-invoice", Method: http.MethodDelete, Path: "/api/invoices/{id}/purge",
		Tags: []string{"Invoices"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID xid.ID `path:"id"`
	}) (*struct{ Body map[string]string }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		if err := d.Store.PurgeInvoice(ctx, tid, input.ID); errors.Is(err, store.ErrNotFound) {
			return nil, httpx.NotFound("tagihan tidak ditemukan")
		} else if errors.Is(err, store.ErrInvoiceNotTrashed) || errors.Is(err, store.ErrInvoiceHasPaid) {
			return nil, httpx.BadRequest(err.Error())
		} else if err != nil {
			return nil, httpx.Internal(err)
		}
		auditEvent(ctx, d, AuditInvoicePurge, "invoice", &input.ID, nil)
		return &struct{ Body map[string]string }{Body: map[string]string{"status": "purged"}}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-payments", Method: http.MethodGet, Path: "/api/payments",
		Tags: []string{"Payments"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Search  string `query:"search"`
		Trashed bool   `query:"trashed"`
		Limit   int    `query:"limit"`
		Offset  int    `query:"offset"`
	}) (*struct {
		Body struct {
			Data  []store.Payment `json:"data"`
			Total int64           `json:"total"`
		}
	}, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		list, total, err := d.Store.ListPayments(ctx, tid, input.Search, input.Trashed, input.Limit, input.Offset)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		for i := range list {
			list[i].Method = payment.NormalizeMethod(list[i].Method)
		}
		out := &struct {
			Body struct {
				Data  []store.Payment `json:"data"`
				Total int64           `json:"total"`
			}
		}{}
		out.Body.Data = list
		out.Body.Total = total
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "delete-payment", Method: http.MethodDelete, Path: "/api/payments/{id}",
		Tags: []string{"Payments"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID xid.ID `path:"id"`
	}) (*struct{ Body map[string]string }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		if err := d.Store.DeletePayment(ctx, tid, input.ID); errors.Is(err, store.ErrNotFound) {
			return nil, httpx.NotFound("pembayaran tidak ditemukan")
		} else if err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body map[string]string }{Body: map[string]string{"status": "deleted"}}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "restore-payment", Method: http.MethodPost, Path: "/api/payments/{id}/restore",
		Tags: []string{"Payments"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID xid.ID `path:"id"`
	}) (*struct{ Body map[string]string }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		if err := d.Store.RestorePayment(ctx, tid, input.ID); errors.Is(err, store.ErrNotFound) {
			return nil, httpx.NotFound("pembayaran tidak ditemukan")
		} else if err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body map[string]string }{Body: map[string]string{"status": "restored"}}, nil
	})
}

func registerTickets(api huma.API, d *Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "list-tickets", Method: http.MethodGet, Path: "/api/tickets",
		Tags: []string{"Tickets"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Status string `query:"status"`
		Search string `query:"search"`
		Limit  int    `query:"limit"`
		Offset int    `query:"offset"`
	}) (*struct {
		Body struct {
			Data  []store.Ticket `json:"data"`
			Total int64          `json:"total"`
		}
	}, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		var assignedTo *xid.ID
		if isFieldOps(ctx, d) {
			uid := userIDFromCtx(ctx)
			assignedTo = &uid
		}
		list, total, err := d.Store.ListTickets(ctx, tid, input.Status, input.Search, assignedTo, input.Limit, input.Offset)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		if list == nil {
			list = []store.Ticket{}
		}
		out := &struct {
			Body struct {
				Data  []store.Ticket `json:"data"`
				Total int64          `json:"total"`
			}
		}{}
		out.Body.Data = list
		out.Body.Total = total
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "create-ticket", Method: http.MethodPost, Path: "/api/tickets",
		Tags: []string{"Tickets"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Body struct {
			CustomerID  *xid.ID  `json:"customer_id,omitempty"`
			Subject     string   `json:"subject"`
			Description string   `json:"description"`
			Category    string   `json:"category"`
			Priority    string   `json:"priority"`
			ImageURLs   []string `json:"image_urls,omitempty"`
		}
	}) (*struct{ Body store.Ticket }, error) {
		if err := requireDispatchOps(ctx, d); err != nil {
			return nil, err
		}
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		subject := strings.TrimSpace(input.Body.Subject)
		if subject == "" {
			return nil, httpx.BadRequest("subjek wajib")
		}
		desc := strings.TrimSpace(input.Body.Description)
		if desc == "" {
			return nil, httpx.BadRequest("deskripsi wajib")
		}
		t := &store.Ticket{
			TenantID: tid, CustomerID: input.Body.CustomerID, Subject: subject, Description: &desc,
			Category: input.Body.Category, Priority: input.Body.Priority, Status: "open",
		}
		if t.Category == "" {
			t.Category = "general"
		}
		if t.Priority == "" {
			t.Priority = "normal"
		}
		if err := d.Store.CreateTicket(ctx, t); err != nil {
			return nil, httpx.BadRequest(err.Error())
		}
		imageURLs := filterTenantUploadURLs(tid, input.Body.ImageURLs)
		if len(imageURLs) > 0 {
			var senderID *xid.ID
			if info, ok := tenant.FromContext(ctx); ok && !xid.IsNil(info.UserID) {
				uid := info.UserID
				senderID = &uid
			}
			if _, err := d.Store.AddTicketMessage(ctx, tid, t.ID, "staff", senderID, "", imageURLs); err != nil {
				return nil, httpx.BadRequest(err.Error())
			}
		}
		full, _ := d.Store.GetTicket(ctx, tid, t.ID)
		if full != nil {
			who := strings.TrimSpace(full.CustomerName)
			if who == "" {
				who = "tanpa pelanggan"
			}
			_ = d.Notify.QueueTicketTelegram(ctx, tid, full.CustomerID, full.Subject,
				"Prioritas: "+full.Priority,
				"Kategori: "+full.Category,
				"Pelanggan: "+who,
			)
			queueTicketAlert(ctx, d, tid, full)
			return &struct{ Body store.Ticket }{Body: *full}, nil
		}
		_ = d.Notify.QueueTicketTelegram(ctx, tid, t.CustomerID, t.Subject,
			"Prioritas: "+t.Priority,
			"Kategori: "+t.Category,
		)
		queueTicketAlert(ctx, d, tid, t)
		return &struct{ Body store.Ticket }{Body: *t}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-ticket", Method: http.MethodGet, Path: "/api/tickets/{id}",
		Tags: []string{"Tickets"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID xid.ID `path:"id"`
	}) (*struct{ Body store.Ticket }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		t, err := d.Store.GetTicket(ctx, tid, input.ID)
		if errors.Is(err, store.ErrNotFound) {
			return nil, httpx.NotFound("tiket tidak ditemukan")
		}
		if err != nil {
			return nil, httpx.Internal(err)
		}
		if err := enforceTicketAssigned(ctx, d, t); err != nil {
			return nil, err
		}
		return &struct{ Body store.Ticket }{Body: *t}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "update-ticket-status", Method: http.MethodPatch, Path: "/api/tickets/{id}/status",
		Tags: []string{"Tickets"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID   xid.ID `path:"id"`
		Body struct {
			Status string `json:"status"`
		}
	}) (*struct{ Body store.Ticket }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		cur, err := d.Store.GetTicket(ctx, tid, input.ID)
		if errors.Is(err, store.ErrNotFound) {
			return nil, httpx.NotFound("tiket tidak ditemukan")
		}
		if err != nil {
			return nil, httpx.Internal(err)
		}
		if err := enforceTicketAssigned(ctx, d, cur); err != nil {
			return nil, err
		}
		fromStatus := cur.Status
		newStatus := store.NormalizeTicketStatus(input.Body.Status)
		if err := d.Store.UpdateTicketStatus(ctx, tid, input.ID, input.Body.Status); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return nil, httpx.NotFound("tiket tidak ditemukan")
			}
			return nil, httpx.BadRequest(err.Error())
		}
		if newStatus != "" && fromStatus != newStatus {
			var actor *xid.ID
			if uid := userIDFromCtx(ctx); !xid.IsNil(uid) {
				actor = &uid
			}
			msg := fmt.Sprintf("Status: %s → %s", store.TicketStatusLabel(fromStatus), store.TicketStatusLabel(newStatus))
			_, _ = d.Store.AddTicketMessage(ctx, tid, input.ID, "system", actor, msg, nil)
		}
		t, err := d.Store.GetTicket(ctx, tid, input.ID)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body store.Ticket }{Body: *t}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "assign-ticket", Method: http.MethodPatch, Path: "/api/tickets/{id}/assign",
		Tags: []string{"Tickets"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID   xid.ID `path:"id"`
		Body struct {
			AssignedTo *xid.ID `json:"assigned_to,omitempty"`
		}
	}) (*struct{ Body store.Ticket }, error) {
		if err := requireDispatchOps(ctx, d); err != nil {
			return nil, err
		}
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		assigned := input.Body.AssignedTo
		if assigned == nil {
			if info, ok := tenant.FromContext(ctx); ok && !xid.IsNil(info.UserID) {
				uid := info.UserID
				assigned = &uid
			}
		}
		if err := d.Store.AssignTicket(ctx, tid, input.ID, assigned); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return nil, httpx.NotFound("tiket tidak ditemukan")
			}
			return nil, httpx.Internal(err)
		}
		t, err := d.Store.GetTicket(ctx, tid, input.ID)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body store.Ticket }{Body: *t}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-ticket-messages", Method: http.MethodGet, Path: "/api/tickets/{id}/messages",
		Tags: []string{"Tickets"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID xid.ID `path:"id"`
	}) (*struct{ Body []store.TicketMessage }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		cur, err := d.Store.GetTicket(ctx, tid, input.ID)
		if errors.Is(err, store.ErrNotFound) {
			return nil, httpx.NotFound("tiket tidak ditemukan")
		}
		if err != nil {
			return nil, httpx.Internal(err)
		}
		if err := enforceTicketAssigned(ctx, d, cur); err != nil {
			return nil, err
		}
		list, err := d.Store.ListTicketMessages(ctx, tid, input.ID)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		if list == nil {
			list = []store.TicketMessage{}
		}
		return &struct{ Body []store.TicketMessage }{Body: list}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "add-ticket-message", Method: http.MethodPost, Path: "/api/tickets/{id}/messages",
		Tags: []string{"Tickets"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID   xid.ID `path:"id"`
		Body struct {
			Message   string   `json:"message"`
			ImageURLs []string `json:"image_urls,omitempty"`
		}
	}) (*struct{ Body store.TicketMessage }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		cur, err := d.Store.GetTicket(ctx, tid, input.ID)
		if errors.Is(err, store.ErrNotFound) {
			return nil, httpx.NotFound("tiket tidak ditemukan")
		}
		if err != nil {
			return nil, httpx.Internal(err)
		}
		if err := enforceTicketAssigned(ctx, d, cur); err != nil {
			return nil, err
		}
		var senderID *xid.ID
		if info, ok := tenant.FromContext(ctx); ok && !xid.IsNil(info.UserID) {
			uid := info.UserID
			senderID = &uid
		}
		m, err := d.Store.AddTicketMessage(ctx, tid, input.ID, "staff", senderID, input.Body.Message, input.Body.ImageURLs)
		if err != nil {
			return nil, httpx.BadRequest(err.Error())
		}
		if store.NormalizeTicketStatus(cur.Status) == "open" {
			if err := d.Store.UpdateTicketStatus(ctx, tid, input.ID, "in_progress"); err == nil {
				msg := fmt.Sprintf("Status: %s → %s", store.TicketStatusLabel("open"), store.TicketStatusLabel("in_progress"))
				_, _ = d.Store.AddTicketMessage(ctx, tid, input.ID, "system", senderID, msg, nil)
			}
		}
		return &struct{ Body store.TicketMessage }{Body: *m}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "delete-ticket", Method: http.MethodDelete, Path: "/api/tickets/{id}",
		Tags: []string{"Tickets"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID xid.ID `path:"id"`
	}) (*struct{ Body map[string]string }, error) {
		if err := requireDispatchOps(ctx, d); err != nil {
			return nil, err
		}
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		if err := d.Store.DeleteTicket(ctx, tid, input.ID); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return nil, httpx.NotFound("tiket tidak ditemukan")
			}
			return nil, httpx.Internal(err)
		}
		return &struct{ Body map[string]string }{Body: map[string]string{"status": "deleted"}}, nil
	})
}

func registerODP(api huma.API, d *Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "list-odps", Method: http.MethodGet, Path: "/api/odps",
		Tags: []string{"FTTH"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, _ *struct{}) (*struct{ Body []store.ODP }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		list, err := d.Store.ListODPs(ctx, tid)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body []store.ODP }{Body: list}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "export-odps-csv", Method: http.MethodGet, Path: "/api/odps/export.csv",
		Summary: "Export ODP as CSV", Tags: []string{"FTTH"},
		Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ClusterID string `query:"cluster_id"`
	}) (*struct {
		ContentType        string `header:"Content-Type"`
		ContentDisposition string `header:"Content-Disposition"`
		Body               []byte
	}, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		list, err := d.Store.ListODPs(ctx, tid)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		clusterFilter, err := optionalQueryID(input.ClusterID)
		if err != nil {
			return nil, err
		}
		var buf bytes.Buffer
		w := csv.NewWriter(&buf)
		_ = w.Write([]string{"code", "name", "latitude", "longitude", "port_count", "address", "cluster_code"})
		clusters, _ := d.Store.ListClusters(ctx, tid)
		codeByID := map[xid.ID]string{}
		for _, c := range clusters {
			codeByID[c.ID] = c.Code
		}
		for _, o := range list {
			if clusterFilter != nil {
				if o.ClusterID == nil || *o.ClusterID != *clusterFilter {
					continue
				}
			}
			lat, lng, addr, ccode := "", "", "", ""
			if o.Latitude != nil {
				lat = strconv.FormatFloat(*o.Latitude, 'f', -1, 64)
			}
			if o.Longitude != nil {
				lng = strconv.FormatFloat(*o.Longitude, 'f', -1, 64)
			}
			if o.Address != nil {
				addr = *o.Address
			}
			if o.ClusterID != nil {
				ccode = codeByID[*o.ClusterID]
			}
			_ = w.Write([]string{o.Code, o.Name, lat, lng, strconv.Itoa(o.PortCount), addr, ccode})
		}
		w.Flush()
		if err := w.Error(); err != nil {
			return nil, httpx.Internal(err)
		}
		name := "odps.csv"
		if clusterFilter != nil {
			name = "odps-" + clusterFilter.String() + ".csv"
		}
		return &struct {
			ContentType        string `header:"Content-Type"`
			ContentDisposition string `header:"Content-Disposition"`
			Body               []byte
		}{
			ContentType:        "text/csv; charset=utf-8",
			ContentDisposition: fmt.Sprintf(`attachment; filename="%s"`, name),
			Body:               buf.Bytes(),
		}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "import-odps-csv", Method: http.MethodPost, Path: "/api/odps/import",
		Summary: "Import ODP from CSV (upsert by code)", Tags: []string{"FTTH"},
		Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Body struct {
			ClusterID *xid.ID `json:"cluster_id,omitempty"`
			CSV       string  `json:"csv"`
		}
	}) (*struct {
		Body struct {
			Created int      `json:"created"`
			Updated int      `json:"updated"`
			Skipped int      `json:"skipped"`
			Errors  []string `json:"errors,omitempty"`
		}
	}, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		raw := strings.TrimSpace(input.Body.CSV)
		if raw == "" {
			return nil, httpx.BadRequest("csv is required")
		}
		var defaultCluster *xid.ID
		if input.Body.ClusterID != nil {
			if _, err := d.Store.GetCluster(ctx, tid, *input.Body.ClusterID); errors.Is(err, store.ErrNotFound) {
				return nil, httpx.BadRequest("cluster not found")
			} else if err != nil {
				return nil, httpx.Internal(err)
			}
			defaultCluster = input.Body.ClusterID
		}
		clusters, err := d.Store.ListClusters(ctx, tid)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		idByCode := map[string]xid.ID{}
		for _, c := range clusters {
			idByCode[strings.ToUpper(c.Code)] = c.ID
		}

		r := csv.NewReader(strings.NewReader(raw))
		r.TrimLeadingSpace = true
		r.FieldsPerRecord = -1
		records, err := r.ReadAll()
		if err != nil {
			return nil, httpx.BadRequest("invalid csv: " + err.Error())
		}
		if len(records) == 0 {
			return nil, httpx.BadRequest("csv is empty")
		}

		header := map[string]int{}
		start := 0
		first := records[0]
		looksHeader := false
		for i, h := range first {
			key := strings.ToLower(strings.TrimSpace(h))
			header[key] = i
			if key == "code" || key == "name" {
				looksHeader = true
			}
		}
		if looksHeader {
			start = 1
		} else {
			header = map[string]int{"code": 0, "name": 1, "latitude": 2, "longitude": 3, "port_count": 4, "address": 5, "cluster_code": 6}
		}
		col := func(row []string, key string) string {
			i, ok := header[key]
			if !ok || i < 0 || i >= len(row) {
				return ""
			}
			return strings.TrimSpace(row[i])
		}

		created, updated, skipped := 0, 0, 0
		var errs []string
		for i := start; i < len(records); i++ {
			row := records[i]
			if len(row) == 0 || (len(row) == 1 && strings.TrimSpace(row[0]) == "") {
				continue
			}
			code := col(row, "code")
			name := col(row, "name")
			if code == "" || name == "" {
				skipped++
				errs = append(errs, fmt.Sprintf("baris %d: code dan name wajib", i+1))
				continue
			}
			portCount := 8
			if pc := col(row, "port_count"); pc != "" {
				if n, err := strconv.Atoi(pc); err == nil && n > 0 {
					portCount = n
				}
			}
			var lat, lng *float64
			if v := col(row, "latitude"); v != "" {
				if f, err := strconv.ParseFloat(v, 64); err == nil {
					lat = &f
				} else {
					errs = append(errs, fmt.Sprintf("baris %d: latitude tidak valid", i+1))
					skipped++
					continue
				}
			}
			if v := col(row, "longitude"); v != "" {
				if f, err := strconv.ParseFloat(v, 64); err == nil {
					lng = &f
				} else {
					errs = append(errs, fmt.Sprintf("baris %d: longitude tidak valid", i+1))
					skipped++
					continue
				}
			}
			var addr *string
			if v := col(row, "address"); v != "" {
				addr = &v
			}
			clusterID := defaultCluster
			if cc := col(row, "cluster_code"); cc != "" {
				if id, ok := idByCode[strings.ToUpper(cc)]; ok {
					cid := id
					clusterID = &cid
				} else {
					errs = append(errs, fmt.Sprintf("baris %d: cluster_code %q tidak ditemukan", i+1, cc))
					skipped++
					continue
				}
			}

			existing, err := d.Store.GetODPByCode(ctx, tid, code)
			if errors.Is(err, store.ErrNotFound) {
				o := &store.ODP{
					TenantID: tid, ClusterID: clusterID, Name: name, Code: code,
					Address: addr, Latitude: lat, Longitude: lng, PortCount: portCount,
				}
				if err := d.Store.CreateODP(ctx, o); err != nil {
					errs = append(errs, fmt.Sprintf("baris %d: gagal buat", i+1))
					skipped++
					continue
				}
				created++
				continue
			}
			if err != nil {
				errs = append(errs, fmt.Sprintf("baris %d: gagal baca", i+1))
				skipped++
				continue
			}
			existing.Name = name
			existing.Address = addr
			existing.Latitude = lat
			existing.Longitude = lng
			if clusterID != nil {
				existing.ClusterID = clusterID
			}
			if err := d.Store.UpdateODP(ctx, existing); err != nil {
				errs = append(errs, fmt.Sprintf("baris %d: gagal update", i+1))
				skipped++
				continue
			}
			updated++
		}

		out := &struct {
			Body struct {
				Created int      `json:"created"`
				Updated int      `json:"updated"`
				Skipped int      `json:"skipped"`
				Errors  []string `json:"errors,omitempty"`
			}
		}{}
		out.Body.Created = created
		out.Body.Updated = updated
		out.Body.Skipped = skipped
		if len(errs) > 20 {
			errs = append(errs[:20], fmt.Sprintf("… +%d error lain", len(errs)-20))
		}
		out.Body.Errors = errs
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "create-odp", Method: http.MethodPost, Path: "/api/odps",
		Tags: []string{"FTTH"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Body struct {
			Name             string   `json:"name"`
			Code             string   `json:"code"`
			Address          *string  `json:"address,omitempty"`
			Latitude         *float64 `json:"latitude,omitempty"`
			Longitude        *float64 `json:"longitude,omitempty"`
			CoverageRadiusKm *float64 `json:"coverage_radius_km,omitempty"`
			PortCount        int      `json:"port_count,omitempty"`
			ClusterID        *xid.ID  `json:"cluster_id,omitempty"`
			ODCID            *xid.ID  `json:"odc_id,omitempty"`
		}
	}) (*struct{ Body store.ODP }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		name := strings.TrimSpace(input.Body.Name)
		code := strings.TrimSpace(input.Body.Code)
		if name == "" || code == "" {
			return nil, httpx.BadRequest("name and code are required")
		}
		if input.Body.ClusterID != nil {
			if _, err := d.Store.GetCluster(ctx, tid, *input.Body.ClusterID); errors.Is(err, store.ErrNotFound) {
				return nil, httpx.BadRequest("cluster not found")
			} else if err != nil {
				return nil, httpx.Internal(err)
			}
		}
		o := store.ODP{
			TenantID: tid, ClusterID: input.Body.ClusterID, ODCID: input.Body.ODCID, Name: name, Code: code,
			Address: input.Body.Address, Latitude: input.Body.Latitude, Longitude: input.Body.Longitude,
			CoverageRadiusKm: input.Body.CoverageRadiusKm,
			PortCount:        input.Body.PortCount,
		}
		if o.PortCount <= 0 {
			o.PortCount = 8
		}
		if err := d.Store.CreateODP(ctx, &o); err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body store.ODP }{Body: o}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "update-odp", Method: http.MethodPut, Path: "/api/odps/{id}",
		Tags: []string{"FTTH"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID   xid.ID `path:"id"`
		Body struct {
			Name             string   `json:"name"`
			Code             string   `json:"code"`
			Address          *string  `json:"address,omitempty"`
			Latitude         *float64 `json:"latitude,omitempty"`
			Longitude        *float64 `json:"longitude,omitempty"`
			CoverageRadiusKm *float64 `json:"coverage_radius_km,omitempty"`
			ClusterID        *xid.ID  `json:"cluster_id,omitempty"`
			ODCID            *xid.ID  `json:"odc_id,omitempty"`
		}
	}) (*struct{ Body store.ODP }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		name := strings.TrimSpace(input.Body.Name)
		code := strings.TrimSpace(input.Body.Code)
		if name == "" || code == "" {
			return nil, httpx.BadRequest("name and code are required")
		}
		if input.Body.ClusterID != nil {
			if _, err := d.Store.GetCluster(ctx, tid, *input.Body.ClusterID); errors.Is(err, store.ErrNotFound) {
				return nil, httpx.BadRequest("cluster not found")
			} else if err != nil {
				return nil, httpx.Internal(err)
			}
		}
		o := store.ODP{
			ID: input.ID, TenantID: tid, ClusterID: input.Body.ClusterID, ODCID: input.Body.ODCID,
			Name: name, Code: code, Address: input.Body.Address,
			Latitude: input.Body.Latitude, Longitude: input.Body.Longitude,
			CoverageRadiusKm: input.Body.CoverageRadiusKm,
		}
		if err := d.Store.UpdateODP(ctx, &o); errors.Is(err, store.ErrNotFound) {
			return nil, httpx.NotFound("odp not found")
		} else if err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body store.ODP }{Body: o}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "put-odp-coverage", Method: http.MethodPut, Path: "/api/odps/{id}/coverage",
		Summary: "Set ODP coverage radius (km)", Tags: []string{"FTTH"},
		Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID   xid.ID `path:"id"`
		Body struct {
			CoverageRadiusKm *float64 `json:"coverage_radius_km"`
		}
	}) (*struct{ Body store.ODP }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		if err := d.Store.UpdateODPCoverage(ctx, tid, input.ID, input.Body.CoverageRadiusKm); errors.Is(err, store.ErrNotFound) {
			return nil, httpx.NotFound("odp not found")
		} else if err != nil {
			return nil, httpx.Internal(err)
		}
		o, err := d.Store.GetODP(ctx, tid, input.ID)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body store.ODP }{Body: *o}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "delete-odp", Method: http.MethodDelete, Path: "/api/odps/{id}",
		Tags: []string{"FTTH"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID xid.ID `path:"id"`
	}) (*struct{ Body map[string]string }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		if err := d.Store.DeleteODP(ctx, tid, input.ID); errors.Is(err, store.ErrNotFound) {
			return nil, httpx.NotFound("odp not found")
		} else if err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body map[string]string }{Body: map[string]string{"status": "deleted"}}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-odp-ports", Method: http.MethodGet, Path: "/api/odps/{id}/ports",
		Summary: "List ODP port slots (used/available)", Tags: []string{"FTTH"},
		Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID xid.ID `path:"id"`
	}) (*struct {
		Body struct {
			ODP   store.ODP       `json:"odp"`
			Ports []store.ODPPort `json:"ports"`
		}
	}, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		odp, err := d.Store.GetODP(ctx, tid, input.ID)
		if errors.Is(err, store.ErrNotFound) {
			return nil, httpx.NotFound("odp not found")
		}
		if err != nil {
			return nil, httpx.Internal(err)
		}
		ports, err := d.Store.ListODPPorts(ctx, tid, input.ID)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		out := &struct {
			Body struct {
				ODP   store.ODP       `json:"odp"`
				Ports []store.ODPPort `json:"ports"`
			}
		}{}
		out.Body.ODP = *odp
		out.Body.Ports = ports
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "ftth-map-assets", Method: http.MethodGet, Path: "/api/ftth/map-assets",
		Summary: "ODP + customers with coordinates for map",
		Tags:    []string{"FTTH"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, _ *struct{}) (*struct {
		Body struct {
			ODPs      []store.ODP      `json:"odps"`
			Customers []store.Customer `json:"customers"`
			Clusters  []store.Cluster  `json:"clusters"`
			Icons     store.MapIcons   `json:"icons"`
		}
	}, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		odps, err := d.Store.ListODPs(ctx, tid)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		custs, err := d.Store.ListCustomersWithCoords(ctx, tid)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		clusters, err := d.Store.ListClusters(ctx, tid)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		icons, err := d.Store.EffectiveMapIcons(ctx, tid)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		out := &struct {
			Body struct {
				ODPs      []store.ODP      `json:"odps"`
				Customers []store.Customer `json:"customers"`
				Clusters  []store.Cluster  `json:"clusters"`
				Icons     store.MapIcons   `json:"icons"`
			}
		}{}
		out.Body.ODPs = odps
		out.Body.Customers = custs
		out.Body.Clusters = clusters
		out.Body.Icons = icons
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "coverage-check", Method: http.MethodGet, Path: "/api/coverage-check",
		Summary: "Check whether a lat/lng is inside POP or ODP coverage",
		Tags:    []string{"FTTH"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Lat float64 `query:"lat"`
		Lng float64 `query:"lng"`
	}) (*struct{ Body store.CoverageCheckResult }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		res, err := d.Store.CheckCoverage(ctx, tid, input.Lat, input.Lng)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body store.CoverageCheckResult }{Body: res}, nil
	})
}

func registerCableRoutes(api huma.API, d *Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "list-cable-routes", Method: http.MethodGet, Path: "/api/cable-routes",
		Tags: []string{"FTTH"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ClusterID string `query:"cluster_id"`
	}) (*struct{ Body []store.CableRoute }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		clusterID, err := optionalQueryID(input.ClusterID)
		if err != nil {
			return nil, err
		}
		list, err := d.Store.ListCableRoutes(ctx, tid, clusterID)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body []store.CableRoute }{Body: list}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "export-cable-routes", Method: http.MethodGet, Path: "/api/cable-routes/export.json",
		Summary: "Backup cable routes as JSON", Tags: []string{"FTTH"},
		Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ClusterID string `query:"cluster_id"`
	}) (*struct {
		ContentType        string `header:"Content-Type"`
		ContentDisposition string `header:"Content-Disposition"`
		Body               []byte
	}, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		clusterID, err := optionalQueryID(input.ClusterID)
		if err != nil {
			return nil, err
		}
		list, err := d.Store.ListCableRoutes(ctx, tid, clusterID)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		clusterCode := ""
		if clusterID != nil {
			if c, err := d.Store.GetCluster(ctx, tid, *clusterID); err == nil {
				clusterCode = c.Code
			}
		}
		type routeItem struct {
			Name     string          `json:"name"`
			Path     json.RawMessage `json:"path"`
			Color    string          `json:"color,omitempty"`
			Notes    *string         `json:"notes,omitempty"`
			IsActive bool            `json:"is_active"`
		}
		payload := struct {
			Version     int         `json:"version"`
			ClusterCode string      `json:"cluster_code,omitempty"`
			ExportedAt  string      `json:"exported_at"`
			Routes      []routeItem `json:"routes"`
		}{
			Version:     1,
			ClusterCode: clusterCode,
			ExportedAt:  time.Now().UTC().Format(time.RFC3339),
			Routes:      make([]routeItem, 0, len(list)),
		}
		for _, r := range list {
			payload.Routes = append(payload.Routes, routeItem{
				Name: r.Name, Path: r.Path, Color: r.Color, Notes: r.Notes, IsActive: r.IsActive,
			})
		}
		raw, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return nil, httpx.Internal(err)
		}
		fname := "cable-routes.json"
		if clusterCode != "" {
			fname = "cable-routes-" + clusterCode + ".json"
		}
		return &struct {
			ContentType        string `header:"Content-Type"`
			ContentDisposition string `header:"Content-Disposition"`
			Body               []byte
		}{
			ContentType:        "application/json; charset=utf-8",
			ContentDisposition: fmt.Sprintf(`attachment; filename="%s"`, fname),
			Body:               raw,
		}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "import-cable-routes", Method: http.MethodPost, Path: "/api/cable-routes/import",
		Summary: "Restore cable routes from JSON backup (upsert by name+cluster)", Tags: []string{"FTTH"},
		Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Body struct {
			ClusterID *xid.ID         `json:"cluster_id,omitempty"`
			JSON      json.RawMessage `json:"json,omitempty"`
			Routes    []struct {
				Name     string          `json:"name"`
				Path     json.RawMessage `json:"path"`
				Color    string          `json:"color,omitempty"`
				Notes    *string         `json:"notes,omitempty"`
				IsActive *bool           `json:"is_active,omitempty"`
			} `json:"routes,omitempty"`
			ClusterCode string `json:"cluster_code,omitempty"`
		}
	}) (*struct {
		Body struct {
			Created int      `json:"created"`
			Updated int      `json:"updated"`
			Skipped int      `json:"skipped"`
			Errors  []string `json:"errors,omitempty"`
		}
	}, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}

		type routeIn struct {
			Name     string          `json:"name"`
			Path     json.RawMessage `json:"path"`
			Color    string          `json:"color,omitempty"`
			Notes    *string         `json:"notes,omitempty"`
			IsActive *bool           `json:"is_active,omitempty"`
		}
		var routes []routeIn
		var clusterCode string

		if len(input.Body.JSON) > 0 {
			var wrap struct {
				ClusterCode string    `json:"cluster_code"`
				Routes      []routeIn `json:"routes"`
			}
			if err := json.Unmarshal(input.Body.JSON, &wrap); err != nil {
				return nil, httpx.BadRequest("invalid json backup")
			}
			routes = wrap.Routes
			clusterCode = wrap.ClusterCode
		} else {
			for _, r := range input.Body.Routes {
				routes = append(routes, routeIn{
					Name: r.Name, Path: r.Path, Color: r.Color, Notes: r.Notes, IsActive: r.IsActive,
				})
			}
			clusterCode = input.Body.ClusterCode
		}
		if len(routes) == 0 {
			return nil, httpx.BadRequest("no routes to import")
		}

		var clusterID *xid.ID
		if input.Body.ClusterID != nil {
			if _, err := d.Store.GetCluster(ctx, tid, *input.Body.ClusterID); errors.Is(err, store.ErrNotFound) {
				return nil, httpx.BadRequest("cluster not found")
			} else if err != nil {
				return nil, httpx.Internal(err)
			}
			clusterID = input.Body.ClusterID
		} else if strings.TrimSpace(clusterCode) != "" {
			clusters, err := d.Store.ListClusters(ctx, tid)
			if err != nil {
				return nil, httpx.Internal(err)
			}
			want := strings.ToUpper(strings.TrimSpace(clusterCode))
			for _, c := range clusters {
				if strings.EqualFold(c.Code, want) {
					id := c.ID
					clusterID = &id
					break
				}
			}
			if clusterID == nil {
				return nil, httpx.BadRequest("cluster_code not found: " + clusterCode)
			}
		}

		created, updated, skipped := 0, 0, 0
		var errs []string
		for i, item := range routes {
			name := strings.TrimSpace(item.Name)
			if name == "" {
				skipped++
				errs = append(errs, fmt.Sprintf("item %d: name wajib", i+1))
				continue
			}
			var pathCoords [][]float64
			if err := json.Unmarshal(item.Path, &pathCoords); err != nil || len(pathCoords) < 2 {
				skipped++
				errs = append(errs, fmt.Sprintf("%s: path minimal 2 titik [[lng,lat],...]", name))
				continue
			}
			raw, err := json.Marshal(pathCoords)
			if err != nil {
				skipped++
				errs = append(errs, fmt.Sprintf("%s: path invalid", name))
				continue
			}
			active := true
			if item.IsActive != nil {
				active = *item.IsActive
			}
			color := item.Color
			if color == "" {
				color = "#8C7355"
			}

			existing, err := d.Store.GetCableRouteByName(ctx, tid, clusterID, name)
			if errors.Is(err, store.ErrNotFound) {
				r := &store.CableRoute{
					TenantID: tid, ClusterID: clusterID, Name: name,
					Path: raw, Color: color, Notes: item.Notes, IsActive: active,
					FromKind: "manual", ToKind: "manual",
				}
				if err := d.Store.CreateCableRoute(ctx, r); err != nil {
					skipped++
					errs = append(errs, fmt.Sprintf("%s: gagal buat", name))
					continue
				}
				created++
				continue
			}
			if err != nil {
				skipped++
				errs = append(errs, fmt.Sprintf("%s: gagal baca", name))
				continue
			}
			existing.Path = raw
			existing.Color = color
			existing.Notes = item.Notes
			existing.IsActive = active
			if clusterID != nil {
				existing.ClusterID = clusterID
			}
			if err := d.Store.UpdateCableRoute(ctx, existing); err != nil {
				skipped++
				errs = append(errs, fmt.Sprintf("%s: gagal update", name))
				continue
			}
			updated++
		}

		out := &struct {
			Body struct {
				Created int      `json:"created"`
				Updated int      `json:"updated"`
				Skipped int      `json:"skipped"`
				Errors  []string `json:"errors,omitempty"`
			}
		}{}
		out.Body.Created = created
		out.Body.Updated = updated
		out.Body.Skipped = skipped
		if len(errs) > 20 {
			errs = append(errs[:20], fmt.Sprintf("… +%d error lain", len(errs)-20))
		}
		out.Body.Errors = errs
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "create-cable-route", Method: http.MethodPost, Path: "/api/cable-routes",
		Tags: []string{"FTTH"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Body struct {
			Name      string      `json:"name"`
			ClusterID *xid.ID     `json:"cluster_id,omitempty"`
			FromKind  string      `json:"from_kind,omitempty"`
			FromID    *xid.ID     `json:"from_id,omitempty"`
			ToKind    string      `json:"to_kind,omitempty"`
			ToID      *xid.ID     `json:"to_id,omitempty"`
			Path      [][]float64 `json:"path"`
			Color     string      `json:"color,omitempty"`
			Notes     *string     `json:"notes,omitempty"`
			IsActive  *bool       `json:"is_active,omitempty"`
		}
	}) (*struct{ Body store.CableRoute }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		name := strings.TrimSpace(input.Body.Name)
		if name == "" {
			return nil, httpx.BadRequest("name is required")
		}
		if len(input.Body.Path) < 2 {
			return nil, httpx.BadRequest("path needs at least 2 points [[lng,lat],...]")
		}
		raw, err := json.Marshal(input.Body.Path)
		if err != nil {
			return nil, httpx.BadRequest("invalid path")
		}
		active := true
		if input.Body.IsActive != nil {
			active = *input.Body.IsActive
		}
		r := &store.CableRoute{
			TenantID: tid, ClusterID: input.Body.ClusterID, Name: name,
			FromKind: input.Body.FromKind, FromID: input.Body.FromID,
			ToKind: input.Body.ToKind, ToID: input.Body.ToID,
			Path: raw, Color: input.Body.Color, Notes: input.Body.Notes, IsActive: active,
		}
		if err := d.Store.CreateCableRoute(ctx, r); err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body store.CableRoute }{Body: *r}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "update-cable-route", Method: http.MethodPut, Path: "/api/cable-routes/{id}",
		Tags: []string{"FTTH"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID   xid.ID `path:"id"`
		Body struct {
			Name      string      `json:"name"`
			ClusterID *xid.ID     `json:"cluster_id,omitempty"`
			FromKind  string      `json:"from_kind,omitempty"`
			FromID    *xid.ID     `json:"from_id,omitempty"`
			ToKind    string      `json:"to_kind,omitempty"`
			ToID      *xid.ID     `json:"to_id,omitempty"`
			Path      [][]float64 `json:"path"`
			Color     string      `json:"color,omitempty"`
			Notes     *string     `json:"notes,omitempty"`
			IsActive  *bool       `json:"is_active,omitempty"`
		}
	}) (*struct{ Body store.CableRoute }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		existing, err := d.Store.GetCableRoute(ctx, tid, input.ID)
		if errors.Is(err, store.ErrNotFound) {
			return nil, httpx.NotFound("cable route not found")
		}
		if err != nil {
			return nil, httpx.Internal(err)
		}
		name := strings.TrimSpace(input.Body.Name)
		if name == "" {
			return nil, httpx.BadRequest("name is required")
		}
		if len(input.Body.Path) < 2 {
			return nil, httpx.BadRequest("path needs at least 2 points")
		}
		raw, err := json.Marshal(input.Body.Path)
		if err != nil {
			return nil, httpx.BadRequest("invalid path")
		}
		existing.Name = name
		existing.Path = raw
		if input.Body.Color != "" {
			existing.Color = input.Body.Color
		}
		if input.Body.Notes != nil {
			existing.Notes = input.Body.Notes
		}
		if input.Body.IsActive != nil {
			existing.IsActive = *input.Body.IsActive
		}
		if input.Body.FromKind != "" {
			existing.FromKind = input.Body.FromKind
			existing.FromID = input.Body.FromID
		}
		if input.Body.ToKind != "" {
			existing.ToKind = input.Body.ToKind
			existing.ToID = input.Body.ToID
		}
		if input.Body.ClusterID != nil {
			existing.ClusterID = input.Body.ClusterID
		}
		if err := d.Store.UpdateCableRoute(ctx, existing); err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body store.CableRoute }{Body: *existing}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "delete-cable-route", Method: http.MethodDelete, Path: "/api/cable-routes/{id}",
		Tags: []string{"FTTH"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID xid.ID `path:"id"`
	}) (*struct{ Body map[string]string }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		if err := d.Store.DeleteCableRoute(ctx, tid, input.ID); errors.Is(err, store.ErrNotFound) {
			return nil, httpx.NotFound("cable route not found")
		} else if err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body map[string]string }{Body: map[string]string{"status": "deleted"}}, nil
	})
}

func registerDashboard(api huma.API, d *Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "dashboard-stats", Method: http.MethodGet, Path: "/api/dashboard/stats",
		Tags: []string{"Dashboard"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, _ *struct{}) (*struct{ Body map[string]any }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		var stats map[string]any
		if isFieldOps(ctx, d) {
			uid := userIDFromCtx(ctx)
			stats, err = d.Store.FieldOpsDashboardStats(ctx, tid, uid)
		} else {
			stats, err = d.Store.DashboardStats(ctx, tid)
		}
		if err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body map[string]any }{Body: stats}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "revenue-chart", Method: http.MethodGet, Path: "/api/dashboard/revenue-chart",
		Tags: []string{"Dashboard"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Months int `query:"months"`
	}) (*struct {
		Body []map[string]any `json:"data"`
	}, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		data, err := d.Store.RevenueChart(ctx, tid, input.Months)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct {
			Body []map[string]any `json:"data"`
		}{Body: data}, nil
	})
}

func registerSearch(api huma.API, d *Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "global-search", Method: http.MethodGet, Path: "/api/search",
		Summary: "Search customers, subscriptions, invoices, ODP, plans, routers, clusters",
		Tags:    []string{"Search"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Q     string `query:"q" minLength:"1"`
		Limit int    `query:"limit"`
	}) (*struct {
		Body struct {
			Query   string            `json:"query"`
			Results []store.SearchHit `json:"results"`
		}
	}, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		q := strings.TrimSpace(input.Q)
		if q == "" {
			return nil, httpx.BadRequest("q is required")
		}
		hits, err := d.Store.GlobalSearch(ctx, tid, q, input.Limit)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		out := &struct {
			Body struct {
				Query   string            `json:"query"`
				Results []store.SearchHit `json:"results"`
			}
		}{}
		out.Body.Query = q
		out.Body.Results = hits
		return out, nil
	})
}

func ensurePlanOfferedForCustomer(ctx context.Context, d *Deps, tid, planID xid.ID, cust *store.Customer) error {
	if cust == nil || cust.ClusterID == nil || xid.IsNil(*cust.ClusterID) {
		return nil
	}
	offer, err := d.Store.GetPlanOfferByPlanCluster(ctx, tid, planID, *cust.ClusterID)
	if errors.Is(err, store.ErrNotFound) || (err == nil && !offer.IsActive) {
		return httpx.BadRequest("paket belum ditawarkan di cluster pelanggan")
	}
	if err != nil {
		return httpx.Internal(err)
	}
	return nil
}

func applyPlanChangeToRouter(ctx context.Context, d *Deps, tid xid.ID, sub *store.Subscription, cust *store.Customer, newPlanID xid.ID) (*billing.PlanChangeQuote, *store.Invoice, *store.Subscription, error) {
	oldUsername := sub.Username
	oldRouterID := sub.RouterID
	q, inv, err := d.Billing.ApplyPlanChange(ctx, tid, sub.ID, newPlanID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, nil, nil, httpx.NotFound(err.Error())
		}
		return nil, nil, nil, httpx.BadRequest(err.Error())
	}
	outSub, err := d.Store.GetSubscription(ctx, tid, sub.ID)
	if err != nil {
		return nil, nil, nil, httpx.Internal(err)
	}
	newPlan, err := d.Store.GetPlan(ctx, tid, newPlanID)
	if err != nil {
		return nil, nil, nil, httpx.Internal(err)
	}
	outSub.CustomerName = cust.FullName
	outSub.CustomerCode = cust.CustomerCode
	outSub.PlanName = newPlan.Name
	if err := syncSubscriptionToRouter(ctx, d, outSub, newPlan, oldUsername, oldRouterID); err != nil {
		slog.Warn("change-plan: sync router failed", "err", err, "subscription_id", sub.ID)
	}
	return q, inv, outSub, nil
}

func collectPortalInvoices(ctx context.Context, d *Deps, tenantID xid.ID, byID map[xid.ID]*store.Customer) []store.Invoice {
	var list []store.Invoice
	for _, c := range byID {
		rows, err := d.Store.ListCustomerInvoices(ctx, tenantID, c.ID, 100)
		if err != nil {
			continue
		}
		for _, inv := range rows {
			inv.CustomerCode = c.CustomerCode
			list = append(list, inv)
		}
	}
	if list == nil {
		list = []store.Invoice{}
	}
	// Lengkapi nama item yang ditagih agar portal bisa menampilkan "Paket X, Denda, ...".
	if len(list) > 0 {
		ids := make([]xid.ID, 0, len(list))
		for _, inv := range list {
			ids = append(ids, inv.ID)
		}
		if itemsMap, err := d.Store.InvoiceItemsMap(ctx, tenantID, ids); err == nil {
			for i := range list {
				items := itemsMap[list[i].ID]
				list[i].Items = items
				list[i].ItemsSummary = store.SummarizeInvoiceItems(items)
			}
		}
	}
	attachInvoiceAdminFees(ctx, d, tenantID, list)
	// Terbaru di atas: urutkan berdasarkan waktu terbit (fallback jatuh tempo
	// bila issued_at kosong).
	sort.Slice(list, func(i, j int) bool {
		ti := list[i].DueDate
		if list[i].IssuedAt != nil {
			ti = *list[i].IssuedAt
		}
		tj := list[j].DueDate
		if list[j].IssuedAt != nil {
			tj = *list[j].IssuedAt
		}
		if ti.Equal(tj) {
			return list[i].InvoiceNumber > list[j].InvoiceNumber
		}
		return ti.After(tj)
	})
	return list
}

func collectPortalPayments(ctx context.Context, d *Deps, tenantID xid.ID, custs []*store.Customer) []store.Payment {
	var payments []store.Payment
	for _, c := range custs {
		plist, _ := d.Store.ListCustomerPayments(ctx, tenantID, c.ID, 20)
		for i := range plist {
			plist[i].CustomerName = c.FullName
			plist[i].CustomerCode = c.CustomerCode
			plist[i].Method = payment.NormalizeMethod(plist[i].Method)
			payments = append(payments, plist[i])
		}
		intents, _ := d.Store.ListCustomerOpenPaymentIntents(ctx, tenantID, c.ID, 20)
		for i := range intents {
			pi := intents[i]
			row := store.Payment{
				ID:           pi.ID,
				TenantID:     pi.TenantID,
				CustomerID:   pi.CustomerID,
				InvoiceID:    pi.InvoiceID,
				Amount:       pi.Amount,
				Method:       payment.MethodFromProvider(pi.Provider),
				Status:       pi.Status,
				CreatedAt:    pi.CreatedAt,
				CustomerName: c.FullName,
				CustomerCode: c.CustomerCode,
			}
			if pi.InvoiceID != nil {
				row.InvoiceNumber = d.Store.InvoiceNumberByID(ctx, tenantID, *pi.InvoiceID)
			}
			payments = append(payments, row)
		}
	}
	if payments == nil {
		payments = []store.Payment{}
	}
	// Lengkapi nama item dari invoice terkait agar riwayat pembayaran
	// menampilkan apa yang dibayar, bukan cuma nomor invoice.
	if len(payments) > 0 {
		ids := make([]xid.ID, 0, len(payments))
		for _, p := range payments {
			if p.InvoiceID != nil && !xid.IsNil(*p.InvoiceID) {
				ids = append(ids, *p.InvoiceID)
			}
		}
		if itemsMap, err := d.Store.InvoiceItemsMap(ctx, tenantID, ids); err == nil {
			for i := range payments {
				if payments[i].InvoiceID == nil || xid.IsNil(*payments[i].InvoiceID) {
					continue
				}
				items := itemsMap[*payments[i].InvoiceID]
				payments[i].Items = items
				payments[i].ItemsSummary = store.SummarizeInvoiceItems(items)
			}
		}
	}
	sort.Slice(payments, func(i, j int) bool {
		ti := payments[i].PaidAt
		if ti == nil {
			ti = &payments[i].CreatedAt
		}
		tj := payments[j].PaidAt
		if tj == nil {
			tj = &payments[j].CreatedAt
		}
		return ti.After(*tj)
	})
	return payments
}

func registerPortal(api huma.API, d *Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "portal-login", Method: http.MethodPost, Path: "/api/portal/login",
		Tags: []string{"Portal"},
	}, func(ctx context.Context, input *struct {
		Body struct {
			Phone      string `json:"phone"`
			Password   string `json:"password"`
			TenantID   xid.ID `json:"tenant_id,omitempty"`
			TenantSlug string `json:"tenant_slug,omitempty"`
		}
	}) (*struct {
		Body struct {
			Customer      store.Customer       `json:"customer"`
			Customers     []store.Customer     `json:"customers"`
			Subscriptions []store.Subscription `json:"subscriptions"`
			Invoices      []store.Invoice      `json:"invoices"`
			Payments      []store.Payment      `json:"payments"`
			WalletBalance int64                `json:"wallet_balance"`
			TenantSlug    string               `json:"tenant_slug"`
			TenantName    string               `json:"tenant_name"`
			PortalToken   string               `json:"portal_token"`
		}
	}, error) {
		ten, custs, err := authenticatePortalCustomers(ctx, d, input.Body.TenantSlug, input.Body.TenantID, input.Body.Phone, input.Body.Password)
		if err != nil {
			return nil, err
		}
		byID := map[xid.ID]*store.Customer{}
		for _, c := range custs {
			byID[c.ID] = c
		}
		customers := make([]store.Customer, 0, len(custs))
		for _, c := range custs {
			customers = append(customers, *c)
		}
		var subs []store.Subscription
		for _, c := range custs {
			cid := c.ID
			list, _, _ := d.Store.ListSubscriptions(ctx, ten.ID, "", &cid, 200, 0)
			subs = append(subs, list...)
		}
		if subs == nil {
			subs = []store.Subscription{}
		}
		custInvoices := collectPortalInvoices(ctx, d, ten.ID, byID)
		payments := collectPortalPayments(ctx, d, ten.ID, custs)
		var balance int64
		for _, c := range custs {
			if b, berr := d.Store.GetWallet(ctx, ten.ID, c.ID); berr == nil {
				balance += b
			}
		}
		out := &struct {
			Body struct {
				Customer      store.Customer       `json:"customer"`
				Customers     []store.Customer     `json:"customers"`
				Subscriptions []store.Subscription `json:"subscriptions"`
				Invoices      []store.Invoice      `json:"invoices"`
				Payments      []store.Payment      `json:"payments"`
				WalletBalance int64                `json:"wallet_balance"`
				TenantSlug    string               `json:"tenant_slug"`
				TenantName    string               `json:"tenant_name"`
				PortalToken   string               `json:"portal_token"`
			}
		}{}
		out.Body.Customer = customers[0]
		out.Body.Customers = customers
		out.Body.Subscriptions = subs
		out.Body.Invoices = custInvoices
		out.Body.Payments = payments
		out.Body.WalletBalance = balance
		out.Body.TenantSlug = ten.Slug
		out.Body.TenantName = ten.Name
		if tok, terr := d.Tokens.CreatePortalToken(ten.ID, customers[0].Phone); terr == nil {
			out.Body.PortalToken = tok
		}
		portalCtx := tenant.WithInfo(ctx, tenant.Info{ID: ten.ID, Role: "portal", UserID: customers[0].ID})
		auditEvent(portalCtx, d, AuditPortalLogin, "customer", &customers[0].ID, map[string]any{"phone": customers[0].Phone})
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "portal-change-password", Method: http.MethodPost, Path: "/api/portal/change-password",
		Summary: "Change portal password", Tags: []string{"Portal"},
	}, func(ctx context.Context, input *struct {
		Body struct {
			TenantSlug      string  `json:"tenant_slug"`
			Phone           string  `json:"phone"`
			CurrentPassword string  `json:"current_password"`
			NewPassword     string  `json:"new_password"`
			CustomerID      *xid.ID `json:"customer_id,omitempty"`
		}
	}) (*struct{ Body map[string]string }, error) {
		newPW := strings.TrimSpace(input.Body.NewPassword)
		if len(newPW) < 6 {
			return nil, httpx.BadRequest("password baru minimal 6 karakter")
		}
		ten, custs, err := authenticatePortalCustomers(ctx, d, input.Body.TenantSlug, xid.Nil(), input.Body.Phone, input.Body.CurrentPassword)
		if err != nil {
			return nil, err
		}
		target := custs[0]
		if input.Body.CustomerID != nil {
			found := false
			for _, c := range custs {
				if c.ID == *input.Body.CustomerID {
					target = c
					found = true
					break
				}
			}
			if !found {
				return nil, httpx.BadRequest("akun tidak ditemukan")
			}
		} else if len(custs) > 1 {
			return nil, httpx.BadRequest("pilih akun yang passwordnya diubah")
		}
		if newPW == strings.TrimSpace(input.Body.CurrentPassword) {
			return nil, httpx.BadRequest("password baru harus berbeda dari password lama")
		}
		hash, err := auth.HashPassword(newPW)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		if err := d.Store.UpdatePortalUser(ctx, ten.ID, target.ID, true, &hash); err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body map[string]string }{Body: map[string]string{"status": "ok"}}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "portal-pay-options", Method: http.MethodGet, Path: "/api/portal/pay-options",
		Tags: []string{"Portal"},
	}, func(ctx context.Context, input *struct {
		Authorization string `header:"Authorization"`
	}) (*struct{ Body []payOptionView }, error) {
		ten, _, err := authenticatePortalRequest(ctx, d, input.Authorization, "", "", "")
		if err != nil {
			return nil, err
		}
		return &struct{ Body []payOptionView }{Body: listEnabledPayOptions(ctx, d, ten.ID)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "portal-invoice-checkout", Method: http.MethodPost, Path: "/api/portal/invoices/{id}/checkout",
		Tags: []string{"Portal"},
	}, func(ctx context.Context, input *struct {
		ID              xid.ID `path:"id"`
		Authorization   string `header:"Authorization"`
		Host            string `header:"Host"`
		Origin          string `header:"Origin"`
		Referer         string `header:"Referer"`
		XForwardedHost  string `header:"X-Forwarded-Host"`
		XForwardedProto string `header:"X-Forwarded-Proto"`
		Body            *struct {
			Provider  string `json:"provider,omitempty"`
			Channel   string `json:"channel,omitempty"`
			ReturnURL string `json:"return_url,omitempty"`
		}
	}) (*struct{ Body store.PaymentIntent }, error) {
		ten, custs, err := authenticatePortalRequest(ctx, d, input.Authorization, "", "", "")
		if err != nil {
			return nil, err
		}
		inv, _, err := d.Store.GetInvoice(ctx, ten.ID, input.ID)
		if err != nil {
			return nil, httpx.NotFound("invoice not found")
		}
		allowed := false
		for _, c := range custs {
			if c.ID == inv.CustomerID {
				allowed = true
				break
			}
		}
		if !allowed {
			return nil, httpx.NotFound("invoice not found")
		}
		providerName := ""
		returnURL := ""
		if input.Body != nil {
			providerName = strings.TrimSpace(input.Body.Provider)
			returnURL = strings.TrimSpace(input.Body.ReturnURL)
		}
		if providerName == "" {
			opts := listEnabledPayOptions(ctx, d, ten.ID)
			if len(opts) == 0 {
				return nil, httpx.BadRequest("belum ada metode pembayaran online yang aktif")
			}
			providerName = opts[0].Provider
		}
		origin := appPublicOrigin(ctx, d, ten.ID, input.Origin, input.Referer, input.XForwardedProto, input.XForwardedHost, input.Host)
		if returnURL == "" {
			returnURL = origin
		}
		pi, err := checkoutInvoice(ctx, d, ten.ID, inv, providerName, "", returnURL, origin)
		if err != nil {
			return nil, err
		}
		return &struct{ Body store.PaymentIntent }{Body: *pi}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "portal-invoice-sandbox-pay", Method: http.MethodPost, Path: "/api/portal/invoices/{id}/sandbox-pay",
		Tags: []string{"Portal"},
	}, func(ctx context.Context, input *struct {
		ID            xid.ID `path:"id"`
		Authorization string `header:"Authorization"`
	}) (*struct{ Body store.PaymentIntent }, error) {
		ten, custs, err := authenticatePortalRequest(ctx, d, input.Authorization, "", "", "")
		if err != nil {
			return nil, err
		}
		inv, _, err := d.Store.GetInvoice(ctx, ten.ID, input.ID)
		if err != nil {
			return nil, httpx.NotFound("invoice not found")
		}
		allowed := false
		for _, c := range custs {
			if c.ID == inv.CustomerID {
				allowed = true
				break
			}
		}
		if !allowed {
			return nil, httpx.NotFound("invoice not found")
		}
		pi, err := simulateSandboxInvoicePayment(ctx, d, ten.ID, inv)
		if err != nil {
			return nil, err
		}
		return &struct{ Body store.PaymentIntent }{Body: *pi}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "portal-invoice-payment-intent", Method: http.MethodGet, Path: "/api/portal/invoices/{id}/payment-intent",
		Tags: []string{"Portal"},
	}, func(ctx context.Context, input *struct {
		ID            xid.ID `path:"id"`
		Authorization string `header:"Authorization"`
	}) (*struct{ Body store.PaymentIntent }, error) {
		ten, custs, err := authenticatePortalRequest(ctx, d, input.Authorization, "", "", "")
		if err != nil {
			return nil, err
		}
		inv, _, err := d.Store.GetInvoice(ctx, ten.ID, input.ID)
		if err != nil {
			return nil, httpx.NotFound("invoice not found")
		}
		allowed := false
		for _, c := range custs {
			if c.ID == inv.CustomerID {
				allowed = true
				break
			}
		}
		if !allowed {
			return nil, httpx.NotFound("invoice not found")
		}
		pi, err := latestInvoicePaymentIntent(ctx, d, ten.ID, inv)
		if err != nil {
			return nil, err
		}
		return &struct{ Body store.PaymentIntent }{Body: *pi}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "portal-invoice-payment-intent-cancel", Method: http.MethodPost, Path: "/api/portal/invoices/{id}/payment-intent/cancel",
		Tags: []string{"Portal"},
	}, func(ctx context.Context, input *struct {
		ID            xid.ID `path:"id"`
		Authorization string `header:"Authorization"`
	}) (*struct{ Body store.PaymentIntent }, error) {
		ten, custs, err := authenticatePortalRequest(ctx, d, input.Authorization, "", "", "")
		if err != nil {
			return nil, err
		}
		inv, _, err := d.Store.GetInvoice(ctx, ten.ID, input.ID)
		if err != nil {
			return nil, httpx.NotFound("invoice not found")
		}
		allowed := false
		for _, c := range custs {
			if c.ID == inv.CustomerID {
				allowed = true
				break
			}
		}
		if !allowed {
			return nil, httpx.NotFound("invoice not found")
		}
		pi, err := cancelInvoicePaymentIntent(ctx, d, ten.ID, inv)
		if err != nil {
			return nil, err
		}
		return &struct{ Body store.PaymentIntent }{Body: *pi}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "portal-invoice-pdf", Method: http.MethodGet, Path: "/api/portal/invoices/{id}/pdf",
		Tags: []string{"Portal"},
	}, func(ctx context.Context, input *struct {
		ID            xid.ID `path:"id"`
		Authorization string `header:"Authorization"`
	}) (*struct {
		ContentType        string `header:"Content-Type"`
		ContentDisposition string `header:"Content-Disposition"`
		Body               []byte
	}, error) {
		ten, custs, err := authenticatePortalRequest(ctx, d, input.Authorization, "", "", "")
		if err != nil {
			return nil, err
		}
		inv, items, err := d.Store.GetInvoice(ctx, ten.ID, input.ID)
		if err != nil {
			return nil, httpx.NotFound("invoice not found")
		}
		allowed := false
		for _, c := range custs {
			if c.ID == inv.CustomerID {
				allowed = true
				break
			}
		}
		if !allowed {
			return nil, httpx.NotFound("invoice not found")
		}
		return &struct {
			ContentType        string `header:"Content-Type"`
			ContentDisposition string `header:"Content-Disposition"`
			Body               []byte
		}{
			ContentType:        "application/pdf",
			ContentDisposition: fmt.Sprintf(`attachment; filename="%s.pdf"`, inv.InvoiceNumber),
			Body:               renderInvoicePDF(ctx, d, ten.ID, inv, items),
		}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "portal-list-tickets", Method: http.MethodGet, Path: "/api/portal/tickets",
		Tags: []string{"Portal"},
	}, func(ctx context.Context, input *struct {
		Authorization string `header:"Authorization"`
	}) (*struct {
		Body struct {
			Data []store.Ticket `json:"data"`
		}
	}, error) {
		ten, custs, err := authenticatePortalRequest(ctx, d, input.Authorization, "", "", "")
		if err != nil {
			return nil, err
		}
		ids := make([]xid.ID, 0, len(custs))
		for _, c := range custs {
			ids = append(ids, c.ID)
		}
		list, err := d.Store.ListTicketsByCustomers(ctx, ten.ID, ids, 100)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		if list == nil {
			list = []store.Ticket{}
		}
		return &struct {
			Body struct {
				Data []store.Ticket `json:"data"`
			}
		}{Body: struct {
			Data []store.Ticket `json:"data"`
		}{Data: list}}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "portal-create-ticket", Method: http.MethodPost, Path: "/api/portal/tickets",
		Tags: []string{"Portal"},
	}, func(ctx context.Context, input *struct {
		Authorization string `header:"Authorization"`
		Body          struct {
			CustomerID  *xid.ID `json:"customer_id,omitempty"`
			Subject     string  `json:"subject"`
			Description string  `json:"description"`
			Category    string  `json:"category"`
			Priority    string  `json:"priority"`
		}
	}) (*struct{ Body store.Ticket }, error) {
		ten, custs, err := authenticatePortalRequest(ctx, d, input.Authorization, "", "", "")
		if err != nil {
			return nil, err
		}
		target := custs[0]
		if input.Body.CustomerID != nil {
			found := false
			for _, c := range custs {
				if c.ID == *input.Body.CustomerID {
					target = c
					found = true
					break
				}
			}
			if !found {
				return nil, httpx.BadRequest("akun tidak ditemukan")
			}
		} else if len(custs) > 1 {
			return nil, httpx.BadRequest("pilih akun untuk keluhan ini")
		}
		subject := strings.TrimSpace(input.Body.Subject)
		if subject == "" {
			return nil, httpx.BadRequest("subjek wajib")
		}
		desc := strings.TrimSpace(input.Body.Description)
		if desc == "" {
			return nil, httpx.BadRequest("deskripsi wajib")
		}
		cat := strings.TrimSpace(strings.ToLower(input.Body.Category))
		if cat == "" {
			cat = "general"
		}
		pri := strings.TrimSpace(strings.ToLower(input.Body.Priority))
		if pri == "" {
			pri = "normal"
		}
		cid := target.ID
		t := &store.Ticket{
			TenantID: ten.ID, CustomerID: &cid, Subject: subject, Description: &desc,
			Category: cat, Priority: pri, Status: "open",
		}
		if err := d.Store.CreateTicket(ctx, t); err != nil {
			return nil, httpx.BadRequest(err.Error())
		}
		_, _ = d.Store.AddTicketMessage(ctx, ten.ID, t.ID, "customer", &cid, desc, nil)
		full, _ := d.Store.GetTicket(ctx, ten.ID, t.ID)
		who := strings.TrimSpace(target.FullName)
		if who == "" {
			who = target.CustomerCode
		}
		subj := t.Subject
		if full != nil {
			subj = full.Subject
		}
		_ = d.Notify.QueueTicketTelegram(ctx, ten.ID, &cid, subj,
			"Dari portal pelanggan",
			"Prioritas: "+t.Priority,
			"Kategori: "+t.Category,
			"Pelanggan: "+who,
		)
		queueTicketAlert(ctx, d, ten.ID, t)
		if full != nil {
			return &struct{ Body store.Ticket }{Body: *full}, nil
		}
		return &struct{ Body store.Ticket }{Body: *t}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "portal-list-ticket-messages", Method: http.MethodGet, Path: "/api/portal/tickets/{id}/messages",
		Tags: []string{"Portal"},
	}, func(ctx context.Context, input *struct {
		ID            xid.ID `path:"id"`
		Authorization string `header:"Authorization"`
	}) (*struct{ Body []store.TicketMessage }, error) {
		ten, _, t, err := portalTicketForSession(ctx, d, input.Authorization, input.ID)
		if err != nil {
			return nil, err
		}
		list, err := d.Store.ListTicketMessages(ctx, ten.ID, t.ID)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		if list == nil {
			list = []store.TicketMessage{}
		}
		return &struct{ Body []store.TicketMessage }{Body: list}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "portal-add-ticket-message", Method: http.MethodPost, Path: "/api/portal/tickets/{id}/messages",
		Tags: []string{"Portal"},
	}, func(ctx context.Context, input *struct {
		ID            xid.ID `path:"id"`
		Authorization string `header:"Authorization"`
		Body          struct {
			Message string `json:"message"`
		}
	}) (*struct{ Body store.TicketMessage }, error) {
		ten, cust, t, err := portalTicketForSession(ctx, d, input.Authorization, input.ID)
		if err != nil {
			return nil, err
		}
		if !store.TicketAllowsCustomerReply(t.Status) {
			return nil, httpx.BadRequest("tiket ini sudah ditutup")
		}
		body := strings.TrimSpace(input.Body.Message)
		if body == "" {
			return nil, httpx.BadRequest("pesan wajib")
		}
		cid := cust.ID
		m, err := d.Store.AddTicketMessage(ctx, ten.ID, t.ID, "customer", &cid, body, nil)
		if err != nil {
			return nil, httpx.BadRequest(err.Error())
		}
		who := strings.TrimSpace(cust.FullName)
		if who == "" {
			who = cust.CustomerCode
		}
		preview := body
		if r := []rune(preview); len(r) > 200 {
			preview = string(r[:200]) + "…"
		}
		_ = d.Notify.QueueTicketTelegram(ctx, ten.ID, &cid, t.Subject,
			"Balasan pelanggan",
			"Pelanggan: "+who,
			preview,
		)
		return &struct{ Body store.TicketMessage }{Body: *m}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "portal-list-subscriptions", Method: http.MethodGet, Path: "/api/portal/subscriptions",
		Tags: []string{"Portal"},
	}, func(ctx context.Context, input *struct {
		Authorization string `header:"Authorization"`
	}) (*struct {
		Body struct {
			Data []store.Subscription `json:"data"`
		}
	}, error) {
		ten, custs, err := authenticatePortalRequest(ctx, d, input.Authorization, "", "", "")
		if err != nil {
			return nil, err
		}
		var subs []store.Subscription
		for _, c := range custs {
			cid := c.ID
			list, _, _ := d.Store.ListSubscriptions(ctx, ten.ID, "", &cid, 200, 0)
			subs = append(subs, list...)
		}
		if subs == nil {
			subs = []store.Subscription{}
		}
		return &struct {
			Body struct {
				Data []store.Subscription `json:"data"`
			}
		}{Body: struct {
			Data []store.Subscription `json:"data"`
		}{Data: subs}}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "portal-list-invoices", Method: http.MethodGet, Path: "/api/portal/invoices",
		Tags: []string{"Portal"},
	}, func(ctx context.Context, input *struct {
		Authorization string `header:"Authorization"`
	}) (*struct {
		Body struct {
			Data []store.Invoice `json:"data"`
		}
	}, error) {
		ten, custs, err := authenticatePortalRequest(ctx, d, input.Authorization, "", "", "")
		if err != nil {
			return nil, err
		}
		byID := map[xid.ID]*store.Customer{}
		for _, c := range custs {
			byID[c.ID] = c
		}
		list := collectPortalInvoices(ctx, d, ten.ID, byID)
		return &struct {
			Body struct {
				Data []store.Invoice `json:"data"`
			}
		}{Body: struct {
			Data []store.Invoice `json:"data"`
		}{Data: list}}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "portal-list-payments", Method: http.MethodGet, Path: "/api/portal/payments",
		Tags: []string{"Portal"},
	}, func(ctx context.Context, input *struct {
		Authorization string `header:"Authorization"`
	}) (*struct {
		Body struct {
			Data []store.Payment `json:"data"`
		}
	}, error) {
		ten, custs, err := authenticatePortalRequest(ctx, d, input.Authorization, "", "", "")
		if err != nil {
			return nil, err
		}
		list := collectPortalPayments(ctx, d, ten.ID, custs)
		return &struct {
			Body struct {
				Data []store.Payment `json:"data"`
			}
		}{Body: struct {
			Data []store.Payment `json:"data"`
		}{Data: list}}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "portal-list-plans", Method: http.MethodGet, Path: "/api/portal/plans",
		Tags:    []string{"Portal"},
		Summary: "Catalog of portal-visible plans for the logged-in customer",
	}, func(ctx context.Context, input *struct {
		Authorization string `header:"Authorization"`
	}) (*struct {
		Body struct {
			Data []store.PortalPlanOption `json:"data"`
		}
	}, error) {
		ten, custs, err := authenticatePortalRequest(ctx, d, input.Authorization, "", "", "")
		if err != nil {
			return nil, err
		}
		byID := map[xid.ID]store.PortalPlanOption{}
		for _, c := range custs {
			opts, err := d.Store.ListPortalPlans(ctx, ten.ID, c.ClusterID, "", xid.Nil())
			if err != nil {
				return nil, httpx.Internal(err)
			}
			for i := range opts {
				p := opts[i]
				d.Store.DecoratePortalPlanPrice(ctx, ten.ID, c.ID, &p)
				if prev, ok := byID[p.ID]; ok && p.Price >= prev.Price {
					continue
				}
				byID[p.ID] = p
			}
		}
		list := make([]store.PortalPlanOption, 0, len(byID))
		for _, p := range byID {
			list = append(list, p)
		}
		sort.Slice(list, func(i, j int) bool {
			if list[i].Price != list[j].Price {
				return list[i].Price < list[j].Price
			}
			return list[i].Name < list[j].Name
		})
		return &struct {
			Body struct {
				Data []store.PortalPlanOption `json:"data"`
			}
		}{Body: struct {
			Data []store.PortalPlanOption `json:"data"`
		}{Data: list}}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "portal-list-subscription-plans", Method: http.MethodGet, Path: "/api/portal/subscriptions/{id}/plans",
		Tags: []string{"Portal"},
	}, func(ctx context.Context, input *struct {
		ID            xid.ID `path:"id"`
		Authorization string `header:"Authorization"`
	}) (*struct {
		Body struct {
			Data []store.PortalPlanOption `json:"data"`
		}
	}, error) {
		ten, cust, sub, err := portalSubscriptionForSession(ctx, d, input.Authorization, input.ID)
		if err != nil {
			return nil, err
		}
		list, err := d.Store.ListPortalPlans(ctx, ten.ID, cust.ClusterID, sub.ServiceType, sub.PlanID)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		for i := range list {
			d.Store.DecoratePortalPlanPrice(ctx, ten.ID, cust.ID, &list[i])
		}
		return &struct {
			Body struct {
				Data []store.PortalPlanOption `json:"data"`
			}
		}{Body: struct {
			Data []store.PortalPlanOption `json:"data"`
		}{Data: list}}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "portal-preview-change-plan", Method: http.MethodPost, Path: "/api/portal/subscriptions/{id}/change-plan/preview",
		Tags: []string{"Portal"},
	}, func(ctx context.Context, input *struct {
		ID            xid.ID `path:"id"`
		Authorization string `header:"Authorization"`
		Body          struct {
			PlanID xid.ID `json:"plan_id"`
		}
	}) (*struct{ Body billing.PlanChangeQuote }, error) {
		ten, cust, sub, err := portalSubscriptionForSession(ctx, d, input.Authorization, input.ID)
		if err != nil {
			return nil, err
		}
		if xid.IsNil(input.Body.PlanID) {
			return nil, httpx.BadRequest("plan_id wajib")
		}
		if err := ensurePortalPlanChoice(ctx, d, ten.ID, cust, sub, input.Body.PlanID); err != nil {
			return nil, err
		}
		q, _, _, _, err := d.Billing.QuotePlanChange(ctx, ten.ID, sub.ID, input.Body.PlanID, time.Now())
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return nil, httpx.NotFound(err.Error())
			}
			return nil, httpx.BadRequest(err.Error())
		}
		return &struct{ Body billing.PlanChangeQuote }{Body: *q}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "portal-change-plan", Method: http.MethodPost, Path: "/api/portal/subscriptions/{id}/change-plan",
		Tags: []string{"Portal"},
	}, func(ctx context.Context, input *struct {
		ID            xid.ID `path:"id"`
		Authorization string `header:"Authorization"`
		Body          struct {
			PlanID xid.ID `json:"plan_id"`
		}
	}) (*struct {
		Body struct {
			Quote   billing.PlanChangeQuote `json:"quote"`
			Invoice *store.Invoice          `json:"invoice,omitempty"`
			Status  string                  `json:"status"`
		}
	}, error) {
		ten, cust, sub, err := portalSubscriptionForSession(ctx, d, input.Authorization, input.ID)
		if err != nil {
			return nil, err
		}
		if xid.IsNil(input.Body.PlanID) {
			return nil, httpx.BadRequest("plan_id wajib")
		}
		if err := ensurePortalPlanChoice(ctx, d, ten.ID, cust, sub, input.Body.PlanID); err != nil {
			return nil, err
		}
		q, inv, outSub, err := applyPlanChangeToRouter(ctx, d, ten.ID, sub, cust, input.Body.PlanID)
		if err != nil {
			return nil, err
		}
		who := strings.TrimSpace(cust.FullName)
		if who == "" {
			who = cust.CustomerCode
		}
		lines := []string{
			"Dari portal pelanggan",
			q.OldPlanName + " → " + q.NewPlanName,
			"Arah: " + q.Direction,
			"User: " + outSub.Username,
		}
		if q.Direction == "downgrade" {
			lines = append(lines, "Sisa tagihan tidak di-refund")
		}
		if q.RequiresCharge {
			lines = append(lines, fmt.Sprintf("Tagihan sekarang: %d", q.TotalAmount))
		}
		cluster, router, _ := d.Store.CustomerNetworkLabels(ctx, ten.ID, cust.ID)
		lines = append(lines, notify.OpsNetworkLines(cluster, router)...)
		if d.Notify != nil {
			_ = d.Notify.QueueTenantTelegram(ctx, ten.ID, notify.OpsMsg("paket", who, lines...))
		}
		return &struct {
			Body struct {
				Quote   billing.PlanChangeQuote `json:"quote"`
				Invoice *store.Invoice          `json:"invoice,omitempty"`
				Status  string                  `json:"status"`
			}
		}{Body: struct {
			Quote   billing.PlanChangeQuote `json:"quote"`
			Invoice *store.Invoice          `json:"invoice,omitempty"`
			Status  string                  `json:"status"`
		}{Quote: *q, Invoice: inv, Status: outSub.Status}}, nil
	})
}

func portalTicketForSession(ctx context.Context, d *Deps, authorization string, ticketID xid.ID) (*store.Tenant, *store.Customer, *store.Ticket, error) {
	ten, custs, err := authenticatePortalRequest(ctx, d, authorization, "", "", "")
	if err != nil {
		return nil, nil, nil, err
	}
	t, err := d.Store.GetTicket(ctx, ten.ID, ticketID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil, nil, httpx.NotFound("tiket tidak ditemukan")
	}
	if err != nil {
		return nil, nil, nil, httpx.Internal(err)
	}
	if t.CustomerID == nil {
		return nil, nil, nil, httpx.NotFound("tiket tidak ditemukan")
	}
	for _, c := range custs {
		if c != nil && c.ID == *t.CustomerID {
			return ten, c, t, nil
		}
	}
	return nil, nil, nil, httpx.NotFound("tiket tidak ditemukan")
}

func portalSubscriptionForSession(ctx context.Context, d *Deps, authorization string, subscriptionID xid.ID) (*store.Tenant, *store.Customer, *store.Subscription, error) {
	ten, custs, err := authenticatePortalRequest(ctx, d, authorization, "", "", "")
	if err != nil {
		return nil, nil, nil, err
	}
	sub, err := d.Store.GetSubscription(ctx, ten.ID, subscriptionID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil, nil, httpx.NotFound("langganan tidak ditemukan")
	}
	if err != nil {
		return nil, nil, nil, httpx.Internal(err)
	}
	for _, c := range custs {
		if c != nil && c.ID == sub.CustomerID {
			return ten, c, sub, nil
		}
	}
	return nil, nil, nil, httpx.NotFound("langganan tidak ditemukan")
}

func ensurePortalPlanChoice(ctx context.Context, d *Deps, tid xid.ID, cust *store.Customer, sub *store.Subscription, newPlanID xid.ID) error {
	opts, err := d.Store.ListPortalPlans(ctx, tid, cust.ClusterID, sub.ServiceType, sub.PlanID)
	if err != nil {
		return httpx.Internal(err)
	}
	for _, o := range opts {
		if o.ID == newPlanID {
			return nil
		}
	}
	return httpx.BadRequest("paket tidak tersedia di portal")
}

// authenticatePortalCustomers resolves tenant + ALL customers sharing the phone
// number whose portal password verifies (one payer, several installations).
func authenticatePortalCustomers(ctx context.Context, d *Deps, tenantSlug string, tenantID xid.ID, phone, password string) (*store.Tenant, []*store.Customer, error) {
	slug := strings.ToLower(strings.TrimSpace(tenantSlug))
	var ten *store.Tenant
	var err error
	tid := tenantID
	if slug != "" {
		ten, err = d.Store.GetTenantBySlug(ctx, slug)
		if err != nil {
			return nil, nil, httpx.Unauthorized("invalid credentials")
		}
		tid = ten.ID
	} else if !xid.IsNil(tid) {
		ten, err = d.Store.GetTenant(ctx, tid)
		if err != nil {
			return nil, nil, httpx.Unauthorized("invalid credentials")
		}
	} else {
		ten, err = singleTenant(ctx, d)
		if err != nil || ten == nil {
			return nil, nil, httpx.Unauthorized("invalid credentials")
		}
		tid = ten.ID
	}
	if !ten.IsActive {
		return nil, nil, httpx.Unauthorized("tenant inactive")
	}
	phone = strings.TrimSpace(phone)
	pw := strings.TrimSpace(password)
	if phone == "" || pw == "" {
		return nil, nil, httpx.Unauthorized("invalid credentials")
	}
	candidates, err := d.Store.ListCustomersByPhone(ctx, tid, phone)
	if err != nil {
		return nil, nil, httpx.Internal(err)
	}
	var matched []*store.Customer
	for i := range candidates {
		c := &candidates[i]
		if !c.IsActive || !c.PortalEnabled {
			continue
		}
		hash, herr := d.Store.GetCustomerPasswordHash(ctx, tid, c.ID)
		if herr != nil {
			continue
		}
		if hash == "" {
			// Legacy account without stored hash: default password is the phone itself.
			if pw != c.Phone {
				continue
			}
			if h, herr := auth.HashPassword(c.Phone); herr == nil {
				_ = d.Store.UpdatePortalUser(ctx, tid, c.ID, true, &h)
			}
			matched = append(matched, c)
			continue
		}
		ok, verr := auth.VerifyPassword(pw, hash)
		if verr != nil || !ok {
			continue
		}
		matched = append(matched, c)
	}
	if len(matched) == 0 {
		return nil, nil, httpx.Unauthorized("invalid credentials")
	}
	return ten, matched, nil
}

func authenticatePortalRequest(ctx context.Context, d *Deps, authorization, tenantSlug, phone, password string) (*store.Tenant, []*store.Customer, error) {
	token := strings.TrimSpace(authorization)
	if len(token) > 7 && strings.EqualFold(token[:7], "bearer ") {
		token = strings.TrimSpace(token[7:])
	}
	if token != "" && d.Tokens != nil {
		claims, err := d.Tokens.ParseToken(token)
		if err == nil && claims != nil && claims.Type == "portal" {
			tid, perr := xid.Parse(claims.TenantID)
			if perr != nil {
				return nil, nil, httpx.Unauthorized("invalid credentials")
			}
			ten, err := d.Store.GetTenant(ctx, tid)
			if err != nil || ten == nil || !ten.IsActive {
				return nil, nil, httpx.Unauthorized("invalid credentials")
			}
			portalPhone := strings.TrimSpace(claims.Email)
			if portalPhone == "" {
				return nil, nil, httpx.Unauthorized("invalid credentials")
			}
			candidates, err := d.Store.ListCustomersByPhone(ctx, tid, portalPhone)
			if err != nil {
				return nil, nil, httpx.Internal(err)
			}
			var matched []*store.Customer
			for i := range candidates {
				c := &candidates[i]
				if c.IsActive && c.PortalEnabled {
					matched = append(matched, c)
				}
			}
			if len(matched) == 0 {
				return nil, nil, httpx.Unauthorized("invalid credentials")
			}
			return ten, matched, nil
		}
	}
	return authenticatePortalCustomers(ctx, d, tenantSlug, xid.Nil(), phone, password)
}

type paymentWebhookInput struct {
	RawBody            []byte
	ContentType        string `header:"Content-Type"`
	XSignature         string `header:"X-Signature"`
	XCallbackToken     string `header:"X-CALLBACK-TOKEN"`
	XCallbackSignature string `header:"X-Callback-Signature"`
	XEventType         string `header:"X-Event-Type"`
	ClientID           string `header:"Client-Id"`
	RequestID          string `header:"Request-Id"`
	RequestTimestamp   string `header:"Request-Timestamp"`
	Signature          string `header:"Signature"`
	XPartnerID         string `header:"X-PARTNER-ID"`
	XExternalID        string `header:"X-EXTERNAL-ID"`
	XTimestamp         string `header:"X-TIMESTAMP"`
	XSnapSignature     string `header:"X-SIGNATURE"`
	ChannelID          string `header:"CHANNEL-ID"`
	Authorization      string `header:"Authorization"`
	UserAgent          string `header:"User-Agent"`
	Tenant             string `query:"tenant"`
}

func registerPaymentWebhookRoute(api huma.API, d *Deps, provider string) {
	provider = normalizePaymentProviderName(provider)
	path := paymentWebhookPathFor(provider)
	huma.Register(api, huma.Operation{
		OperationID:      "payment-webhook-" + provider,
		Method:           http.MethodPost,
		Path:             path,
		Tags:             []string{"Webhooks"},
		SkipValidateBody: true,
	}, func(ctx context.Context, input *paymentWebhookInput) (*struct{ Body map[string]string }, error) {
		return processPaymentWebhook(ctx, d, provider, input)
	})
}

// webhookAck is a 200 OK body for webhooks we accept but do not act on, so a
// misconfigured/test callback still gets a clear status instead of a 404.
func webhookAck(status, reason string) *struct{ Body map[string]string } {
	body := map[string]string{"status": status}
	if reason != "" {
		body["reason"] = reason
	}
	return &struct{ Body map[string]string }{Body: body}
}

func processPaymentWebhook(ctx context.Context, d *Deps, providerName string, input *paymentWebhookInput) (*struct{ Body map[string]string }, error) {
	started := time.Now()
	providerName = normalizePaymentProviderName(providerName)

	raw := input.RawBody
	bodyMap := payment.ParseWebhookBodyBytes(input.ContentType, raw)
	parsed, parseErr := payment.ParseWebhookEvent(providerName, bodyMap)
	if parsed == nil {
		parsed = &payment.WebhookEvent{Raw: bodyMap, Status: "pending"}
	}
	parseErrStr := ""
	if parseErr != nil {
		parseErrStr = parseErr.Error()
	}
	slog.Info("payment webhook received",
		"provider", providerName,
		"content_type", input.ContentType,
		"bytes", len(raw),
		"external_id", parsed.ExternalID,
		"reference", parsed.Reference,
		"event_status", parsed.Status,
		"amount", parsed.Amount,
		"has_signature", strings.TrimSpace(input.XSignature) != "",
		"has_callback_token", strings.TrimSpace(input.XCallbackToken) != "",
		"event_type", input.XEventType,
		"user_agent", input.UserAgent,
		"parse_error", parseErrStr,
	)

	if providerName == payment.ProviderManual {
		slog.Warn("payment webhook ignored", "provider", providerName, "reason", "manual provider has no webhook")
		return webhookAck("ignored", "manual provider has no webhook"), nil
	}
	if providerName != payment.ProviderDuitku && providerName != payment.ProviderDoku {
		slog.Warn("payment webhook ignored", "provider", providerName, "reason", "unknown provider")
		return webhookAck("received", "unknown provider"), nil
	}

	headers := map[string]string{
		"Content-Type":         input.ContentType,
		"X-Signature":          input.XSignature,
		"X-CALLBACK-TOKEN":     input.XCallbackToken,
		"X-Callback-Signature": input.XCallbackSignature,
		"X-Event-Type":         input.XEventType,
		"Authorization":        input.Authorization,
		"Client-Id":            input.ClientID,
		"Request-Id":           input.RequestID,
		"Request-Timestamp":    input.RequestTimestamp,
		"Signature":            input.Signature,
		"X-PARTNER-ID":         input.XPartnerID,
		"X-EXTERNAL-ID":        input.XExternalID,
		"X-TIMESTAMP":          input.XTimestamp,
		"X-SIGNATURE":          input.XSnapSignature,
		"CHANNEL-ID":           input.ChannelID,
	}

	var prov payment.Provider
	var perr error
	var webhookTenantID *xid.ID
	reason := ""
	if parsed.ExternalID != "" {
		if pi, ierr := d.Store.GetPaymentIntentByExternalID(ctx, parsed.ExternalID); ierr == nil && pi != nil {
			if normalizePaymentProviderName(pi.Provider) != providerName {
				slog.Warn("payment webhook ignored",
					"provider", providerName, "external_id", parsed.ExternalID, "reason", "payment intent provider mismatch")
				return webhookAck("ignored", "payment intent provider mismatch"), nil
			}
			prov, perr = resolvePaymentProvider(ctx, d, pi.TenantID, providerName)
			if prov == nil {
				reason = "provider not configured"
				if perr != nil {
					reason = perr.Error()
				}
			} else {
				tid := pi.TenantID
				webhookTenantID = &tid
			}
		} else {
			slog.Info("payment webhook: no matching payment intent",
				"provider", providerName, "external_id", parsed.ExternalID)
		}
	}
	if prov == nil && strings.TrimSpace(input.Tenant) != "" {
		if t, terr := d.Store.GetTenantBySlug(ctx, strings.TrimSpace(input.Tenant)); terr == nil && t != nil {
			prov, perr = resolvePaymentProvider(ctx, d, t.ID, providerName)
			if prov == nil {
				reason = "provider not configured"
				if perr != nil {
					reason = perr.Error()
				}
			} else {
				tid := t.ID
				webhookTenantID = &tid
			}
		} else {
			reason = "unknown tenant: " + strings.TrimSpace(input.Tenant)
		}
	}
	if prov == nil {
		if reason == "" {
			if parsed.ExternalID == "" {
				reason = "missing external id (test webhook?)"
			} else {
				reason = "no matching payment intent (test webhook?)"
			}
		}
		slog.Warn("payment webhook accepted but not processed",
			"provider", providerName, "external_id", parsed.ExternalID, "tenant", input.Tenant, "reason", reason)
		return webhookAck("received", reason), nil
	}

	var event *payment.WebhookEvent
	verified, verr := prov.VerifyWebhook(ctx, headers, raw)
	if verr != nil && webhookTenantID != nil {
		// Kredensial sandbox & produksi tersimpan bersamaan: callback yang
		// dibuat di env satunya tetap valid walau mode aktif sudah pindah.
		if altProv, altEvent, altErr := verifyPaymentWebhookBothEnvs(ctx, d, *webhookTenantID, providerName, headers, raw); altErr == nil {
			prov = altProv
			verified = altEvent
			verr = nil
		}
	}
	if verr != nil {
		softFail := d.Config != nil && d.Config.AppEnv == "development"
		if softFail {
			slog.Warn("webhook signature soft-fail in development", "provider", providerName, "err", verr)
		} else {
			slog.Warn("payment webhook rejected: invalid signature",
				"provider", providerName, "external_id", parsed.ExternalID, "err", verr)
			return nil, httpx.Unauthorized("invalid webhook signature")
		}
	} else {
		event = verified
	}
	if event == nil {
		event = parsed
	}

	if event.ExternalID != "" {
		if pi, ierr := d.Store.GetPaymentIntentByExternalID(ctx, event.ExternalID); ierr == nil && pi != nil {
			if normalizePaymentProviderName(pi.Provider) != providerName {
				slog.Warn("payment webhook ignored",
					"provider", providerName, "external_id", event.ExternalID, "reason", "payment intent provider mismatch")
				return webhookAck("ignored", "payment intent provider mismatch"), nil
			}
			event.ExternalID = pi.ExternalID
		}
	}

	if payment.WebhookIsPaid(event.Status) && event.ExternalID != "" {
		if err := completePaidWebhook(ctx, d, providerName, event); err != nil {
			slog.Error("complete paid webhook", "external_id", event.ExternalID, "err", err)
			return nil, httpx.Internal(err)
		}
	}

	// Sync the intent status after completion: completePaidWebhook relies on
	// the pre-webhook status for idempotency, so this must run last.
	if event.ExternalID != "" {
		_ = d.Store.UpdatePaymentIntentStatus(ctx, event.ExternalID, event.Status)
	}

	slog.Info("payment webhook processed",
		"provider", providerName,
		"external_id", event.ExternalID,
		"event_status", event.Status,
		"paid", payment.WebhookIsPaid(event.Status),
		"duration_ms", time.Since(started).Milliseconds(),
	)
	return &struct{ Body map[string]string }{Body: map[string]string{"status": "ok"}}, nil
}

func registerWebhooks(api huma.API, d *Deps) {
	registerPaymentWebhookRoute(api, d, payment.ProviderDuitku)
	registerPaymentWebhookRoute(api, d, payment.ProviderDoku)

	huma.Register(api, huma.Operation{
		OperationID: "whatsapp-webhook", Method: http.MethodPost, Path: "/api/webhooks/whatsapp",
		Summary: "GOWA incoming message webhook (customer bot)",
		Tags:    []string{"Webhooks"},
	}, func(ctx context.Context, input *struct {
		Body whatsappWebhookInput
	}) (*struct {
		Body struct {
			Reply string `json:"reply"`
		}
	}, error) {
		empty := &struct {
			Body struct {
				Reply string `json:"reply"`
			}
		}{}
		b := input.Body
		// Bot pushes replies straight from the bot number; the webhook
		// itself always answers empty.
		if strings.TrimSpace(b.Event) != "" {
			in, ok := parseGOWAMessage(&b)
			if !ok {
				return empty, nil
			}
			ten, terr := singleTenant(ctx, d)
			if terr != nil || ten == nil {
				return empty, nil
			}
			handleWhatsAppBotMessage(ctx, d, ten.ID, in)
			return empty, nil
		}
		if strings.TrimSpace(b.Message) == "" {
			return empty, nil
		}
		tid := b.TenantID
		if _, terr := d.Store.GetTenant(ctx, tid); terr != nil {
			fallback, ferr := singleTenant(ctx, d)
			if ferr != nil || fallback == nil {
				return empty, nil
			}
			tid = fallback.ID
		}
		handleWhatsAppBotMessage(ctx, d, tid, waBotIncoming{
			From: b.Phone, ChatID: b.Phone, Body: b.Message,
		})
		return empty, nil
	})
}

func registerVouchers(api huma.API, d *Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "list-voucher-batches", Method: http.MethodGet, Path: "/api/vouchers/batches",
		Tags: []string{"Vouchers"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, _ *struct{}) (*struct{ Body []store.VoucherBatch }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		list, err := d.Store.ListVoucherBatches(ctx, tid)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		if list == nil {
			list = []store.VoucherBatch{}
		}
		return &struct{ Body []store.VoucherBatch }{Body: list}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-voucher-batch", Method: http.MethodGet, Path: "/api/vouchers/batches/{id}",
		Tags: []string{"Vouchers"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID xid.ID `path:"id"`
	}) (*struct{ Body store.VoucherBatch }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		b, err := d.Store.GetVoucherBatch(ctx, tid, input.ID)
		if errors.Is(err, store.ErrNotFound) {
			return nil, httpx.NotFound("batch tidak ditemukan")
		}
		if err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body store.VoucherBatch }{Body: *b}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-batch-vouchers", Method: http.MethodGet, Path: "/api/vouchers/batches/{id}/codes",
		Tags: []string{"Vouchers"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID     xid.ID `path:"id"`
		Status string `query:"status"`
	}) (*struct{ Body []store.Voucher }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		list, err := d.Store.ListVouchersByBatch(ctx, tid, input.ID, input.Status)
		if errors.Is(err, store.ErrNotFound) {
			return nil, httpx.NotFound("batch tidak ditemukan")
		}
		if err != nil {
			return nil, httpx.Internal(err)
		}
		if list == nil {
			list = []store.Voucher{}
		}
		return &struct{ Body []store.Voucher }{Body: list}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "create-voucher-batch", Method: http.MethodPost, Path: "/api/vouchers/batches",
		Tags: []string{"Vouchers"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Body struct {
			Name      string     `json:"name"`
			PlanID    *xid.ID    `json:"plan_id,omitempty"`
			RouterID  *xid.ID    `json:"router_id,omitempty"`
			Price     int64      `json:"price"`
			Quantity  int        `json:"quantity,omitempty"`
			Prefix    string     `json:"prefix,omitempty"`
			ExpiresAt *time.Time `json:"expires_at,omitempty"`
			Codes     []string   `json:"codes,omitempty"`
			SkipSync  bool       `json:"skip_sync,omitempty"`
		}
	}) (*struct{ Body store.VoucherBatch }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		name := strings.TrimSpace(input.Body.Name)
		if name == "" {
			return nil, httpx.BadRequest("nama batch wajib")
		}
		if input.Body.Price < 0 {
			return nil, httpx.BadRequest("harga tidak valid")
		}
		if input.Body.RouterID == nil {
			return nil, httpx.BadRequest("router wajib dipilih untuk sync hotspot")
		}
		if _, err := d.Store.GetRouter(ctx, tid, *input.Body.RouterID); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return nil, httpx.BadRequest("router tidak ditemukan")
			}
			return nil, httpx.Internal(err)
		}
		if input.Body.PlanID != nil {
			plan, err := d.Store.GetPlan(ctx, tid, *input.Body.PlanID)
			if err != nil {
				if errors.Is(err, store.ErrNotFound) {
					return nil, httpx.BadRequest("paket tidak ditemukan")
				}
				return nil, httpx.Internal(err)
			}
			if plan.ServiceType != "" && plan.ServiceType != "hotspot" {
				return nil, httpx.BadRequest("paket harus bertipe hotspot")
			}
		}
		codes := input.Body.Codes
		b := &store.VoucherBatch{
			TenantID: tid, PlanID: input.Body.PlanID, RouterID: input.Body.RouterID, Name: name,
			Price: input.Body.Price, Quantity: input.Body.Quantity, ExpiresAt: input.Body.ExpiresAt,
			SyncStatus: "pending",
		}
		var errCreate error
		if len(codes) == 0 {
			qty := input.Body.Quantity
			if qty <= 0 {
				qty = 10
			}
			if qty > 5000 {
				return nil, httpx.BadRequest("maksimal 5000 kode per batch")
			}
			b.Quantity = qty
			prefix := strings.TrimSpace(input.Body.Prefix)
			if prefix == "" {
				prefix = "VCH"
			}
			errCreate = d.Store.CreateVoucherBatchWithPrefix(ctx, b, prefix)
		} else {
			errCreate = d.Store.CreateVoucherBatch(ctx, b, codes)
		}
		if errCreate != nil {
			return nil, httpx.BadRequest(errCreate.Error())
		}
		out, _ := d.Store.GetVoucherBatch(ctx, tid, b.ID)
		if out == nil {
			out = b
		}
		if !input.Body.SkipSync {
			synced, syncErr := syncVoucherBatchToRouter(ctx, d, out)
			if synced != nil {
				out = synced
			} else if syncErr != nil {
				slog.Warn("voucher batch sync", "batch_id", b.ID, "err", syncErr)
			}
		}
		return &struct{ Body store.VoucherBatch }{Body: *out}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "sync-voucher-batch", Method: http.MethodPost, Path: "/api/vouchers/batches/{id}/sync",
		Tags: []string{"Vouchers"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID xid.ID `path:"id"`
	}) (*struct{ Body store.VoucherBatch }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		b, err := d.Store.GetVoucherBatch(ctx, tid, input.ID)
		if errors.Is(err, store.ErrNotFound) {
			return nil, httpx.NotFound("batch tidak ditemukan")
		}
		if err != nil {
			return nil, httpx.Internal(err)
		}
		if b.RouterID == nil {
			return nil, httpx.BadRequest("batch belum punya router")
		}
		out, err := syncVoucherBatchToRouter(ctx, d, b)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		if out == nil {
			out = b
		}
		return &struct{ Body store.VoucherBatch }{Body: *out}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "delete-voucher-batch", Method: http.MethodDelete, Path: "/api/vouchers/batches/{id}",
		Tags: []string{"Vouchers"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID xid.ID `path:"id"`
	}) (*struct{}, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		b, err := d.Store.GetVoucherBatch(ctx, tid, input.ID)
		if errors.Is(err, store.ErrNotFound) {
			return nil, httpx.NotFound("batch tidak ditemukan")
		}
		if err != nil {
			return nil, httpx.Internal(err)
		}
		removeVoucherBatchFromRouter(ctx, d, b)
		if err := d.Store.DeleteVoucherBatch(ctx, tid, input.ID); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return nil, httpx.NotFound("batch tidak ditemukan")
			}
			return nil, httpx.Internal(err)
		}
		return &struct{}{}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "export-voucher-batch-csv", Method: http.MethodGet, Path: "/api/vouchers/batches/{id}/export.csv",
		Tags: []string{"Vouchers"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID xid.ID `path:"id"`
	}) (*struct {
		ContentType        string `header:"Content-Type"`
		ContentDisposition string `header:"Content-Disposition"`
		Body               string
	}, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		list, err := d.Store.ListVouchersByBatch(ctx, tid, input.ID, "")
		if errors.Is(err, store.ErrNotFound) {
			return nil, httpx.NotFound("batch tidak ditemukan")
		}
		if err != nil {
			return nil, httpx.Internal(err)
		}
		out := "code,status,used_by,used_at,expires_at\n"
		for _, v := range list {
			usedAt := ""
			if v.UsedAt != nil {
				usedAt = v.UsedAt.Format(time.RFC3339)
			}
			exp := ""
			if v.ExpiresAt != nil {
				exp = v.ExpiresAt.Format(time.RFC3339)
			}
			out += fmt.Sprintf("%s,%s,%s,%s,%s\n", v.Code, v.Status, v.UsedName, usedAt, exp)
		}
		return &struct {
			ContentType        string `header:"Content-Type"`
			ContentDisposition string `header:"Content-Disposition"`
			Body               string
		}{
			ContentType:        "text/csv",
			ContentDisposition: fmt.Sprintf(`attachment; filename="vouchers-%s.csv"`, input.ID.String()),
			Body:               out,
		}, nil
	})
}
