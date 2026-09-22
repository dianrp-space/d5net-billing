package store

import (
	"strings"
	"testing"
)

func TestDefaultIsolirHTMLUsesPrimaryColor(t *testing.T) {
	out := DefaultIsolirHTML("Acme", "", "https://x/login", "#1a2b3c")
	if !strings.Contains(out, "color:#1A2B3C") || !strings.Contains(out, "background:#1A2B3C") {
		t.Fatal("expected tenant primary color in h1 and button")
	}
	def := DefaultIsolirHTML("Acme", "", "https://x/login", "")
	if !strings.Contains(def, "color:#5A5A40") || !strings.Contains(def, "background:#5A5A40") {
		t.Fatal("expected olive fallback for empty primary")
	}
	bad := DefaultIsolirHTML("Acme", "", "https://x/login", "bukan-warna")
	if strings.Contains(bad, "bukan-warna") || !strings.Contains(bad, "#5A5A40") {
		t.Fatal("invalid primary must fall back to olive")
	}
}

func TestIsolirProfileName(t *testing.T) {
	cfg := IsolirNetworkSettings{ProfileName: "isolir-tenant"}
	if got := IsolirProfileName(cfg, nil); got != "isolir-tenant" {
		t.Fatalf("from settings = %q", got)
	}
	plan := &Plan{}
	if got := IsolirProfileName(cfg, plan); got != "isolir-tenant" {
		t.Fatalf("empty plan override = %q", got)
	}
	name := "isolir-paket"
	plan.IsolirProfile = &name
	if got := IsolirProfileName(cfg, plan); got != "isolir-paket" {
		t.Fatalf("plan override = %q", got)
	}
	if got := IsolirProfileName(IsolirNetworkSettings{}, nil); got != "isolir" {
		t.Fatalf("default = %q", got)
	}
}

func TestIsolirPortalURL(t *testing.T) {
	if got := IsolirClientPath("acme"); got != "/login" {
		t.Fatalf("path = %q", got)
	}
	if got := IsolirPortalURL("https://billing.example.com/", "acme"); got != "https://billing.example.com/login" {
		t.Fatalf("url = %q", got)
	}
	if got := IsolirPortalURL("", "acme"); got != "/login" {
		t.Fatalf("relative = %q", got)
	}
}

func TestIsolirLandingURL(t *testing.T) {
	if got := IsolirLandingPath(); got != "/api/public/isolir" {
		t.Fatalf("path = %q", got)
	}
	if got := IsolirLandingURL("https://billing.example.com/"); got != "https://billing.example.com/api/public/isolir" {
		t.Fatalf("url = %q", got)
	}
	if got := IsolirLandingURL(""); got != "/api/public/isolir" {
		t.Fatalf("relative = %q", got)
	}
}

func TestIsolirPoolName(t *testing.T) {
	if got := IsolirPoolName(IsolirNetworkSettings{PoolName: "pool-isolir"}); got != "pool-isolir" {
		t.Fatalf("named = %q", got)
	}
	if got := IsolirPoolName(IsolirNetworkSettings{}); got != "isolir" {
		t.Fatalf("default = %q", got)
	}
}

func TestDefaultIsolirNetworkSettings(t *testing.T) {
	def := DefaultIsolirNetworkSettings()
	if def.RedirectMode != IsolirRedirectDSTNAT {
		t.Fatalf("redirect mode = %q, want %q", def.RedirectMode, IsolirRedirectDSTNAT)
	}
	if def.IsolirHostPort != DefaultIsolirHostPort {
		t.Fatalf("host port = %q, want %q", def.IsolirHostPort, DefaultIsolirHostPort)
	}
}

func TestIsolirHostPortOrDefault(t *testing.T) {
	if got := IsolirHostPortOrDefault(IsolirNetworkSettings{IsolirHostPort: "9000"}); got != "9000" {
		t.Fatalf("explicit = %q", got)
	}
	if got := IsolirHostPortOrDefault(IsolirNetworkSettings{IsolirHostPort: "  "}); got != DefaultIsolirHostPort {
		t.Fatalf("blank falls back = %q", got)
	}
	if got := IsolirHostPortOrDefault(IsolirNetworkSettings{}); got != DefaultIsolirHostPort {
		t.Fatalf("default = %q", got)
	}
}
