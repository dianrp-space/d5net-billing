package routeros

import (
	"strings"
	"testing"
)

func TestIsolirPortalHostPath(t *testing.T) {
	host, _ := isolirPortalHostPath("https://billing.dianrp.com/login", "d5nnet")
	if host != "billing.dianrp.com" {
		t.Fatalf("host=%q", host)
	}
	host, _ = isolirPortalHostPath("https://billing.dianrp.com:8443/", "x")
	if host != "billing.dianrp.com" {
		t.Fatalf("host with port=%q", host)
	}
}

func TestProxyRedirectPropSetsROS7First(t *testing.T) {
	sets := proxyRedirectPropSets("10.250.0.0/24", "https://billing.example.com/isolir/acme")
	if len(sets) != 2 {
		t.Fatalf("len=%d", len(sets))
	}
	joined0 := strings.Join(sets[0], " ")
	if !strings.Contains(joined0, "=action=redirect") || !strings.Contains(joined0, "=action-data=https://billing.example.com/isolir/acme") {
		t.Fatalf("ros7 props = %v", sets[0])
	}
	if strings.Contains(joined0, "redirect-to") {
		t.Fatal("ros7 set must not use redirect-to")
	}
	joined1 := strings.Join(sets[1], " ")
	if !strings.Contains(joined1, "=action=deny") || !strings.Contains(joined1, "=redirect-to=") {
		t.Fatalf("ros6 props = %v", sets[1])
	}
}
