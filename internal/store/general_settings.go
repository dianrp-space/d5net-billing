package store

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/dianrp-space/d5net-billing/internal/xid"
)

const generalSettingKey = "tenant.general"

var hexColorRE = regexp.MustCompile(`(?i)^#?[0-9A-F]{6}$`)

type GeneralSettings struct {
	Timezone          string  `json:"timezone"`
	DefaultTaxPercent float64 `json:"default_tax_percent"`
	PrimaryColor      string  `json:"primary_color,omitempty"`
	// IsolirGraceDays is days after invoice due_date before auto-isolir (tenant-wide).
	IsolirGraceDays int `json:"isolir_grace_days"`
	// BillingCycleStartDay is the calendar day (1–28) prorata activations lock to.
	BillingCycleStartDay int `json:"billing_cycle_start_day"`
	// InvoiceDueDay is the tenant default calendar day (1–28) invoices fall due.
	// Plans and cluster offers may override it.
	InvoiceDueDay int `json:"invoice_due_day"`
	// LateFeePercent is denda keterlambatan (%) atas total tunggakan saat
	// tagihan baru terbit. 0 = nonaktif.
	LateFeePercent float64 `json:"late_fee_percent"`
	// WalletEnabled mengaktifkan saldo pelanggan: topup mandiri + auto-pay
	// tagihan dari saldo saat tagihan terbit.
	WalletEnabled bool `json:"wallet_enabled"`
	// WalletMinTopup adalah minimum nominal topup saldo (Rp).
	WalletMinTopup int64 `json:"wallet_min_topup"`
	// Profil publik situs (dipakai halaman landing & onboarding payment gateway).
	About              string `json:"about,omitempty"`
	ProductDescription string `json:"product_description,omitempty"`
	SupportEmail       string `json:"support_email,omitempty"`
	SupportPhone       string `json:"support_phone,omitempty"`
	SupportAddress     string `json:"support_address,omitempty"`
}

func DefaultGeneralSettings() GeneralSettings {
	return GeneralSettings{
		Timezone:             "Asia/Jakarta",
		DefaultTaxPercent:    0,
		PrimaryColor:         "",
		BillingCycleStartDay: 1,
		InvoiceDueDay:        10,
		LateFeePercent:       5,
		WalletEnabled:        false,
		WalletMinTopup:       10000,
	}
}

// ClampDueDay bounds a calendar due day to 1–28. Unset/zero falls back to def
// (or 1 when def is itself out of range); values above 28 cap at 28.
func ClampDueDay(day, def int) int {
	if day < 1 {
		if def < 1 || def > 28 {
			return 1
		}
		return def
	}
	if day > 28 {
		return 28
	}
	return day
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
	if g.IsolirGraceDays < 0 {
		g.IsolirGraceDays = 0
	}
	if g.IsolirGraceDays > 30 {
		g.IsolirGraceDays = 30
	}
	if g.BillingCycleStartDay < 1 {
		g.BillingCycleStartDay = def.BillingCycleStartDay
	}
	if g.BillingCycleStartDay > 28 {
		g.BillingCycleStartDay = 28
	}
	g.InvoiceDueDay = ClampDueDay(g.InvoiceDueDay, def.InvoiceDueDay)
	if g.LateFeePercent < 0 {
		g.LateFeePercent = 0
	}
	if g.LateFeePercent > 100 {
		g.LateFeePercent = 100
	}
	if g.WalletMinTopup < 1000 {
		g.WalletMinTopup = def.WalletMinTopup
	}
	if g.WalletMinTopup > 100_000_000 {
		g.WalletMinTopup = 100_000_000
	}
	g.About = strings.TrimSpace(g.About)
	g.ProductDescription = strings.TrimSpace(g.ProductDescription)
	g.SupportEmail = strings.TrimSpace(g.SupportEmail)
	g.SupportPhone = strings.TrimSpace(g.SupportPhone)
	g.SupportAddress = strings.TrimSpace(g.SupportAddress)
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

// IsolirGraceDays is how many days after invoice due_date auto-isolir waits.
func (s *Store) IsolirGraceDays(ctx context.Context, tenantID xid.ID) int {
	g, err := s.GetGeneralSettings(ctx, tenantID)
	if err != nil {
		return 0
	}
	return g.IsolirGraceDays
}

// BillingCycleStartDay is the tenant calendar day prorata next_bill_at snaps to.
func (s *Store) BillingCycleStartDay(ctx context.Context, tenantID xid.ID) int {
	g, err := s.GetGeneralSettings(ctx, tenantID)
	if err != nil {
		return 1
	}
	return g.BillingCycleStartDay
}

// InvoiceDueDay is the tenant default calendar day (1–28) invoices fall due.
func (s *Store) InvoiceDueDay(ctx context.Context, tenantID xid.ID) int {
	g, err := s.GetGeneralSettings(ctx, tenantID)
	if err != nil {
		return DefaultGeneralSettings().InvoiceDueDay
	}
	return ClampDueDay(g.InvoiceDueDay, DefaultGeneralSettings().InvoiceDueDay)
}

// LateFeePercent is denda keterlambatan (%) atas tunggakan (0 = nonaktif).
func (s *Store) LateFeePercent(ctx context.Context, tenantID xid.ID) float64 {
	g, err := s.GetGeneralSettings(ctx, tenantID)
	if err != nil {
		return DefaultGeneralSettings().LateFeePercent
	}
	if g.LateFeePercent < 0 || g.LateFeePercent > 100 {
		return DefaultGeneralSettings().LateFeePercent
	}
	return g.LateFeePercent
}

// WalletEnabled menunjukkan apakah fitur saldo pelanggan aktif untuk tenant.
func (s *Store) WalletEnabled(ctx context.Context, tenantID xid.ID) bool {
	g, err := s.GetGeneralSettings(ctx, tenantID)
	if err != nil {
		return false
	}
	return g.WalletEnabled
}

// WalletMinTopup adalah nominal minimum topup saldo (Rp).
func (s *Store) WalletMinTopup(ctx context.Context, tenantID xid.ID) int64 {
	g, err := s.GetGeneralSettings(ctx, tenantID)
	if err != nil {
		return DefaultGeneralSettings().WalletMinTopup
	}
	return g.WalletMinTopup
}
