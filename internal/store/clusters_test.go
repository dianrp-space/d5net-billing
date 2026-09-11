package store

import (
	"testing"
	"time"
)

func TestFormatCustomerCode(t *testing.T) {
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	got := FormatCustomerCode("{prefix}-{yyyymm}{seq}", "BTC", 4, now, 1)
	want := "BTC-2026090001"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	got2 := FormatCustomerCode("{prefix}/{yyyy}/{seq}", "ABC", 3, now, 12)
	want2 := "ABC/2026/012"
	if got2 != want2 {
		t.Fatalf("got %q want %q", got2, want2)
	}
}

func TestNormalizeClusterCode(t *testing.T) {
	if got := NormalizeClusterCode("dl-ma!"); got != "DLMA" {
		t.Fatalf("got %q", got)
	}
}
