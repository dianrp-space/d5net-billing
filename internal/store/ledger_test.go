package store

import (
	"testing"
	"time"
)

func TestNormalBalance(t *testing.T) {
	cases := []struct {
		typ         string
		debit       int64
		credit      int64
		wantBalance int64
	}{
		{"asset", 100, 30, 70},
		{"expense", 50, 0, 50},
		{"revenue", 0, 200, 200},
		{"liability", 10, 90, 80},
		{"equity", 0, 300, 300},
		{"ASSET", 100, 30, 70},
	}
	for _, c := range cases {
		if got := NormalBalance(c.typ, c.debit, c.credit); got != c.wantBalance {
			t.Errorf("NormalBalance(%q,%d,%d) = %d, want %d", c.typ, c.debit, c.credit, got, c.wantBalance)
		}
	}
}

func TestNormalizeRange(t *testing.T) {
	from := time.Date(2026, 3, 10, 15, 0, 0, 0, time.UTC)
	to := time.Date(2026, 4, 5, 8, 0, 0, 0, time.UTC)
	start, end := normalizeRange(from, to)
	if dateOnly(start) != "2026-03-10" || dateOnly(end) != "2026-04-05" {
		t.Fatalf("normalizeRange = %s..%s", dateOnly(start), dateOnly(end))
	}
	// Rentang terbalik ditukar.
	start, end = normalizeRange(to, from)
	if !start.Before(end) {
		t.Fatalf("rentang terbalik tidak ditukar: %s..%s", dateOnly(start), dateOnly(end))
	}
}
