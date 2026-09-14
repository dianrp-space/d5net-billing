package routeros

import (
	"strings"
	"testing"
)

func TestIsolirPortalHostPath(t *testing.T) {
	host, _ := isolirPortalHostPath("https://billing.dianrp.com/isolir", "d5nnet")
	if host != "billing.dianrp.com" {
		t.Fatalf("host=%q", host)
	}
	host, _ = isolirPortalHostPath("https://billing.dianrp.com:8443/", "x")
	if host != "billing.dianrp.com" {
		t.Fatalf("host with port=%q", host)
	}
}

func TestIsolirBlockPosition(t *testing.T) {
	accepts := []string{isolirRuleCommentDNS, isolirRuleCommentDNS + "-tcp", isolirRuleCommentPortal}

	// Block already after every accept rule: no move.
	rules := []rosRule{
		{ID: "*1", Comment: isolirRuleCommentDNS},
		{ID: "*2", Comment: isolirRuleCommentDNS + "-tcp"},
		{ID: "*3", Comment: isolirRuleCommentPortal},
		{ID: "*4", Comment: isolirRuleCommentBlock},
	}
	if _, _, move := isolirBlockPosition(rules, isolirRuleCommentBlock, accepts...); move {
		t.Fatal("block after all accepts must not move")
	}

	// Regression: block sits right after portal but DNS rules come after it.
	// It must be moved to the end, otherwise DNS is dropped and clients can no
	// longer resolve the portal domain.
	rules = []rosRule{
		{ID: "*1", Comment: isolirRuleCommentPortal},
		{ID: "*2", Comment: isolirRuleCommentBlock},
		{ID: "*3", Comment: isolirRuleCommentDNS},
		{ID: "*4", Comment: isolirRuleCommentDNS + "-tcp"},
	}
	id, dest, move := isolirBlockPosition(rules, isolirRuleCommentBlock, accepts...)
	if !move || id != "*2" || dest != "" {
		t.Fatalf("want move *2 to end, got id=%q dest=%q move=%v", id, dest, move)
	}

	// Block before the accepts with a rule after the last accept: move before
	// that next rule.
	rules = []rosRule{
		{ID: "*1", Comment: isolirRuleCommentBlock},
		{ID: "*2", Comment: isolirRuleCommentPortal},
		{ID: "*3", Comment: "user-rule"},
	}
	id, dest, move = isolirBlockPosition(rules, isolirRuleCommentBlock, accepts...)
	if !move || id != "*1" || dest != "*3" {
		t.Fatalf("want move *1 before *3, got id=%q dest=%q move=%v", id, dest, move)
	}
}

func TestResolveHostIPv4Literal(t *testing.T) {
	got, err := resolveHostIPv4("203.0.113.5")
	if err != nil || got != "203.0.113.5" {
		t.Fatalf("literal IPv4 = %q err=%v", got, err)
	}
	if _, err := resolveHostIPv4(""); err == nil {
		t.Fatal("empty host must error")
	}
}
