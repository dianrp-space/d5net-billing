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

func TestIsolirBlockPosition(t *testing.T) {
	rules := []rosRule{
		{ID: "*1", Comment: isolirRuleCommentDNS},
		{ID: "*2", Comment: isolirRuleCommentDNS + "-tcp"},
		{ID: "*3", Comment: isolirRuleCommentPortal},
		{ID: "*4", Comment: "user-rule"},
		{ID: "*5", Comment: isolirRuleCommentBlock},
	}
	id, dest, move := isolirBlockPosition(rules, isolirRuleCommentBlock, isolirRuleCommentPortal)
	if !move || id != "*5" || dest != "*4" {
		t.Fatalf("want move *5 before *4, got id=%q dest=%q move=%v", id, dest, move)
	}

	rules = []rosRule{
		{ID: "*1", Comment: isolirRuleCommentPortal},
		{ID: "*2", Comment: isolirRuleCommentBlock},
		{ID: "*3", Comment: "user-rule"},
	}
	if _, _, move = isolirBlockPosition(rules, isolirRuleCommentBlock, isolirRuleCommentPortal); move {
		t.Fatal("already right after portal must not move")
	}

	rules = []rosRule{
		{ID: "*1", Comment: isolirRuleCommentBlock},
		{ID: "*2", Comment: isolirRuleCommentPortal},
	}
	id, dest, move = isolirBlockPosition(rules, isolirRuleCommentBlock, isolirRuleCommentPortal)
	if !move || id != "*1" || dest != "" {
		t.Fatalf("want move *1 to end, got id=%q dest=%q move=%v", id, dest, move)
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
