package billing

import (
	"testing"
	"time"

	"github.com/dianrp-space/d5net-billing/internal/store"
)

func TestNextCycleAnchor(t *testing.T) {
	loc := time.FixedZone("WIB", 7*3600)
	jan5 := time.Date(2026, 1, 5, 9, 0, 0, 0, loc)
	if got := NextCycleAnchor(jan5, 1); !got.Equal(time.Date(2026, 2, 1, 12, 0, 0, 0, loc)) {
		t.Fatalf("day1 = %s", got)
	}
	if got := NextCycleAnchor(jan5, 25); !got.Equal(time.Date(2026, 1, 25, 12, 0, 0, 0, loc)) {
		t.Fatalf("day25 = %s", got)
	}
	jan1 := time.Date(2026, 1, 1, 12, 0, 0, 0, loc)
	if got := NextCycleAnchor(jan1, 1); !got.Equal(time.Date(2026, 2, 1, 12, 0, 0, 0, loc)) {
		t.Fatalf("on-day = %s", got)
	}
	jan25 := time.Date(2026, 1, 25, 12, 0, 0, 0, loc)
	if got := NextCycleAnchor(jan25, 25); !got.Equal(time.Date(2026, 2, 25, 12, 0, 0, 0, loc)) {
		t.Fatalf("on-anchor = %s", got)
	}
}

func TestNextDueDate(t *testing.T) {
	loc := time.FixedZone("WIB", 7*3600)
	// Issued before the due day → same month.
	jan5 := time.Date(2026, 1, 5, 9, 0, 0, 0, loc)
	if got := NextDueDate(jan5, 10); !got.Equal(time.Date(2026, 1, 10, 12, 0, 0, 0, loc)) {
		t.Fatalf("before = %s", got)
	}
	// Issued on the due day → that same day.
	jan10 := time.Date(2026, 1, 10, 0, 0, 0, 0, loc)
	if got := NextDueDate(jan10, 10); !got.Equal(time.Date(2026, 1, 10, 12, 0, 0, 0, loc)) {
		t.Fatalf("on-day = %s", got)
	}
	// Issued after the due day → next month.
	jan20 := time.Date(2026, 1, 20, 9, 0, 0, 0, loc)
	if got := NextDueDate(jan20, 10); !got.Equal(time.Date(2026, 2, 10, 12, 0, 0, 0, loc)) {
		t.Fatalf("after = %s", got)
	}
}

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
