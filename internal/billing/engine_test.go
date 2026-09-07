package billing

import (
	"testing"
	"time"

	"github.com/dianrp/drp-billing/internal/store"
)

func TestProrateAmount(t *testing.T) {
	if ProrateAmount(300_000, 15, 30) != 150_000 {
		t.Fatal("half month")
	}
	if ProrateAmount(100, 0, 0) != 100 {
		t.Fatal("zero period")
	}
}

func TestCalculateInvoiceAmount(t *testing.T) {
	e := New(nil)
	plan := &store.Plan{Price: 100_000, TaxPercent: 11}
	sub, tax := e.CalculateInvoiceAmount(plan, 0, 0)
	if sub != 100000 || tax != 11000 {
		t.Fatalf("sub=%d tax=%d", sub, tax)
	}
	sub, _ = e.CalculateInvoiceAmount(plan, 15, 30)
	if sub != 50000 {
		t.Fatalf("prorate sub=%d", sub)
	}
}

func TestApplyLateFee(t *testing.T) {
	if ApplyLateFee(100000, 10) != 10000 {
		t.Fatal("late fee")
	}
}

func TestRemainingDaysUntil(t *testing.T) {
	now := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	until := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	if d := RemainingDaysUntil(now, until, 30); d != 22 {
		t.Fatalf("days=%d want 22", d)
	}
	if RemainingDaysUntil(now, now, 30) != 0 {
		t.Fatal("same day")
	}
}

func TestPlanChangeDelta(t *testing.T) {
	// old 300k, new 500k, 15/30 days → credit 150k, charge 250k, delta 100k
	oldC := ProrateAmount(300_000, 15, 30)
	newC := ProrateAmount(500_000, 15, 30)
	delta := newC - oldC
	if oldC != 150_000 || newC != 250_000 || delta != 100_000 {
		t.Fatalf("old=%d new=%d delta=%d", oldC, newC, delta)
	}
	// downgrade
	deltaDown := ProrateAmount(300_000, 15, 30) - ProrateAmount(500_000, 15, 30)
	if deltaDown != -100_000 {
		t.Fatalf("downgrade delta=%d", deltaDown)
	}
}

func TestNextBillDate(t *testing.T) {
	e := New(nil)
	from := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	if e.NextBillDate(from, "monthly").Month() != 2 {
		t.Fatal("monthly")
	}
	if e.NextBillDate(from, "daily").Day() != 16 {
		t.Fatal("daily")
	}
}
