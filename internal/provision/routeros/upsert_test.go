package routeros

import (
	"strings"
	"testing"
)

func TestParseROSProp(t *testing.T) {
	k, v, ok := parseROSProp("=rate-limit=1M/1M")
	if !ok || k != "rate-limit" || v != "1M/1M" {
		t.Fatalf("got %q %q %v", k, v, ok)
	}
	if _, _, ok := parseROSProp("=numbers=*1"); ok {
		t.Fatal("numbers should be ignored")
	}
}

func TestRowMatchesProps(t *testing.T) {
	row := map[string]string{
		"ranges":   "10.10.70.2-10.10.70.254",
		"comment":  "D5N-NET",
		"enabled":  "true",
		"disabled": "false",
		"port":     "8080",
	}
	if !rowMatchesProps(row, []string{"=ranges=10.10.70.2-10.10.70.254", "=comment=D5N-NET"}) {
		t.Fatal("expected match")
	}
	if rowMatchesProps(row, []string{"=ranges=10.10.70.2-10.10.70.253"}) {
		t.Fatal("ranges differ")
	}
	if !rowMatchesProps(row, []string{"=enabled=yes", "=port=8080"}) {
		t.Fatal("enabled yes vs true")
	}
	if !rowMatchesProps(row, []string{"=disabled=no"}) {
		t.Fatal("disabled false vs no")
	}
}

func TestRowMatchesPropsPPPSecret(t *testing.T) {
	row := map[string]string{
		"profile":        "isolir",
		"disabled":       "false",
		"comment":        "ISOLIR D5N-NET:RDA-202609-0002 Dian Rama Putra",
		"remote-address": "",
	}
	props := []string{"=profile=isolir", "=disabled=no", "=comment=ISOLIR D5N-NET:RDA-202609-0002 Dian Rama Putra"}
	if !rowMatchesProps(row, props) {
		t.Fatal("isolir secret should already match")
	}
	if fieldFilled(row, "remote-address") {
		t.Fatal("empty remote-address should not unset")
	}
}

func TestIsolirLegacyCommentMapping(t *testing.T) {
	for _, c := range []string{
		isolirRuleCommentDNS, isolirRuleCommentDNS + "-tcp", isolirRuleCommentPortal,
		isolirRuleCommentNATProxy, isolirProxyAllowComment, isolirProxyRedirectComment,
	} {
		legacy := isolirLegacyComment(c)
		if legacy == "" || legacy == c {
			t.Fatalf("missing legacy mapping for %q", c)
		}
		if !strings.HasPrefix(legacy, "drp-isolir") {
			t.Fatalf("legacy %q should keep drp- prefix", legacy)
		}
		if strings.HasPrefix(legacy, "d5n-") {
			t.Fatalf("legacy %q must not use new prefix", legacy)
		}
	}
	if isolirLegacyComment("unrelated") != "" {
		t.Fatal("unknown comments must have no legacy")
	}
}

func TestCommentExactMatchDNSVsTCP(t *testing.T) {
	dns := "d5n-isolir:dns"
	tcp := "d5n-isolir:dns-tcp"
	if dns == tcp {
		t.Fatal("comments must differ")
	}
	if !(len(tcp) > len(dns) && tcp[:len(dns)] == dns) {
		t.Fatal("tcp comment is a prefix of dns; upsert must use exact equality, not Contains")
	}
	if isolirLegacyComment(dns) != "drp-isolir:dns" || isolirLegacyComment(tcp) != "drp-isolir:dns-tcp" {
		t.Fatal("legacy mapping must cover dns + dns-tcp exactly")
	}
}
