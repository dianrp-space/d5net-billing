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
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/dianrp/drp-billing/internal/auth"
	"github.com/dianrp/drp-billing/internal/billing"
	"github.com/dianrp/drp-billing/internal/config"
	"github.com/dianrp/drp-billing/internal/httpx"
	"github.com/dianrp/drp-billing/internal/notify"
	"github.com/dianrp/drp-billing/internal/payment"
	"github.com/dianrp/drp-billing/internal/provision"
	"github.com/dianrp/drp-billing/internal/provisioner"
	"github.com/dianrp/drp-billing/internal/store"
	"github.com/dianrp/drp-billing/internal/tenant"
	"github.com/dianrp/drp-billing/internal/xid"
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
}

func RegisterAll(api huma.API, d *Deps) {
	httpx.RegisterHealth(api)
	registerAuth(api, d)
	registerPublicTenant(api, d)
	registerPlatform(api, d)
	registerPlatformBranding(api, d)
	registerSettings(api, d)
	registerCustomers(api, d)
	registerClusters(api, d)
	registerPlans(api, d)
	registerPlanOffers(api, d)
	registerSubscriptions(api, d)
	registerRouters(api, d)
	registerIPAM(api, d)
	registerInvoices(api, d)
	registerTickets(api, d)
	registerODP(api, d)
	registerCableRoutes(api, d)
	registerDashboard(api, d)
	registerSearch(api, d)
	registerPortal(api, d)
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
		if user.IsPlatformAdmin {
			return nil, httpx.Unauthorized("use /api/auth/platform/login for platform admin")
		}
		tenants, err := d.Store.ListUserTenants(ctx, user.ID)
		if err != nil || len(tenants) == 0 {
			return nil, httpx.Unauthorized("no tenant access")
		}
		slug := strings.ToLower(strings.TrimSpace(body.TenantSlug))
		tid := tenants[0].TenantID
		role := tenants[0].RoleSlug
		matched := false
		if slug != "" {
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
		} else {
			return nil, httpx.BadRequest("tenant_slug is required")
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
		role := "platform"
		if user.IsPlatformAdmin {
			tid = xid.Nil()
		} else {
			tenants, err := d.Store.ListUserTenants(ctx, user.ID)
			if err != nil {
				return nil, httpx.Internal(err)
			}
			if len(tenants) == 0 {
				return nil, httpx.Unauthorized("no tenant access")
			}
			slug := strings.ToLower(strings.TrimSpace(input.Body.TenantSlug))
			matched := false
			if slug != "" {
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
			} else {
				tid = tenants[0].TenantID
				role = tenants[0].RoleSlug
			}
			ten, err := d.Store.GetTenant(ctx, tid)
			if err != nil || !ten.IsActive {
				return nil, httpx.Unauthorized("tenant inactive")
			}
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
		if !user.IsPlatformAdmin {
			tenants, _ := d.Store.ListUserTenants(ctx, user.ID)
			out.Body.Tenants = tenants
		}
		out.SetCookie = http.Cookie{
			Name: "refresh_token", Value: refresh, Path: "/api/auth",
			HttpOnly: true, Secure: d.Config.AppEnv == "production",
			SameSite: http.SameSiteStrictMode, Expires: refreshExp,
		}
		return out, nil
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
		return nil, httpx.BadRequest("public registration disabled; use platform admin or drpctl create-tenant")
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

func registerCustomers(api huma.API, d *Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "list-customers", Method: http.MethodGet, Path: "/api/customers",
		Summary: "List customers", Tags: []string{"Customers"},
		Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Search    string `query:"search"`
		ClusterID string `query:"cluster_id"`
		Limit     int    `query:"limit" minimum:"1" maximum:"100"`
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
		list, total, err := d.Store.ListCustomers(ctx, store.CustomerFilter{
			TenantID: tid, ClusterID: clusterID, Search: input.Search, Limit: input.Limit, Offset: input.Offset,
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
			ClusterID     *xid.ID  `json:"cluster_id,omitempty"`
			CustomerCode  *string  `json:"customer_code,omitempty"`
			FullName      string   `json:"full_name"`
			Email         *string  `json:"email,omitempty"`
			Phone         string   `json:"phone"`
			Address       *string  `json:"address,omitempty"`
			Latitude      *float64 `json:"latitude,omitempty"`
			Longitude     *float64 `json:"longitude,omitempty"`
			IsActive      *bool    `json:"is_active,omitempty"`
			PortalEnabled *bool    `json:"portal_enabled,omitempty"`
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
		portal := true
		if input.Body.PortalEnabled != nil {
			portal = *input.Body.PortalEnabled
		}

		c := &store.Customer{
			TenantID: tid, ClusterID: input.Body.ClusterID, CustomerCode: code,
			FullName: name, Email: input.Body.Email, Phone: phone, Address: input.Body.Address,
			Latitude: input.Body.Latitude, Longitude: input.Body.Longitude,
			IsActive: active, PortalEnabled: portal,
		}
		if err := d.Store.CreateCustomer(ctx, c); err != nil {
			return nil, httpx.Internal(err)
		}
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
		OperationID: "update-customer", Method: http.MethodPut, Path: "/api/customers/{id}",
		Tags: []string{"Customers"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID   xid.ID `path:"id"`
		Body struct {
			ClusterID     *xid.ID  `json:"cluster_id,omitempty"`
			CustomerCode  string   `json:"customer_code"`
			FullName      string   `json:"full_name"`
			Email         *string  `json:"email,omitempty"`
			Phone         string   `json:"phone"`
			Address       *string  `json:"address,omitempty"`
			Latitude      *float64 `json:"latitude,omitempty"`
			Longitude     *float64 `json:"longitude,omitempty"`
			IsActive      bool     `json:"is_active"`
			PortalEnabled bool     `json:"portal_enabled"`
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
		existing.Phone = phone
		existing.Address = input.Body.Address
		existing.Latitude = input.Body.Latitude
		existing.Longitude = input.Body.Longitude
		existing.IsActive = input.Body.IsActive
		existing.PortalEnabled = input.Body.PortalEnabled
		if err := d.Store.UpdateCustomer(ctx, existing); err != nil {
			return nil, httpx.Internal(err)
		}
		full, err := d.Store.GetCustomer(ctx, tid, existing.ID)
		if err != nil {
			return &struct{ Body store.Customer }{Body: *existing}, nil
		}
		return &struct{ Body store.Customer }{Body: *full}, nil
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
		if err := d.Store.DeleteCustomer(ctx, tid, input.ID); errors.Is(err, store.ErrNotFound) {
			return nil, httpx.NotFound("customer not found")
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
		c.Notes = input.Body.Notes
		c.IsActive = input.Body.IsActive
		if err := d.Store.UpdateCluster(ctx, c); err != nil {
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
			Name          string  `json:"name"`
			Code          string  `json:"code"`
			ServiceType   string  `json:"service_type"`
			Price         int64   `json:"price"`
			BillingCycle  string  `json:"billing_cycle,omitempty"`
			DownloadMbps  int     `json:"download_mbps,omitempty"`
			UploadMbps    int     `json:"upload_mbps,omitempty"`
			QuotaGB       *int    `json:"quota_gb,omitempty"`
			ProfileName   *string `json:"profile_name,omitempty"`
			IsolirProfile *string `json:"isolir_profile,omitempty"`
			GraceDays     *int    `json:"grace_days,omitempty"`
			TaxPercent    *float64 `json:"tax_percent,omitempty"`
			IsActive      *bool   `json:"is_active,omitempty"`
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
			QuotaGB: input.Body.QuotaGB, ProfileName: input.Body.ProfileName, IsolirProfile: input.Body.IsolirProfile,
			IsActive: true,
		}
		if input.Body.GraceDays != nil {
			p.GraceDays = *input.Body.GraceDays
		}
		if input.Body.TaxPercent != nil {
			p.TaxPercent = *input.Body.TaxPercent
		}
		if input.Body.IsActive != nil {
			p.IsActive = *input.Body.IsActive
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
			ProfileName   *string  `json:"profile_name,omitempty"`
			IsolirProfile *string  `json:"isolir_profile,omitempty"`
			GraceDays     *int     `json:"grace_days,omitempty"`
			TaxPercent    *float64 `json:"tax_percent,omitempty"`
			IsActive      *bool    `json:"is_active,omitempty"`
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
		if input.Body.GraceDays != nil {
			existing.GraceDays = *input.Body.GraceDays
		}
		if input.Body.TaxPercent != nil {
			existing.TaxPercent = *input.Body.TaxPercent
		}
		if input.Body.IsActive != nil {
			existing.IsActive = *input.Body.IsActive
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
			PlanID    xid.ID `json:"plan_id"`
			ClusterID xid.ID `json:"cluster_id"`
			Price     int64  `json:"price"`
			IsActive  *bool  `json:"is_active,omitempty"`
			Sync      bool   `json:"sync_profiles,omitempty"`
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
		active := true
		if input.Body.IsActive != nil {
			active = *input.Body.IsActive
		}
		o := &store.PlanClusterOffer{
			TenantID: tid, PlanID: input.Body.PlanID, ClusterID: input.Body.ClusterID,
			Price: input.Body.Price, IsActive: active,
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
			Price    int64 `json:"price"`
			IsActive bool  `json:"is_active"`
			Sync     bool  `json:"sync_profiles,omitempty"`
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
		o.Price = input.Body.Price
		o.IsActive = input.Body.IsActive
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
		if err := ensurer.EnsureBandwidthProfile(ctx, tid, r.ID, profile, o.DownloadMbps, o.UploadMbps, o.ServiceType); err != nil {
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

func registerSubscriptions(api huma.API, d *Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "list-subscriptions", Method: http.MethodGet, Path: "/api/subscriptions",
		Tags: []string{"Subscriptions"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Status string `query:"status"`
		Limit  int    `query:"limit"`
		Offset int    `query:"offset"`
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
		list, total, err := d.Store.ListSubscriptions(ctx, tid, input.Status, input.Limit, input.Offset)
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
		OperationID: "activate-subscription", Method: http.MethodPost, Path: "/api/subscriptions/{id}/activate",
		Tags: []string{"Subscriptions"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		ID xid.ID `path:"id"`
	}) (*struct{ Body map[string]string }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		sub, err := d.Store.GetSubscription(ctx, tid, input.ID)
		if err != nil {
			return nil, httpx.NotFound("subscription not found")
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
					if ensurer, ok := prov.(provision.ProfileEnsurer); ok && profile != "" {
						_ = ensurer.EnsureBandwidthProfile(ctx, tid, *sub.RouterID, profile, plan.DownloadMbps, plan.UploadMbps, sub.ServiceType)
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
					_ = prov.Apply(ctx, spec)
				}
			}
		}
		now := time.Now()
		nextBill := d.Billing.NextBillDate(now, plan.BillingCycle)
		_, _ = d.Store.Pool.Exec(ctx, `
			UPDATE subscriptions SET status='active', started_at=$3, next_bill_at=$4, updated_at=NOW()
			WHERE tenant_id=$1 AND id=$2
		`, tid, input.ID, now, nextBill)
		// Refresh local copy for first (possibly prorated) invoice.
		sub.Status = "active"
		sub.StartedAt = &now
		sub.NextBillAt = &nextBill
		if _, err := d.Billing.GenerateInvoiceForSubscription(ctx, tid, input.ID); err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body map[string]string }{Body: map[string]string{"status": "active"}}, nil
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

func registerInvoices(api huma.API, d *Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "list-invoices", Method: http.MethodGet, Path: "/api/invoices",
		Tags: []string{"Invoices"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Status string `query:"status"`
		Limit  int    `query:"limit"`
		Offset int    `query:"offset"`
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
		list, total, err := d.Store.ListInvoices(ctx, tid, input.Status, input.Limit, input.Offset)
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
		inv, _, err := d.Store.GetInvoice(ctx, tid, input.ID)
		if err != nil {
			return nil, httpx.NotFound("invoice not found")
		}
		amount := input.Body.Amount
		if amount == 0 {
			amount = inv.TotalAmount - inv.PaidAmount
		}
		ref := input.Body.Reference
		p := &store.Payment{TenantID: tid, CustomerID: inv.CustomerID, InvoiceID: &input.ID, Amount: amount, Method: input.Body.Method, Reference: &ref}
		if p.Method == "" {
			p.Method = "manual"
		}
		if err := d.Store.RecordPayment(ctx, p); err != nil {
			return nil, httpx.Internal(err)
		}
		if cashID, revID, err := d.Store.FindCashAndRevenueAccounts(ctx, tid); err == nil && !xid.IsNil(cashID) && !xid.IsNil(revID) {
			_ = d.Store.RecordPaymentJournal(ctx, tid, amount, cashID, revID, inv.InvoiceNumber)
		}
		cust, _ := d.Store.GetCustomer(ctx, tid, inv.CustomerID)
		if cust != nil {
			_ = d.Notify.SendPaymentConfirmation(ctx, tid, cust.Phone, inv.InvoiceNumber, amount)
		}
		if inv.SubscriptionID != nil {
			_ = d.Store.UpdateSubscriptionStatus(ctx, tid, *inv.SubscriptionID, "active")
			resumeSubscription(ctx, d, tid, *inv.SubscriptionID)
		}
		return &struct{ Body store.Payment }{Body: *p}, nil
	})
}

func registerTickets(api huma.API, d *Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "list-tickets", Method: http.MethodGet, Path: "/api/tickets",
		Tags: []string{"Tickets"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Status string `query:"status"`
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
		list, total, err := d.Store.ListTickets(ctx, tid, input.Status, input.Limit, input.Offset)
		if err != nil {
			return nil, httpx.Internal(err)
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
			CustomerID  *xid.ID `json:"customer_id, omitempty"`
			Subject     string  `json:"subject"`
			Description string  `json:"description"`
			Category    string  `json:"category"`
			Priority    string  `json:"priority"`
		}
	}) (*struct{ Body store.Ticket }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		desc := input.Body.Description
		t := &store.Ticket{TenantID: tid, CustomerID: input.Body.CustomerID, Subject: input.Body.Subject, Description: &desc, Category: input.Body.Category, Priority: input.Body.Priority, Status: "open"}
		if t.Category == "" {
			t.Category = "general"
		}
		if t.Priority == "" {
			t.Priority = "normal"
		}
		if err := d.Store.CreateTicket(ctx, t); err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body store.Ticket }{Body: *t}, nil
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
			Name      string   `json:"name"`
			Code      string   `json:"code"`
			Address   *string  `json:"address,omitempty"`
			Latitude  *float64 `json:"latitude,omitempty"`
			Longitude *float64 `json:"longitude,omitempty"`
			PortCount int      `json:"port_count,omitempty"`
			ClusterID *xid.ID  `json:"cluster_id,omitempty"`
			ODCID     *xid.ID  `json:"odc_id,omitempty"`
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
			PortCount: input.Body.PortCount,
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
			Name      string   `json:"name"`
			Code      string   `json:"code"`
			Address   *string  `json:"address,omitempty"`
			Latitude  *float64 `json:"latitude,omitempty"`
			Longitude *float64 `json:"longitude,omitempty"`
			ClusterID *xid.ID  `json:"cluster_id,omitempty"`
			ODCID     *xid.ID  `json:"odc_id,omitempty"`
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
		}
		if err := d.Store.UpdateODP(ctx, &o); errors.Is(err, store.ErrNotFound) {
			return nil, httpx.NotFound("odp not found")
		} else if err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body store.ODP }{Body: o}, nil
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
		out := &struct {
			Body struct {
				ODPs      []store.ODP      `json:"odps"`
				Customers []store.Customer `json:"customers"`
				Clusters  []store.Cluster  `json:"clusters"`
			}
		}{}
		out.Body.ODPs = odps
		out.Body.Customers = custs
		out.Body.Clusters = clusters
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "coverage-check", Method: http.MethodGet, Path: "/api/coverage-check",
		Tags: []string{"FTTH"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Lat float64 `query:"lat"`
		Lng float64 `query:"lng"`
	}) (*struct{ Body store.ODP }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		odp, err := d.Store.FindNearestAvailableODP(ctx, tid, input.Lat, input.Lng)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return nil, httpx.NotFound("no available ODP nearby")
			}
			return nil, httpx.Internal(err)
		}
		return &struct{ Body store.ODP }{Body: *odp}, nil
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
		stats, err := d.Store.DashboardStats(ctx, tid)
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

func registerPortal(api huma.API, d *Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "portal-login", Method: http.MethodPost, Path: "/api/portal/login",
		Tags: []string{"Portal"},
	}, func(ctx context.Context, input *struct {
		Body struct {
			Phone      string `json:"phone"`
			Password   string `json:"password"`
			TenantID   xid.ID `json:"tenant_id, omitempty"`
			TenantSlug string `json:"tenant_slug, omitempty"`
		}
	}) (*struct {
		Body struct {
			Customer      store.Customer       `json:"customer"`
			Subscriptions []store.Subscription `json:"subscriptions"`
			Invoices      []store.Invoice      `json:"invoices"`
			Payments      []store.Payment      `json:"payments"`
			WalletBalance int64                `json:"wallet_balance"`
			TenantSlug    string               `json:"tenant_slug"`
			TenantName    string               `json:"tenant_name"`
		}
	}, error) {
		tid := input.Body.TenantID
		slug := strings.ToLower(strings.TrimSpace(input.Body.TenantSlug))
		var ten *store.Tenant
		var err error
		if slug != "" {
			ten, err = d.Store.GetTenantBySlug(ctx, slug)
			if err != nil {
				return nil, httpx.Unauthorized("invalid credentials")
			}
			tid = ten.ID
		} else if !xid.IsNil(tid) {
			ten, err = d.Store.GetTenant(ctx, tid)
			if err != nil {
				return nil, httpx.Unauthorized("invalid credentials")
			}
		} else {
			return nil, httpx.BadRequest("tenant_slug is required")
		}
		if !ten.IsActive {
			return nil, httpx.Unauthorized("tenant inactive")
		}
		cust, err := d.Store.GetCustomerByPhone(ctx, tid, input.Body.Phone)
		if err != nil {
			return nil, httpx.Unauthorized("invalid credentials")
		}
		invoices, _, _ := d.Store.ListInvoices(ctx, tid, "", 20, 0)
		var custInvoices []store.Invoice
		for _, inv := range invoices {
			if inv.CustomerID == cust.ID {
				custInvoices = append(custInvoices, inv)
			}
		}
		subs, _, _ := d.Store.ListSubscriptions(ctx, tid, "", 100, 0)
		var custSubs []store.Subscription
		for _, s := range subs {
			if s.CustomerID == cust.ID {
				custSubs = append(custSubs, s)
			}
		}
		payments, _ := d.Store.ListCustomerPayments(ctx, tid, cust.ID, 20)
		balance, _ := d.Store.GetWallet(ctx, tid, cust.ID)
		out := &struct {
			Body struct {
				Customer      store.Customer       `json:"customer"`
				Subscriptions []store.Subscription `json:"subscriptions"`
				Invoices      []store.Invoice      `json:"invoices"`
				Payments      []store.Payment      `json:"payments"`
				WalletBalance int64                `json:"wallet_balance"`
				TenantSlug    string               `json:"tenant_slug"`
				TenantName    string               `json:"tenant_name"`
			}
		}{}
		out.Body.Customer = *cust
		out.Body.Subscriptions = custSubs
		out.Body.Invoices = custInvoices
		out.Body.Payments = payments
		out.Body.WalletBalance = balance
		out.Body.TenantSlug = ten.Slug
		out.Body.TenantName = ten.Name
		return out, nil
	})
}

func registerWebhooks(api huma.API, d *Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "payment-webhook", Method: http.MethodPost, Path: "/api/webhooks/payment/{provider}",
		Tags: []string{"Webhooks"},
	}, func(ctx context.Context, input *struct {
		Provider           string `path:"provider"`
		RawBody            []byte
		Body               map[string]any
		XSignature         string `header:"X-Signature"`
		XCallbackToken     string `header:"X-CALLBACK-TOKEN"`
		XCallbackSignature string `header:"X-Callback-Signature"`
	}) (*struct{ Body map[string]string }, error) {
		prov, err := d.Payments.Get(input.Provider)
		if err != nil {
			return nil, httpx.NotFound("provider not found")
		}

		bodyMap := input.Body
		raw := input.RawBody
		if len(raw) == 0 && bodyMap != nil {
			raw, _ = json.Marshal(bodyMap)
		}
		if bodyMap == nil && len(raw) > 0 {
			_ = json.Unmarshal(raw, &bodyMap)
		}

		headers := map[string]string{
			"X-Signature":          input.XSignature,
			"X-CALLBACK-TOKEN":     input.XCallbackToken,
			"X-Callback-Signature": input.XCallbackSignature,
		}

		var event *payment.WebhookEvent
		// Verify when provider supports it and keys are configured (non-manual).
		if input.Provider != "manual" && d.Payments.Has(input.Provider) {
			verified, verr := prov.VerifyWebhook(ctx, headers, raw)
			if verr != nil {
				softFail := d.Config != nil && d.Config.AppEnv == "development"
				if softFail {
					slog.Warn("webhook signature soft-fail in development", "provider", input.Provider, "err", verr)
				} else {
					return nil, httpx.Unauthorized("invalid webhook signature")
				}
			} else {
				event = verified
			}
		}
		if event == nil {
			event, err = payment.ParseWebhookEvent(input.Provider, bodyMap)
			if err != nil {
				return nil, httpx.BadRequest(err.Error())
			}
		}

		if event.ExternalID != "" {
			_ = d.Store.UpdatePaymentIntentStatus(ctx, event.ExternalID, event.Status)
		}

		if payment.WebhookIsPaid(event.Status) && event.ExternalID != "" {
			if err := completePaidWebhook(ctx, d, input.Provider, event); err != nil {
				slog.Error("complete paid webhook", "external_id", event.ExternalID, "err", err)
				return nil, httpx.Internal(err)
			}
		}

		return &struct{ Body map[string]string }{Body: map[string]string{"status": "ok"}}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "whatsapp-webhook", Method: http.MethodPost, Path: "/api/webhooks/whatsapp",
		Tags: []string{"Webhooks"},
	}, func(ctx context.Context, input *struct {
		Body struct {
			TenantID xid.ID `json:"tenant_id"`
			Phone    string `json:"phone"`
			Message  string `json:"message"`
		}
	}) (*struct {
		Body struct {
			Reply string `json:"reply"`
		}
	}, error) {
		reply, err := d.Notify.HandleWhatsAppBot(ctx, input.Body.TenantID, input.Body.Phone, input.Body.Message)
		if err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct {
			Body struct {
				Reply string `json:"reply"`
			}
		}{Body: struct {
			Reply string `json:"reply"`
		}{Reply: reply}}, nil
	})
}

func registerVouchers(api huma.API, d *Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "create-voucher-batch", Method: http.MethodPost, Path: "/api/vouchers/batches",
		Tags: []string{"Vouchers"}, Security: []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, input *struct {
		Body struct {
			Name   string   `json:"name"`
			PlanID *xid.ID  `json:"plan_id, omitempty"`
			Price  int64    `json:"price"`
			Codes  []string `json:"codes"`
		}
	}) (*struct{ Body store.VoucherBatch }, error) {
		tid, err := tenantIDFromCtx(ctx)
		if err != nil {
			return nil, err
		}
		b := &store.VoucherBatch{TenantID: tid, PlanID: input.Body.PlanID, Name: input.Body.Name, Price: input.Body.Price}
		if err := d.Store.CreateVoucherBatch(ctx, b, input.Body.Codes); err != nil {
			return nil, httpx.Internal(err)
		}
		return &struct{ Body store.VoucherBatch }{Body: *b}, nil
	})
}
