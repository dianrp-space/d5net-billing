package store

import (
	"strings"
	"testing"
)

func TestNormalizeHexColor(t *testing.T) {
	if got := NormalizeHexColor(""); got != "" {
		t.Fatalf("empty = %q", got)
	}
	if got := NormalizeHexColor("5a5a40"); got != "#5A5A40" {
		t.Fatalf("bare = %q", got)
	}
	if got := NormalizeHexColor("#8c7355"); got != "#8C7355" {
		t.Fatalf("hex = %q", got)
	}
	if got := NormalizeHexColor("not-a-color"); got != "" {
		t.Fatalf("invalid = %q", got)
	}
}

func TestNormalizeGeneralSettingsIsolirGrace(t *testing.T) {
	if got := NormalizeGeneralSettings(GeneralSettings{IsolirGraceDays: -1}).IsolirGraceDays; got != 0 {
		t.Fatalf("neg = %d", got)
	}
	if got := NormalizeGeneralSettings(GeneralSettings{IsolirGraceDays: 7}).IsolirGraceDays; got != 7 {
		t.Fatalf("ok = %d", got)
	}
	if got := NormalizeGeneralSettings(GeneralSettings{IsolirGraceDays: 90}).IsolirGraceDays; got != 30 {
		t.Fatalf("cap = %d", got)
	}
}

func TestNormalizeGeneralSettingsCycleStartDay(t *testing.T) {
	if got := NormalizeGeneralSettings(GeneralSettings{}).BillingCycleStartDay; got != 1 {
		t.Fatalf("default = %d", got)
	}
	if got := NormalizeGeneralSettings(GeneralSettings{BillingCycleStartDay: 25}).BillingCycleStartDay; got != 25 {
		t.Fatalf("ok = %d", got)
	}
	if got := NormalizeGeneralSettings(GeneralSettings{BillingCycleStartDay: 31}).BillingCycleStartDay; got != 28 {
		t.Fatalf("cap = %d", got)
	}
}

func TestNormalizeGeneralSettingsPrimaryColor(t *testing.T) {
	g := NormalizeGeneralSettings(GeneralSettings{Timezone: "Asia/Jakarta", PrimaryColor: "#1a2b3c"})
	if g.PrimaryColor != "#1A2B3C" {
		t.Fatalf("primary = %q", g.PrimaryColor)
	}
}

func TestNormalizeGeneralSettingsInvoiceDueDay(t *testing.T) {
	if got := NormalizeGeneralSettings(GeneralSettings{}).InvoiceDueDay; got != 10 {
		t.Fatalf("default = %d", got)
	}
	if got := NormalizeGeneralSettings(GeneralSettings{InvoiceDueDay: 25}).InvoiceDueDay; got != 25 {
		t.Fatalf("ok = %d", got)
	}
	if got := NormalizeGeneralSettings(GeneralSettings{InvoiceDueDay: 31}).InvoiceDueDay; got != 28 {
		t.Fatalf("cap = %d", got)
	}
}

func TestClampDueDay(t *testing.T) {
	if got := ClampDueDay(0, 10); got != 10 {
		t.Fatalf("zero -> def = %d", got)
	}
	if got := ClampDueDay(15, 10); got != 15 {
		t.Fatalf("ok = %d", got)
	}
	if got := ClampDueDay(31, 10); got != 28 {
		t.Fatalf("cap = %d", got)
	}
	if got := ClampDueDay(0, 0); got != 1 {
		t.Fatalf("no def = %d", got)
	}
}

func TestNormalizeGeneralSettingsWallet(t *testing.T) {
	def := DefaultGeneralSettings()
	if def.WalletEnabled {
		t.Fatal("wallet harus nonaktif secara bawaan")
	}
	if def.WalletMinTopup != 10000 {
		t.Fatalf("default min topup = %d", def.WalletMinTopup)
	}
	if got := NormalizeGeneralSettings(GeneralSettings{WalletMinTopup: 0}).WalletMinTopup; got != 10000 {
		t.Fatalf("min topup 0 harus jatuh ke default, got %d", got)
	}
	if got := NormalizeGeneralSettings(GeneralSettings{WalletMinTopup: 5000}).WalletMinTopup; got != 5000 {
		t.Fatalf("min topup 5000 = %d", got)
	}
	if got := NormalizeGeneralSettings(GeneralSettings{WalletMinTopup: 999_999_999}).WalletMinTopup; got != 100_000_000 {
		t.Fatalf("min topup harus di-cap, got %d", got)
	}
	if !NormalizeGeneralSettings(GeneralSettings{WalletEnabled: true}).WalletEnabled {
		t.Fatal("wallet_enabled harus dipertahankan")
	}
}

func TestNormalizeGeneralSettingsTaglines(t *testing.T) {
	if DefaultAdminTagline != "ISP Billing" {
		t.Fatalf("default admin tagline = %q", DefaultAdminTagline)
	}
	if DefaultPortalTagline != "Portal pelanggan" {
		t.Fatalf("default portal tagline = %q", DefaultPortalTagline)
	}
	got := NormalizeGeneralSettings(GeneralSettings{})
	if got.AdminTagline != DefaultAdminTagline || got.PortalTagline != DefaultPortalTagline {
		t.Fatalf("tagline kosong harus jatuh ke default, got %+v", got)
	}
	got = NormalizeGeneralSettings(GeneralSettings{AdminTagline: "  Panel  ", PortalTagline: "Area Pelanggan"})
	if got.AdminTagline != "Panel" || got.PortalTagline != "Area Pelanggan" {
		t.Fatalf("tagline harus di-trim & dipertahankan, got %+v", got)
	}
	long := strings.Repeat("x", 100)
	if got := NormalizeGeneralSettings(GeneralSettings{AdminTagline: long}).AdminTagline; len([]rune(got)) != 60 {
		t.Fatalf("tagline harus dipotong 60 karakter, got %d", len([]rune(got)))
	}
}
