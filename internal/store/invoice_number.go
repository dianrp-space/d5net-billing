package store

import (
	"context"
	"crypto/rand"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/dianrp/drp-billing/internal/xid"
)

const invoiceNumberMaxLen = 50

const invoiceNumberAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

func randomInvoiceSuffix(n int) string {
	if n <= 0 {
		return ""
	}
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		now := time.Now().UnixNano()
		for i := 0; i < n; i++ {
			b[i] = invoiceNumberAlphabet[int(now>>uint(i*5))%len(invoiceNumberAlphabet)]
		}
		return string(b)
	}
	for i := range b {
		b[i] = invoiceNumberAlphabet[int(b[i])%len(invoiceNumberAlphabet)]
	}
	return string(b)
}

// FormatInvoiceNumber builds INV-<tenant_slug>-<customer_code>-<mmyyyy>-<5 char>.
func FormatInvoiceNumber(tenantSlug, customerCode string, at time.Time) string {
	if at.IsZero() {
		at = time.Now()
	}
	return formatInvoiceNumber(tenantSlug, customerCode, at, randomInvoiceSuffix(5))
}

func formatInvoiceNumber(tenantSlug, customerCode string, at time.Time, suffix string) string {
	suffix = sanitizeInvoiceToken(suffix)
	if suffix == "" {
		suffix = randomInvoiceSuffix(5)
	}
	if len(suffix) > 5 {
		suffix = suffix[:5]
	}
	for len(suffix) < 5 {
		suffix += "X"
	}
	period := at.Format("012006") // mmyyyy
	slug := sanitizeInvoiceToken(tenantSlug)
	code := sanitizeInvoiceToken(customerCode)
	if slug == "" {
		slug = "tenant"
	}
	if code == "" {
		code = "cust"
	}
	budget := invoiceNumberMaxLen - (4 + 1 + 1 + 6 + 1 + 5)
	slug, code = fitInvoiceParts(slug, code, budget)
	return "INV-" + slug + "-" + code + "-" + period + "-" + suffix
}

func fitInvoiceParts(slug, code string, budget int) (string, string) {
	if budget < 2 {
		return "t", "c"
	}
	if len(slug)+len(code) <= budget {
		return slug, code
	}
	slugMax := budget / 2
	if slugMax < 1 {
		slugMax = 1
	}
	codeMax := budget - slugMax
	if len(slug) < slugMax {
		codeMax = budget - len(slug)
		slugMax = len(slug)
	}
	if len(code) < codeMax {
		slugMax = budget - len(code)
		codeMax = len(code)
	}
	if len(slug) > slugMax {
		slug = slug[:slugMax]
	}
	if len(code) > codeMax {
		code = code[:codeMax]
	}
	return slug, code
}

func sanitizeInvoiceToken(s string) string {
	s = strings.TrimSpace(s)
	var b strings.Builder
	b.Grow(len(s))
	prevHyphen := false
	for _, r := range s {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			prevHyphen = false
		case r == '-' || r == '_' || unicode.IsSpace(r):
			if b.Len() == 0 || prevHyphen {
				continue
			}
			b.WriteByte('-')
			prevHyphen = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func (s *Store) NextInvoiceNumber(ctx context.Context, tenantID xid.ID, customerCode string) (string, error) {
	if err := s.SetTenantContext(ctx, tenantID); err != nil {
		return "", err
	}
	slug := ""
	if ten, err := s.GetTenant(ctx, tenantID); err == nil && ten != nil {
		slug = ten.Slug
	}
	for i := 0; i < 12; i++ {
		num := FormatInvoiceNumber(slug, customerCode, time.Now())
		var exists bool
		if err := s.Pool.QueryRow(ctx, `
			SELECT EXISTS(SELECT 1 FROM invoices WHERE tenant_id=$1 AND invoice_number=$2)
		`, tenantID, num).Scan(&exists); err != nil {
			return "", err
		}
		if !exists {
			return num, nil
		}
	}
	return "", fmt.Errorf("gagal membuat nomor invoice unik")
}
