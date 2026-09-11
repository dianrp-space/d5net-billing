package store

import "testing"

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

func TestIsolirPoolName(t *testing.T) {
	if got := IsolirPoolName(IsolirNetworkSettings{PoolName: "pool-isolir"}); got != "pool-isolir" {
		t.Fatalf("named = %q", got)
	}
	if got := IsolirPoolName(IsolirNetworkSettings{}); got != "isolir" {
		t.Fatalf("default = %q", got)
	}
}
