package provision

import (
	"context"
	"strconv"
	"strings"
	"unicode"

	"github.com/dianrp-space/d5net-billing/internal/store"
	"github.com/dianrp-space/d5net-billing/internal/xid"
)

type Drift struct {
	Username string `json:"username"`
	Field    string `json:"field"`
	Desired  string `json:"desired"`
	Actual   string `json:"actual"`
}

func Reconcile(ctx context.Context, st *store.Store, p Provisioner, tenantID xid.ID, routerID xid.ID, apply bool) ([]Drift, error) {
	subs, _, err := st.ListSubscriptions(ctx, tenantID, "active", nil, 1000, 0)
	if err != nil {
		return nil, err
	}
	sessions, err := p.ActiveSessions(ctx, routerID)
	if err != nil {
		return nil, err
	}
	online := map[string]Session{}
	for _, s := range sessions {
		online[s.Username] = s
	}
	var drifts []Drift
	for _, sub := range subs {
		if sub.RouterID == nil || *sub.RouterID != routerID {
			continue
		}
		if _, ok := online[sub.Username]; !ok && sub.Status == "active" {
			drifts = append(drifts, Drift{Username: sub.Username, Field: "session", Desired: "online-or-secret", Actual: "missing"})
			if apply {
				pw := ""
				if sub.Password != nil {
					pw = *sub.Password
				}
				appName, _ := st.EffectiveAppName(ctx, tenantID)
				spec := &ServiceSpec{
					TenantID: tenantID, SubscriptionID: sub.ID, RouterID: routerID,
					Username: sub.Username, Password: pw, ServiceType: sub.ServiceType,
					Comment: CommentTag(appName, sub.CustomerCode, sub.CustomerName),
				}
				_ = p.Apply(ctx, spec)
			}
		}
	}
	return drifts, nil
}

// SanitizeBrandPrefix turns an app name into a RouterOS-safe comment prefix.
func SanitizeBrandPrefix(appName string) string {
	appName = strings.TrimSpace(appName)
	var b strings.Builder
	for _, r := range appName {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '_':
			if b.Len() > 0 {
				b.WriteByte('-')
			}
		}
	}
	s := strings.Trim(b.String(), "-")
	if s == "" {
		return "d5n"
	}
	if len(s) > 32 {
		s = s[:32]
	}
	return s
}

// FormatRpDots formats an amount like 150000 → "150.000" (Indonesia thousand separator).
func FormatRpDots(n int64) string {
	if n < 0 {
		n = -n
	}
	s := strconv.FormatInt(n, 10)
	if len(s) <= 3 {
		return s
	}
	var b strings.Builder
	rem := len(s) % 3
	if rem > 0 {
		b.WriteString(s[:rem])
		if len(s) > rem {
			b.WriteByte('.')
		}
	}
	for i := rem; i < len(s); i += 3 {
		if i > rem {
			b.WriteByte('.')
		}
		b.WriteString(s[i : i+3])
	}
	return b.String()
}

// ProfileComment is RouterOS comment for PPP/hotspot profiles: "Brand Rp150.000".
func ProfileComment(appName string, price int64) string {
	prefix := SanitizeBrandPrefix(appName)
	if price <= 0 {
		return prefix
	}
	return prefix + " Rp" + FormatRpDots(price)
}

// CommentTag builds a RouterOS-friendly ownership comment: "Prefix:KODE Nama".
func CommentTag(appName, customerCode, customerName string) string {
	prefix := SanitizeBrandPrefix(appName)
	code := sanitizeCommentPart(customerCode)
	name := sanitizeCommentPart(customerName)
	switch {
	case code != "" && name != "":
		return prefix + ":" + code + " " + name
	case code != "":
		return prefix + ":" + code
	case name != "":
		return prefix + ":" + name
	default:
		return prefix
	}
}

const IsolirCommentPrefix = "ISOLIR "

// WithIsolirComment prefixes an existing secret comment with "ISOLIR " (idempotent).
func WithIsolirComment(comment string) string {
	c := strings.TrimSpace(comment)
	if c == "" {
		return strings.TrimSpace(IsolirCommentPrefix)
	}
	upper := strings.ToUpper(c)
	if strings.HasPrefix(upper, "ISOLIR ") || upper == "ISOLIR" {
		return c
	}
	return IsolirCommentPrefix + c
}

// SecretLooksIsolir reports whether a PPP/hotspot account is still on the isolir profile
// (or still tagged ISOLIR in the comment). planProfile avoids false positives when the
// paket profile happens to share the isolir profile name.
func SecretLooksIsolir(profile, comment, isolirProfile, planProfile string) bool {
	c := strings.TrimSpace(comment)
	upper := strings.ToUpper(c)
	if strings.HasPrefix(upper, "ISOLIR ") || upper == "ISOLIR" {
		return true
	}
	p := strings.TrimSpace(profile)
	iso := strings.TrimSpace(isolirProfile)
	plan := strings.TrimSpace(planProfile)
	if iso == "" || p == "" {
		return false
	}
	if !strings.EqualFold(p, iso) {
		return false
	}
	if plan != "" && strings.EqualFold(iso, plan) {
		return false
	}
	return true
}

// WithoutIsolirComment strips a leading "ISOLIR " prefix from a secret comment.
func WithoutIsolirComment(comment string) string {
	c := strings.TrimSpace(comment)
	upper := strings.ToUpper(c)
	if strings.HasPrefix(upper, "ISOLIR ") {
		return strings.TrimSpace(c[len("ISOLIR "):])
	}
	if upper == "ISOLIR" {
		return ""
	}
	return c
}

func sanitizeCommentPart(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "=", "-")
	return strings.Join(strings.Fields(s), " ")
}

// IsOwnedComment reports whether comment belongs to this app (effective prefix or legacy drp:).
func IsOwnedComment(comment, appName string) bool {
	prefix := SanitizeBrandPrefix(appName)
	if strings.HasPrefix(comment, prefix+":") || comment == prefix {
		return true
	}
	return strings.HasPrefix(comment, "drp:") || comment == "drp"
}
