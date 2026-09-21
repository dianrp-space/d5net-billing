package store

import (
	"testing"
	"time"
)

func TestTrafficMonthKey(t *testing.T) {
	got := TrafficMonthKey(time.Date(2026, 9, 21, 15, 4, 5, 0, time.FixedZone("WIB", 7*3600)))
	want := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("month key = %v, want %v", got, want)
	}
}

func TestTrafficDelta(t *testing.T) {
	if got := trafficDelta(1000, 400); got != 600 {
		t.Fatalf("delta = %d", got)
	}
	// Counter reset (reconnect): delta = nilai saat ini, bukan negatif.
	if got := trafficDelta(300, 1000); got != 300 {
		t.Fatalf("reset delta = %d", got)
	}
	if got := trafficDelta(-5, 10); got != 0 {
		t.Fatalf("negative = %d", got)
	}
	if got := trafficDelta(0, 0); got != 0 {
		t.Fatalf("zero = %d", got)
	}
}
