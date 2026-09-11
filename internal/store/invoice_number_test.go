package store

import (
	"strings"
	"testing"
	"time"
	"unicode"
)

func TestFormatInvoiceNumber(t *testing.T) {
	at := time.Date(2026, 9, 10, 8, 0, 0, 0, time.UTC)
	got := formatInvoiceNumber("D5N-2026090001", at, "AB3K7Z")
	if got != "INV-D5N-2026090001-092026AB3K7Z" {
		t.Fatalf("got %q", got)
	}
}

func TestFormatInvoiceNumberSanitizes(t *testing.T) {
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	got := formatInvoiceNumber("PLG 01/A", at, "XXXXXX")
	if got != "INV-PLG-01A-012026XXXXXX" {
		t.Fatalf("got %q", got)
	}
}

func TestFormatInvoiceNumberMaxLen(t *testing.T) {
	at := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
	got := formatInvoiceNumber(strings.Repeat("c", 60), at, "ZZZZZZ")
	if len(got) > invoiceNumberMaxLen {
		t.Fatalf("len %d: %q", len(got), got)
	}
	if !strings.HasPrefix(got, "INV-") || !strings.HasSuffix(got, "-122026ZZZZZZ") {
		t.Fatalf("got %q", got)
	}
}

func TestFormatInvoiceNumberRandom(t *testing.T) {
	got := FormatInvoiceNumber("D5N-2026090001", time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC))
	if !strings.HasPrefix(got, "INV-D5N-2026090001-092026") {
		t.Fatalf("got %q", got)
	}
	suf := strings.TrimPrefix(got, "INV-D5N-2026090001-092026")
	if len(suf) != 6 {
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
