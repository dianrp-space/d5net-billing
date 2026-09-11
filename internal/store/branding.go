package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/dianrp-space/d5net-billing/internal/xid"
	"github.com/jackc/pgx/v5"
)

type Branding struct {
	AppName            string  `json:"app_name"`
	LogoURL            *string `json:"logo_url,omitempty"`
	FaviconURL         *string `json:"favicon_url,omitempty"`
	MapPopIconURL      *string `json:"map_pop_icon_url,omitempty"`
	MapODPIconURL      *string `json:"map_odp_icon_url,omitempty"`
	MapCustomerIconURL *string `json:"map_customer_icon_url,omitempty"`
}

type TenantBrandingView struct {
	Overrides Branding `json:"overrides"`
	Effective Branding `json:"effective"`
	FromOwner struct {
		AppName            bool `json:"app_name"`
		LogoURL            bool `json:"logo_url"`
		FaviconURL         bool `json:"favicon_url"`
		MapPopIconURL      bool `json:"map_pop_icon_url"`
		MapODPIconURL      bool `json:"map_odp_icon_url"`
		MapCustomerIconURL bool `json:"map_customer_icon_url"`
	} `json:"from_owner"`
}

func (s *Store) GetPlatformBranding(ctx context.Context) (*Branding, error) {
	row := s.Pool.QueryRow(ctx, `
		SELECT app_name, logo_url, favicon_url,
		       map_pop_icon_url, map_odp_icon_url, map_customer_icon_url
		FROM platform_branding WHERE id = 1
	`)
	var b Branding
	err := row.Scan(&b.AppName, &b.LogoURL, &b.FaviconURL, &b.MapPopIconURL, &b.MapODPIconURL, &b.MapCustomerIconURL)
	if errors.Is(err, pgx.ErrNoRows) {
		return &Branding{AppName: "D5Net"}, nil
	}
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(b.AppName) == "" {
		b.AppName = "D5Net"
	}
	return &b, nil
}

func (s *Store) UpdatePlatformBranding(ctx context.Context, b *Branding) error {
	name := strings.TrimSpace(b.AppName)
	if name == "" {
		name = "D5Net"
	}
	_, err := s.Pool.Exec(ctx, `
		INSERT INTO platform_branding (id, app_name, logo_url, favicon_url,
			map_pop_icon_url, map_odp_icon_url, map_customer_icon_url, updated_at)
		VALUES (1, $1, $2, $3, $4, $5, $6, NOW())
		ON CONFLICT (id) DO UPDATE SET
			app_name = EXCLUDED.app_name,
			logo_url = EXCLUDED.logo_url,
			favicon_url = EXCLUDED.favicon_url,
			map_pop_icon_url = EXCLUDED.map_pop_icon_url,
			map_odp_icon_url = EXCLUDED.map_odp_icon_url,
			map_customer_icon_url = EXCLUDED.map_customer_icon_url,
			updated_at = NOW()
	`, name, b.LogoURL, b.FaviconURL, b.MapPopIconURL, b.MapODPIconURL, b.MapCustomerIconURL)
	return err
}

func (s *Store) GetTenantBrandingRaw(ctx context.Context, tenantID xid.ID) (*Branding, error) {
	row := s.Pool.QueryRow(ctx, `
		SELECT COALESCE(app_name, ''), logo_url, favicon_url,
		       map_pop_icon_url, map_odp_icon_url, map_customer_icon_url
		FROM tenants WHERE id = $1
	`, tenantID)
	var b Branding
	err := row.Scan(&b.AppName, &b.LogoURL, &b.FaviconURL, &b.MapPopIconURL, &b.MapODPIconURL, &b.MapCustomerIconURL)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &b, err
}

