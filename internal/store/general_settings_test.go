package store

import "testing"

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
