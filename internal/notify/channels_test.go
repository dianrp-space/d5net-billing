package notify

import (
	"strings"
	"testing"
)

func TestEmailNotifierResolvedPrefersStructOverEnv(t *testing.T) {
	t.Setenv("SMTP_HOST", "env.example")
	t.Setenv("SMTP_PORT", "25")
	t.Setenv("SMTP_USER", "env-user")
	t.Setenv("SMTP_PASS", "env-pass")
	t.Setenv("SMTP_FROM", "env@example.com")

	n := &EmailNotifier{
		Host:     "smtp.tenant.id",
		Port:     "465",
		User:     "tenant",
		Pass:     "secret",
		From:     "noreply@tenant.id",
		FromName: "Tenant",
	}
	host, port, user, pass, from, fromName := n.resolved()
	if host != "smtp.tenant.id" || port != "465" || user != "tenant" || pass != "secret" || from != "noreply@tenant.id" || fromName != "Tenant" {
		t.Fatalf("resolved = %s %s %s %s %s %s", host, port, user, pass, from, fromName)
	}

	fallback := &EmailNotifier{}
	host, port, user, pass, from, fromName = fallback.resolved()
	if host != "env.example" || port != "25" || user != "env-user" || pass != "env-pass" || from != "env@example.com" || fromName != "" {
		t.Fatalf("env fallback = %s %s %s %s %s %s", host, port, user, pass, from, fromName)
	}

	tenantNoAuth := &EmailNotifier{Host: "smtp.tenant.id", Port: "587", From: "noreply@tenant.id"}
	host, port, user, pass, from, _ = tenantNoAuth.resolved()
	if host != "smtp.tenant.id" || port != "587" || user != "" || pass != "" || from != "noreply@tenant.id" {
		t.Fatalf("tenant must not mix env credentials: %s %s %s %s %s", host, port, user, pass, from)
	}

	if smtpPortOrDefault("") != "587" || smtpPortOrDefault("0") != "587" {
		t.Fatal("expected default SMTP port 587")
	}
}

func TestBuildEmailMessageIncludesFromNameAndUTF8Subject(t *testing.T) {
	raw := string(buildEmailMessage("noreply@tenant.id", "ISP Cibinong", "user@example.com", "Tagihan September", "Halo"))
	if !strings.Contains(raw, "From:") || !strings.Contains(raw, "noreply@tenant.id") {
		t.Fatalf("missing From: %q", raw)
	}
	if !strings.Contains(raw, "To: user@example.com") {
		t.Fatalf("missing To: %q", raw)
	}
	if !strings.Contains(raw, "charset=UTF-8") {
		t.Fatalf("missing charset: %q", raw)
	}
	if !strings.Contains(raw, "Halo") {
		t.Fatalf("missing body: %q", raw)
	}
}
