package store

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/dianrp/drp-billing/internal/xid"
)

const generalSettingKey = "tenant.general"

var hexColorRE = regexp.MustCompile(`(?i)^#?[0-9A-F]{6}$`)

type GeneralSettings struct {
	Timezone          string  `json:"timezone"`
	DefaultTaxPercent float64 `json:"default_tax_percent"`
	PrimaryColor      string  `json:"primary_color,omitempty"`
}

func DefaultGeneralSettings() GeneralSettings {
	return GeneralSettings{
		Timezone:          "Asia/Jakarta",
		DefaultTaxPercent: 0,
		PrimaryColor:      "",
	}
}

func NormalizeHexColor(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if !hexColorRE.MatchString(s) {
		return ""
	}
	if s[0] != '#' {
		s = "#" + s
	}
	return "#" + strings.ToUpper(s[1:])
}

func NormalizeGeneralSettings(g GeneralSettings) GeneralSettings {
	def := DefaultGeneralSettings()
	g.Timezone = strings.TrimSpace(g.Timezone)
	if g.Timezone == "" {
		g.Timezone = def.Timezone
	}
	// Validate IANA name when possible
	if _, err := time.LoadLocation(g.Timezone); err != nil {
		g.Timezone = def.Timezone
	}
	if g.DefaultTaxPercent < 0 {
		g.DefaultTaxPercent = 0
	}
	if g.DefaultTaxPercent > 100 {
		g.DefaultTaxPercent = 100
	}
	g.PrimaryColor = NormalizeHexColor(g.PrimaryColor)
	return g
}

func (s *Store) GetGeneralSettings(ctx context.Context, tenantID xid.ID) (GeneralSettings, error) {
	cfg := DefaultGeneralSettings()
	err := s.GetSettingJSON(ctx, tenantID, generalSettingKey, &cfg)
	if errors.Is(err, ErrNotFound) {
		return DefaultGeneralSettings(), nil
	}
	if err != nil {
		return DefaultGeneralSettings(), err
	}
	return NormalizeGeneralSettings(cfg), nil
}

func (s *Store) UpsertGeneralSettings(ctx context.Context, tenantID xid.ID, cfg GeneralSettings) error {
	return s.UpsertSettingJSON(ctx, tenantID, generalSettingKey, NormalizeGeneralSettings(cfg))
}

func (s *Store) UpdateTenantName(ctx context.Context, tenantID xid.ID, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("nama tenant wajib diisi")
	}
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return err
	}
	tag, err := s.Pool.Exec(ctx, `UPDATE tenants SET name=$2, updated_at=NOW() WHERE id=$1`, tenantID, name)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// EffectiveTaxPercent returns tenant default tax (plans no longer drive tax).
func (s *Store) EffectiveTaxPercent(ctx context.Context, tenantID xid.ID) (float64, error) {
	g, err := s.GetGeneralSettings(ctx, tenantID)
	if err != nil {
		return 0, err
	}
	return g.DefaultTaxPercent, nil
}
