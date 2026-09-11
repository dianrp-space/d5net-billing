package store

import (
	"context"
	"crypto/rand"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/dianrp-space/d5net-billing/internal/xid"
)

const invoiceNumberMaxLen = 50

const invoiceNumberSuffixLen = 6

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

// FormatInvoiceNumber builds INV-<customer_code>-<mmyyyy><6 char>.
// Nomor yang sama dikirim sebagai merchantOrderId (ref) ke payment gateway.
func FormatInvoiceNumber(customerCode string, at time.Time) string {
	if at.IsZero() {
		at = time.Now()
	}
	return formatInvoiceNumber(customerCode, at, randomInvoiceSuffix(invoiceNumberSuffixLen))
}

func formatInvoiceNumber(customerCode string, at time.Time, suffix string) string {
	suffix = sanitizeInvoiceToken(suffix)
	if suffix == "" {
		suffix = randomInvoiceSuffix(invoiceNumberSuffixLen)
	}
	if len(suffix) > invoiceNumberSuffixLen {
		suffix = suffix[:invoiceNumberSuffixLen]
	}
	for len(suffix) < invoiceNumberSuffixLen {
		suffix += "X"
	}
	period := at.Format("012006") // mmyyyy
	code := sanitizeInvoiceToken(customerCode)
	if code == "" {
		code = "cust"
	}
	// "INV-" + code + "-" + mmyyyy + suffix (tanpa strip sebelum suffix)
	budget := invoiceNumberMaxLen - (4 + 1 + 6 + invoiceNumberSuffixLen)
	if len(code) > budget {
		code = code[:budget]
	}
	return "INV-" + code + "-" + period + suffix
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
	for i := 0; i < 12; i++ {
		num := FormatInvoiceNumber(customerCode, time.Now())
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
