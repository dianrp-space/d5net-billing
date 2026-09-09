package store

import "testing"

func TestNormalizeCoverageRadiusKm(t *testing.T) {
	if NormalizeCoverageRadiusKm(nil) != nil {
		t.Fatal("nil")
	}
	z := 0.0
	if NormalizeCoverageRadiusKm(&z) != nil {
		t.Fatal("zero")
	}
	neg := -1.0
	if NormalizeCoverageRadiusKm(&neg) != nil {
		t.Fatal("neg")
	}
	ok := 0.35
	got := NormalizeCoverageRadiusKm(&ok)
	if got == nil || *got != 0.35 {
		t.Fatalf("ok = %v", got)
	}
	big := 90.0
	got = NormalizeCoverageRadiusKm(&big)
	if got == nil || *got != 50 {
		t.Fatalf("cap = %v", got)
	}
}

func TestHaversineMeters(t *testing.T) {
	// ~1.11 km north of equator at lng 0 (approx 1 degree * 111 km / 100)
	d := HaversineMeters(-6.2, 106.8, -6.2, 106.81)
	if d < 900 || d > 1300 {
		t.Fatalf("distance = %v", d)
	}
}
