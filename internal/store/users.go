package store

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/dianrp/drp-billing/internal/db"
	"github.com/dianrp/drp-billing/internal/xid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("not found")

type Store struct {
	Pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Store {
	return &Store{Pool: pool}
}

type User struct {
	ID              xid.ID     `json:"id"`
	Email           string     `json:"email"`
	PasswordHash    string     `json:"-"`
	FullName        string     `json:"full_name"`
	Phone           *string    `json:"phone,omitempty"`
	AvatarURL       *string    `json:"avatar_url,omitempty"`
	IsActive        bool       `json:"is_active"`
	IsPlatformAdmin bool       `json:"is_platform_admin,omitempty"`
	TOTPSecret      *string    `json:"-"`
	TOTPEnabled     bool       `json:"totp_enabled"`
	LastLoginAt     *time.Time `json:"last_login_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
}

type Tenant struct {
	ID        xid.ID    `json:"id"`
	Slug      string    `json:"slug"`
	Name      string    `json:"name"`
	Email     *string   `json:"email,omitempty"`
	Phone     *string   `json:"phone,omitempty"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at,omitempty"`
}

type UserTenant struct {
	UserID     xid.ID `json:"user_id"`
	TenantID   xid.ID `json:"tenant_id"`
	TenantSlug string `json:"tenant_slug,omitempty"`
	TenantName string `json:"tenant_name,omitempty"`
	RoleSlug   string `json:"role_slug"`
	RoleName   string `json:"role_name"`
}

func (s *Store) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	row := s.Pool.QueryRow(ctx, `
		SELECT id, email, password_hash, full_name, phone, avatar_url, is_active, COALESCE(is_platform_admin,false),
		       totp_secret, totp_enabled, last_login_at, created_at
		FROM users WHERE email = $1
	`, email)
	var u User
	err := row.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.FullName, &u.Phone, &u.AvatarURL, &u.IsActive, &u.IsPlatformAdmin,
		&u.TOTPSecret, &u.TOTPEnabled, &u.LastLoginAt, &u.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &u, err
}

func (s *Store) GetUserByID(ctx context.Context, id xid.ID) (*User, error) {
	row := s.Pool.QueryRow(ctx, `
		SELECT id, email, password_hash, full_name, phone, avatar_url, is_active, COALESCE(is_platform_admin,false),
		       totp_secret, totp_enabled, last_login_at, created_at
		FROM users WHERE id = $1
	`, id)
	var u User
	err := row.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.FullName, &u.Phone, &u.AvatarURL, &u.IsActive, &u.IsPlatformAdmin,
		&u.TOTPSecret, &u.TOTPEnabled, &u.LastLoginAt, &u.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &u, err
}

// UpdateMyProfile updates the signed-in user's display name and optionally password hash.
func (s *Store) UpdateMyProfile(ctx context.Context, id xid.ID, fullName string, passwordHash *string) error {
	fullName = strings.TrimSpace(fullName)
	if fullName == "" {
		return errors.New("full_name required")
	}
	if passwordHash != nil && *passwordHash != "" {
		_, err := s.Pool.Exec(ctx, `
			UPDATE users SET full_name=$2, password_hash=$3, updated_at=NOW() WHERE id=$1
		`, id, fullName, *passwordHash)
		return err
	}
	_, err := s.Pool.Exec(ctx, `
		UPDATE users SET full_name=$2, updated_at=NOW() WHERE id=$1
	`, id, fullName)
	return err
}

// UpdateUserAvatar sets or clears the user's avatar URL.
func (s *Store) UpdateUserAvatar(ctx context.Context, id xid.ID, avatarURL *string) error {
	_, err := s.Pool.Exec(ctx, `
		UPDATE users SET avatar_url=$2, updated_at=NOW() WHERE id=$1
	`, id, avatarURL)
	return err
}

func (s *Store) ListUserTenants(ctx context.Context, userID xid.ID) ([]UserTenant, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT ut.user_id, ut.tenant_id, t.slug, t.name, r.slug, r.name
		FROM user_tenants ut
		JOIN roles r ON r.id = ut.role_id
		JOIN tenants t ON t.id = ut.tenant_id
		WHERE ut.user_id = $1
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []UserTenant
	for rows.Next() {
		var ut UserTenant
		if err := rows.Scan(&ut.UserID, &ut.TenantID, &ut.TenantSlug, &ut.TenantName, &ut.RoleSlug, &ut.RoleName); err != nil {
			return nil, err
		}
		list = append(list, ut)
	}
	return list, rows.Err()
}

func (s *Store) SaveRefreshToken(ctx context.Context, userID xid.ID, tokenHash string, expiresAt time.Time) error {
	_, err := s.Pool.Exec(ctx, `
		INSERT INTO refresh_tokens (user_id, token_hash, expires_at) VALUES ($1, $2, $3)
	`, userID, tokenHash, expiresAt)
	return err
}

func (s *Store) RevokeRefreshToken(ctx context.Context, tokenHash string) error {
	_, err := s.Pool.Exec(ctx, `
		UPDATE refresh_tokens SET revoked_at = NOW() WHERE token_hash = $1
	`, tokenHash)
	return err
}

