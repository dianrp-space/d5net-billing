package store

import (
	"strings"
	"testing"
	"time"
	"unicode"
)

func TestFormatInvoiceNumber(t *testing.T) {
	at := time.Date(2026, 9, 10, 8, 0, 0, 0, time.UTC)
	got := formatInvoiceNumber("acme-isp", "C001", at, "AB3K7")
	if got != "INV-acme-isp-C001-092026-AB3K7" {
		t.Fatalf("got %q", got)
	}
}

func TestFormatInvoiceNumberSanitizes(t *testing.T) {
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	got := formatInvoiceNumber(" Acme ISP ", "PLG 01/A", at, "XXXXX")
	if got != "INV-Acme-ISP-PLG-01A-012026-XXXXX" {
		t.Fatalf("got %q", got)
	}
}

func TestFormatInvoiceNumberMaxLen(t *testing.T) {
	at := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
	got := formatInvoiceNumber(strings.Repeat("s", 40), strings.Repeat("c", 40), at, "ZZZZZ")
	if len(got) > invoiceNumberMaxLen {
		t.Fatalf("len %d: %q", len(got), got)
	}
	if !strings.HasPrefix(got, "INV-") || !strings.HasSuffix(got, "-122026-ZZZZZ") {
		t.Fatalf("got %q", got)
	}
}

func TestFormatInvoiceNumberRandom(t *testing.T) {
	got := FormatInvoiceNumber("demo", "A12", time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC))
	if !strings.HasPrefix(got, "INV-demo-A12-092026-") {
		t.Fatalf("got %q", got)
	}
	suf := strings.TrimPrefix(got, "INV-demo-A12-092026-")
	if len(suf) != 5 {
		t.Fatalf("suffix %q", suf)
	}
	for _, r := range suf {
		if !strings.ContainsRune(invoiceNumberAlphabet, r) {
			t.Fatalf("bad suffix rune %q in %q", r, suf)
		}
		if unicode.IsSpace(r) {
			t.Fatal("whitespace")
		}
	}
}