func (s *Store) UpdateTenantBranding(ctx context.Context, tenantID xid.ID, b *Branding) error {
	name := strings.TrimSpace(b.AppName)
	var namePtr *string
	if name != "" {
		namePtr = &name
	}
	tag, err := s.Pool.Exec(ctx, `
		UPDATE tenants SET app_name=$2, logo_url=$3, favicon_url=$4,
			map_pop_icon_url=$5, map_odp_icon_url=$6, map_customer_icon_url=$7, updated_at=NOW()
		WHERE id=$1
	`, tenantID, namePtr, b.LogoURL, b.FaviconURL, b.MapPopIconURL, b.MapODPIconURL, b.MapCustomerIconURL)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// coalesceDisplayName prefers tenant branding app_name, then tenants.name (Umum), then platform branding.
func coalesceDisplayName(appName, tenantName, ownerAppName, def string) (value string, fromOwner bool) {
	if v := strings.TrimSpace(appName); v != "" {
		return v, false
	}
	if v := strings.TrimSpace(tenantName); v != "" {
		return v, false
	}
	if v := strings.TrimSpace(ownerAppName); v != "" {
		return v, true
	}
	return def, true
}

func coalescePtr(primary, fallback *string) (value *string, fromOwner bool) {
	if primary != nil && strings.TrimSpace(*primary) != "" {
		return primary, false
	}
	if fallback != nil && strings.TrimSpace(*fallback) != "" {
		return fallback, true
	}
	return nil, true
}

func (s *Store) ResolveTenantBranding(ctx context.Context, tenantID xid.ID) (*TenantBrandingView, error) {
	raw, err := s.GetTenantBrandingRaw(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	owner, err := s.GetPlatformBranding(ctx)
	if err != nil {
		return nil, err
	}
	tenantName := ""
	if ten, terr := s.GetTenant(ctx, tenantID); terr == nil && ten != nil {
		tenantName = ten.Name
	}
	view := &TenantBrandingView{Overrides: *raw}
	app, fromApp := coalesceDisplayName(raw.AppName, tenantName, owner.AppName, "D5Net")
	logo, fromLogo := coalescePtr(raw.LogoURL, owner.LogoURL)
	fav, fromFav := coalescePtr(raw.FaviconURL, owner.FaviconURL)
	popIcon, fromPopIcon := coalescePtr(raw.MapPopIconURL, owner.MapPopIconURL)
	odpIcon, fromOdpIcon := coalescePtr(raw.MapODPIconURL, owner.MapODPIconURL)
	custIcon, fromCustIcon := coalescePtr(raw.MapCustomerIconURL, owner.MapCustomerIconURL)
	view.Effective = Branding{
		AppName:            app,
		LogoURL:            logo,
		FaviconURL:         fav,
		MapPopIconURL:      popIcon,
		MapODPIconURL:      odpIcon,
		MapCustomerIconURL: custIcon,
	}
	view.FromOwner.AppName = fromApp
	view.FromOwner.LogoURL = fromLogo
	view.FromOwner.FaviconURL = fromFav
	view.FromOwner.MapPopIconURL = fromPopIcon
	view.FromOwner.MapODPIconURL = fromOdpIcon
	view.FromOwner.MapCustomerIconURL = fromCustIcon
	return view, nil
}

// MapIcons carries the tenant-effective custom icon URLs for FTTH map markers.
// A nil field means "use the built-in SVG marker" for that kind.
type MapIcons struct {
	Pop      *string `json:"pop,omitempty"`
	ODP      *string `json:"odp,omitempty"`
	Customer *string `json:"customer,omitempty"`
}

// EffectiveMapIcons returns the tenant-effective custom map icon URLs.
func (s *Store) EffectiveMapIcons(ctx context.Context, tenantID xid.ID) (MapIcons, error) {
	view, err := s.ResolveTenantBranding(ctx, tenantID)
	if err != nil {
		return MapIcons{}, err
	}
	return MapIcons{
		Pop:      view.Effective.MapPopIconURL,
		ODP:      view.Effective.MapODPIconURL,
		Customer: view.Effective.MapCustomerIconURL,
	}, nil
}

func (s *Store) EffectiveAppName(ctx context.Context, tenantID xid.ID) (string, error) {
	v, err := s.ResolveTenantBranding(ctx, tenantID)
	if err != nil {
		return "D5Net", err
	}
	return v.Effective.AppName, nil
}

type Role struct {
	ID          xid.ID    `json:"id"`
	TenantID    xid.ID    `json:"tenant_id"`
	Name        string    `json:"name"`
	Slug        string    `json:"slug"`
	Permissions []string  `json:"permissions"`
	IsSystem    bool      `json:"is_system"`
	CreatedAt   time.Time `json:"created_at,omitempty"`
}

func scanRole(scan func(dest ...any) error) (*Role, error) {
	var r Role
	var perms []byte
	err := scan(&r.ID, &r.TenantID, &r.Name, &r.Slug, &perms, &r.IsSystem, &r.CreatedAt)
	if err != nil {
		return nil, err
	}
	r.Permissions = []string{}
	if len(perms) > 0 {
		_ = json.Unmarshal(perms, &r.Permissions)
	}
	return &r, nil
}

func (s *Store) ListRoles(ctx context.Context, tenantID xid.ID) ([]Role, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT id, tenant_id, name, slug, COALESCE(permissions, '[]'::jsonb), is_system, created_at
		FROM roles WHERE tenant_id = $1 ORDER BY is_system DESC, name
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []Role
	for rows.Next() {
		r, err := scanRole(rows.Scan)
		if err != nil {
			return nil, err
		}
		list = append(list, *r)
	}
	return list, rows.Err()
}

func (s *Store) GetRole(ctx context.Context, tenantID, id xid.ID) (*Role, error) {
	row := s.Pool.QueryRow(ctx, `
		SELECT id, tenant_id, name, slug, COALESCE(permissions, '[]'::jsonb), is_system, created_at
		FROM roles WHERE tenant_id = $1 AND id = $2
	`, tenantID, id)
	r, err := scanRole(row.Scan)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return r, err
}

func (s *Store) GetRoleBySlug(ctx context.Context, tenantID xid.ID, slug string) (*Role, error) {
	row := s.Pool.QueryRow(ctx, `
		SELECT id, tenant_id, name, slug, COALESCE(permissions, '[]'::jsonb), is_system, created_at
		FROM roles WHERE tenant_id = $1 AND slug = $2
	`, tenantID, slug)
	r, err := scanRole(row.Scan)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return r, err
}

func (s *Store) CreateRole(ctx context.Context, r *Role) error {
	if r.Permissions == nil {
		r.Permissions = []string{}
	}
	perms, _ := json.Marshal(r.Permissions)
	return s.Pool.QueryRow(ctx, `
		INSERT INTO roles (tenant_id, name, slug, permissions, is_system)
		VALUES ($1,$2,$3,$4::jsonb,$5) RETURNING id, created_at
	`, r.TenantID, r.Name, r.Slug, perms, r.IsSystem).Scan(&r.ID, &r.CreatedAt)
}

func (s *Store) UpdateRole(ctx context.Context, r *Role) error {
	if r.Permissions == nil {
		r.Permissions = []string{}
	}
	perms, _ := json.Marshal(r.Permissions)
	tag, err := s.Pool.Exec(ctx, `
		UPDATE roles SET name=$3, slug=$4, permissions=$5::jsonb
		WHERE tenant_id=$1 AND id=$2
	`, r.TenantID, r.ID, r.Name, r.Slug, perms)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteRole(ctx context.Context, tenantID, id xid.ID) error {
	var isSystem bool
	err := s.Pool.QueryRow(ctx, `SELECT is_system FROM roles WHERE tenant_id=$1 AND id=$2`, tenantID, id).Scan(&isSystem)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if isSystem {
		return errors.New("system role cannot be deleted")
	}
	tag, err := s.Pool.Exec(ctx, `DELETE FROM roles WHERE tenant_id=$1 AND id=$2 AND is_system=false`, tenantID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

type TenantUser struct {
	UserID    xid.ID  `json:"user_id"`
	Email     string  `json:"email"`
	FullName  string  `json:"full_name"`
	Phone     *string `json:"phone,omitempty"`
	IsActive  bool    `json:"is_active"`
	RoleID    xid.ID  `json:"role_id"`
	RoleSlug  string  `json:"role_slug"`
	RoleName  string  `json:"role_name"`
	CreatedAt time.Time `json:"created_at,omitempty"`
}

func (s *Store) ListTenantUsers(ctx context.Context, tenantID xid.ID) ([]TenantUser, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT u.id, u.email, u.full_name, u.phone, u.is_active, r.id, r.slug, r.name, u.created_at
		FROM user_tenants ut
		JOIN users u ON u.id = ut.user_id
		JOIN roles r ON r.id = ut.role_id
		WHERE ut.tenant_id = $1
		ORDER BY u.full_name
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []TenantUser
	for rows.Next() {
		var u TenantUser
		if err := rows.Scan(&u.UserID, &u.Email, &u.FullName, &u.Phone, &u.IsActive, &u.RoleID, &u.RoleSlug, &u.RoleName, &u.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, u)
	}
	return list, rows.Err()
}

func (s *Store) AddTenantUser(ctx context.Context, tenantID, roleID xid.ID, email, passwordHash, fullName string, phone *string) (xid.ID, error) {
	var userID xid.ID
	err := s.withTenant(ctx, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `SELECT id FROM users WHERE email = $1`, email).Scan(&userID)
		if errors.Is(err, pgx.ErrNoRows) {
			err = tx.QueryRow(ctx, `
				INSERT INTO users (email, password_hash, full_name, phone) VALUES ($1,$2,$3,$4) RETURNING id
			`, email, passwordHash, fullName, phone).Scan(&userID)
			if err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else {
			_, err = tx.Exec(ctx, `
				UPDATE users SET password_hash=$2, full_name=$3, phone=COALESCE($4, phone), updated_at=NOW()
				WHERE id=$1
			`, userID, passwordHash, fullName, phone)
			if err != nil {
				return err
			}
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO user_tenants (user_id, tenant_id, role_id, is_default)
			VALUES ($1,$2,$3,true)
			ON CONFLICT (user_id, tenant_id) DO UPDATE SET role_id = EXCLUDED.role_id
		`, userID, tenantID, roleID)
		return err
	})
	return userID, err
}

func (s *Store) UpdateTenantUser(ctx context.Context, tenantID, userID, roleID xid.ID, fullName string, phone *string, isActive bool) error {
	return s.withTenant(ctx, tenantID, func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE user_tenants SET role_id=$3 WHERE tenant_id=$1 AND user_id=$2
		`, tenantID, userID, roleID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		_, err = tx.Exec(ctx, `
			UPDATE users SET full_name=$2, phone=$3, is_active=$4, updated_at=NOW() WHERE id=$1
		`, userID, fullName, phone, isActive)
		return err
	})
}

func (s *Store) SetUserPassword(ctx context.Context, userID xid.ID, passwordHash string) error {
	tag, err := s.Pool.Exec(ctx, `UPDATE users SET password_hash=$2, updated_at=NOW() WHERE id=$1`, userID, passwordHash)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) RemoveTenantUser(ctx context.Context, tenantID, userID xid.ID) error {
	tag, err := s.Pool.Exec(ctx, `DELETE FROM user_tenants WHERE tenant_id=$1 AND user_id=$2`, tenantID, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

type PortalUser struct {
	ID            xid.ID `json:"id"`
	CustomerCode  string `json:"customer_code"`
	FullName      string `json:"full_name"`
	Phone         string `json:"phone"`
	PortalEnabled bool   `json:"portal_enabled"`
	HasPassword   bool   `json:"has_password"`
	IsActive      bool   `json:"is_active"`
}

func (s *Store) ListPortalUsers(ctx context.Context, tenantID xid.ID) ([]PortalUser, error) {
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return nil, err
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT id, customer_code, full_name, phone, portal_enabled,
		       (password_hash IS NOT NULL AND password_hash <> ''), is_active
		FROM customers WHERE tenant_id = $1 ORDER BY full_name
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []PortalUser
	for rows.Next() {
		var p PortalUser
		if err := rows.Scan(&p.ID, &p.CustomerCode, &p.FullName, &p.Phone, &p.PortalEnabled, &p.HasPassword, &p.IsActive); err != nil {
			return nil, err
		}
		list = append(list, p)
	}
	return list, rows.Err()
}

func (s *Store) UpdatePortalUser(ctx context.Context, tenantID, customerID xid.ID, portalEnabled bool, passwordHash *string) error {
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return err
	}
	if passwordHash != nil {
		tag, err := s.Pool.Exec(ctx, `
			UPDATE customers SET portal_enabled=$3, password_hash=$4, updated_at=NOW()
			WHERE tenant_id=$1 AND id=$2
		`, tenantID, customerID, portalEnabled, *passwordHash)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		return nil
	}
	tag, err := s.Pool.Exec(ctx, `
		UPDATE customers SET portal_enabled=$3, updated_at=NOW()
		WHERE tenant_id=$1 AND id=$2
	`, tenantID, customerID, portalEnabled)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) UpdateCustomerPhoto(ctx context.Context, tenantID, customerID xid.ID, photoURL *string) error {
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return err
	}
	tag, err := s.Pool.Exec(ctx, `
		UPDATE customers SET photo_url=$3, updated_at=NOW()
		WHERE tenant_id=$1 AND id=$2
	`, tenantID, customerID, photoURL)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) GetCustomerPasswordHash(ctx context.Context, tenantID, customerID xid.ID) (string, error) {
	var hash *string
	err := s.Pool.QueryRow(ctx, `
		SELECT password_hash FROM customers WHERE tenant_id=$1 AND id=$2
	`, tenantID, customerID).Scan(&hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if hash == nil {
		return "", nil
	}
	return *hash, nil
}

func RoleHasPermission(perms []string, need string) bool {
	for _, p := range perms {
		if p == "*" || p == need {
			return true
		}
		if p == "settings:*" && strings.HasPrefix(need, "settings") {
			return true
		}
		if strings.HasSuffix(p, ":*") {
			prefix := strings.TrimSuffix(p, "*")
			if strings.HasPrefix(need, prefix) {
				return true
			}
		}
	}
	return false
}

// CanDispatchOps: admin/dispatcher may create & assign tickets/WO.
// Field technicians (ops/tickets/tech without broader admin perms) cannot.
func CanDispatchOps(perms []string) bool {
	if RoleHasPermission(perms, "*") ||
		RoleHasPermission(perms, "settings") ||
		RoleHasPermission(perms, "customers") ||
		RoleHasPermission(perms, "billing") {
		return true
	}
	return false
}

func IsFieldOpsRole(perms []string) bool {
	if CanDispatchOps(perms) {
		return false
	}
	return RoleHasPermission(perms, "ops") ||
		RoleHasPermission(perms, "tickets") ||
		RoleHasPermission(perms, "tech")
}