func (s *Store) IsRefreshTokenValid(ctx context.Context, tokenHash string) (bool, error) {
	var valid bool
	err := s.Pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM refresh_tokens
			WHERE token_hash = $1 AND revoked_at IS NULL AND expires_at > NOW()
		)
	`, tokenHash).Scan(&valid)
	return valid, err
}

func (s *Store) UpdateLastLogin(ctx context.Context, userID xid.ID) error {
	_, err := s.Pool.Exec(ctx, `UPDATE users SET last_login_at = NOW() WHERE id = $1`, userID)
	return err
}

func (s *Store) GetTenant(ctx context.Context, id xid.ID) (*Tenant, error) {
	row := s.Pool.QueryRow(ctx, `
		SELECT id, slug, name, email, phone, is_active, created_at FROM tenants WHERE id = $1
	`, id)
	var t Tenant
	err := row.Scan(&t.ID, &t.Slug, &t.Name, &t.Email, &t.Phone, &t.IsActive, &t.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &t, err
}

func (s *Store) GetTenantBySlug(ctx context.Context, slug string) (*Tenant, error) {
	row := s.Pool.QueryRow(ctx, `
		SELECT id, slug, name, email, phone, is_active, created_at FROM tenants WHERE slug = $1
	`, slug)
	var t Tenant
	err := row.Scan(&t.ID, &t.Slug, &t.Name, &t.Email, &t.Phone, &t.IsActive, &t.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &t, err
}

func (s *Store) UpdateTenant(ctx context.Context, t *Tenant) error {
	_, err := s.Pool.Exec(ctx, `
		UPDATE tenants SET name=$2, email=$3, phone=$4, is_active=$5, updated_at=NOW()
		WHERE id=$1
	`, t.ID, t.Name, t.Email, t.Phone, t.IsActive)
	return err
}

func (s *Store) DeleteTenant(ctx context.Context, id xid.ID) error {
	tag, err := s.Pool.Exec(ctx, `DELETE FROM tenants WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) CreatePlatformAdmin(ctx context.Context, email, passwordHash, fullName string) (xid.ID, error) {
	var id xid.ID
	err := s.Pool.QueryRow(ctx, `
		INSERT INTO users (email, password_hash, full_name, is_platform_admin)
		VALUES ($1,$2,$3,true)
		ON CONFLICT (email) DO UPDATE
		  SET password_hash=EXCLUDED.password_hash, full_name=EXCLUDED.full_name,
		      is_platform_admin=true, updated_at=NOW()
		RETURNING id
	`, email, passwordHash, fullName).Scan(&id)
	return id, err
}

func (s *Store) UpdateUserPassword(ctx context.Context, email, passwordHash string) error {
	tag, err := s.Pool.Exec(ctx, `UPDATE users SET password_hash=$2, updated_at=NOW() WHERE email=$1`, email, passwordHash)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) SetTenantContext(ctx context.Context, tenantID xid.ID) error {
	// set_config requires text; passing int64 makes Postgres reject the call (→ HTTP 500).
	return db.SetTenant(ctx, s.Pool, tenantID)
}

// withTenant runs fn inside a transaction with SET LOCAL app.tenant_id so RLS
// and the query share the same connection (pgx pool otherwise splits them).
func (s *Store) withTenant(ctx context.Context, tenantID xid.ID, fn func(ctx context.Context, tx pgx.Tx) error) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := db.SetTenantTx(ctx, tx, tenantID); err != nil {
		return err
	}
	if err := fn(ctx, tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) CreateTenantWithAdmin(ctx context.Context, slug, name, email, passwordHash, fullName string) (tenantID, userID xid.ID, err error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return xid.Nil(), xid.Nil(), err
	}
	defer tx.Rollback(ctx)

	err = tx.QueryRow(ctx, `
		INSERT INTO tenants (slug, name, email) VALUES ($1, $2, $3) RETURNING id
	`, slug, name, email).Scan(&tenantID)
	if err != nil {
		return xid.Nil(), xid.Nil(), err
	}

	err = tx.QueryRow(ctx, `
		INSERT INTO roles (tenant_id, name, slug, permissions, is_system)
		VALUES ($1, 'Administrator', 'admin', '["*"]', true) RETURNING id
	`, tenantID).Scan(new(xid.ID))
	if err != nil {
		return xid.Nil(), xid.Nil(), err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO roles (tenant_id, name, slug, permissions, is_system) VALUES
			($1, 'Teknisi', 'teknisi', '["dashboard","network","ops","tickets"]', true),
			($1, 'Sales', 'sales', '["dashboard","customers","leads","billing"]', true)
		ON CONFLICT (tenant_id, slug) DO NOTHING
	`, tenantID)
	if err != nil {
		return xid.Nil(), xid.Nil(), err
	}

	var roleID xid.ID
	err = tx.QueryRow(ctx, `SELECT id FROM roles WHERE tenant_id = $1 AND slug = 'admin'`, tenantID).Scan(&roleID)
	if err != nil {
		return xid.Nil(), xid.Nil(), err
	}

	err = tx.QueryRow(ctx, `
		INSERT INTO users (email, password_hash, full_name) VALUES ($1, $2, $3) RETURNING id
	`, email, passwordHash, fullName).Scan(&userID)
	if err != nil {
		return xid.Nil(), xid.Nil(), err
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO user_tenants (user_id, tenant_id, role_id, is_default) VALUES ($1, $2, $3, true)
	`, userID, tenantID, roleID)
	if err != nil {
		return xid.Nil(), xid.Nil(), err
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO chart_of_accounts (tenant_id, code, name, type) VALUES
			($1, '1110', 'Kas', 'asset'),
			($1, '4100', 'Pendapatan langganan', 'revenue'),
			($1, '5100', 'Beban operasional', 'expense')
	`, tenantID)
	if err != nil {
		return xid.Nil(), xid.Nil(), err
	}

	return tenantID, userID, tx.Commit(ctx)
}
